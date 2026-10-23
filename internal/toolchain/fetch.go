package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxDownload bounds both the archive and the executable taken out of it, and maxExpanded
// bounds everything the archive decompresses to, members that are skipped included, so a
// hostile or broken response cannot fill the disk or memory before a checksum is compared.
const (
	maxDownload = 128 << 20
	maxExpanded = 256 << 20
)

// Fetch downloads an asset, checks the archive against its pin, takes the pinned member out of
// it, checks that against its pin too, and only then writes it as an executable named name in
// dir. Nothing is written when any step fails. It is for developing the toolchain: the shipped
// binary never downloads anything.
func Fetch(ctx context.Context, client *http.Client, asset Asset, dir, name string) (string, error) {
	if !toolName.MatchString(name) {
		return "", fmt.Errorf("%q is not a plain tool name", name)
	}

	binary, err := prepare(ctx, client, asset)
	if err != nil {
		return "", err
	}

	return install(binary, dir, name)
}

// prepare downloads an asset and returns the executable once both of its pins have held.
func prepare(ctx context.Context, client *http.Client, asset Asset) ([]byte, error) {
	archive, err := download(ctx, client, asset.URL)
	if err != nil {
		return nil, err
	}
	if got := checksum(archive); got != asset.ArchiveSHA256 {
		return nil, fmt.Errorf("the archive %s %w: it is %s and the manifest pins %s", asset.URL, ErrMismatch, got, asset.ArchiveSHA256)
	}

	binary, err := extract(archive, asset.Member)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", asset.URL, err)
	}
	if got := checksum(binary); got != asset.BinarySHA256 {
		return nil, fmt.Errorf("%s in %s %w: it is %s and the manifest pins %s", asset.Member, asset.URL, ErrMismatch, got, asset.BinarySHA256)
	}

	return binary, nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot request %s: %w", url, err)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", url, err)
	}
	defer closeBody(response.Body)

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cannot download %s: %s", url, response.Status)
	}

	return readLimited(response.Body, url)
}

// zipMagic begins every zip, the form a Python wheel is distributed in. An archive that does not
// begin with it is read as the gzipped tar a release is distributed in.
var zipMagic = []byte("PK\x03\x04")

func extract(archive []byte, member string) ([]byte, error) {
	if bytes.HasPrefix(archive, zipMagic) {
		return extractZip(archive, member)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("the archive is not gzip: %w", err)
	}
	defer closeBody(gz)

	expanding := &io.LimitedReader{R: gz, N: maxExpanded}
	reader := tar.NewReader(expanding)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("the archive has no member %s", member)
		}
		if err != nil {
			return nil, archiveFault(expanding, err)
		}
		if header.Name != member {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("member %s is not a regular file", member)
		}

		data, err := readLimited(reader, member)
		if err != nil {
			return nil, archiveFault(expanding, err)
		}

		return data, nil
	}
}

// extractZip takes a regular file out of a zip by its exact name. A name that occurs twice is
// refused, as readers of a zip disagree about which of two entries is the file.
func extractZip(archive []byte, member string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("the archive is not a valid zip: %w", err)
	}

	var found *zip.File
	for _, file := range reader.File {
		if file.Name != member {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("the archive names %s more than once", member)
		}
		found = file
	}
	if found == nil {
		return nil, fmt.Errorf("the archive has no member %s", member)
	}
	if !found.Mode().IsRegular() {
		return nil, fmt.Errorf("member %s is not a regular file", member)
	}
	if found.UncompressedSize64 > maxDownload {
		return nil, fmt.Errorf("member %s is larger than %d bytes", member, maxDownload)
	}

	opened, err := found.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot open member %s: %w", member, err)
	}
	defer closeBody(opened)

	return readLimited(opened, member)
}

// archiveFault says why reading the archive failed: running into the expansion bound looks to
// the tar reader like a truncated archive, so the bound is checked first.
func archiveFault(expanding *io.LimitedReader, err error) error {
	if expanding.N <= 0 {
		return fmt.Errorf("the archive is larger than %d bytes when expanded", int64(maxExpanded))
	}

	return fmt.Errorf("cannot read the archive: %w", err)
}

func install(binary []byte, dir, name string) (string, error) {
	staged, err := stage(binary, dir, name)
	if err != nil {
		return "", err
	}

	return commit(staged)
}

// staged is an executable written under a hidden name, waiting to take its final name.
type staged struct {
	path  string
	final string
}

// stage writes the executable beside its final name without touching it.
func stage(binary []byte, dir, name string) (staged, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return staged{}, fmt.Errorf("cannot create %s: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, "."+name+"-")
	if err != nil {
		return staged{}, fmt.Errorf("cannot stage %s in %s: %w", name, dir, err)
	}
	waiting := staged{path: file.Name(), final: filepath.Join(dir, name)}

	written, err := file.Write(binary)
	if err != nil || written != len(binary) {
		return staged{}, discard(waiting, errors.Join(fmt.Errorf("cannot write %s", waiting.path), err, file.Close()))
	}
	if err := file.Close(); err != nil {
		return staged{}, discard(waiting, fmt.Errorf("cannot write %s: %w", waiting.path, err))
	}
	if err := os.Chmod(waiting.path, 0o755); err != nil {
		return staged{}, discard(waiting, fmt.Errorf("cannot make %s executable: %w", waiting.path, err))
	}

	return waiting, nil
}

// refuseIfNotReplaceable says why the final name cannot be taken: only a regular file, or
// nothing, may be replaced.
func (s staged) refuseIfNotReplaceable() error {
	info, err := os.Lstat(s.final)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", s.final, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s exists and is not a regular file", s.final)
	}

	return nil
}

func commit(waiting staged) (string, error) {
	if err := waiting.refuseIfNotReplaceable(); err != nil {
		return "", discard(waiting, err)
	}
	if err := os.Rename(waiting.path, waiting.final); err != nil {
		return "", discard(waiting, fmt.Errorf("cannot place %s: %w", waiting.final, err))
	}

	return waiting.final, nil
}

// discard removes a staged file and returns the cause with any failure to remove it.
func discard(waiting staged, cause error) error {
	if err := os.Remove(waiting.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(cause, err)
	}

	return cause
}

func readLimited(source io.Reader, what string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(source, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", what, err)
	}
	if len(data) > maxDownload {
		return nil, fmt.Errorf("%s is larger than %d bytes", what, maxDownload)
	}

	return data, nil
}

func checksum(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

func closeBody(closer io.Closer) {
	if err := closer.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close a download: %v\n", err)
	}
}

// FetchAll fetches every tool the manifest pins for a platform into dir, in name order, and
// returns where each was placed. A tool with no asset for the platform is an error, not a skip.
// Every tool is downloaded and checked, and every executable is staged under a hidden name,
// before any final name is touched; a failure up to then leaves the directory as it was, and a
// file that was already there is never removed. If a rename fails part way through, the tools
// already renamed stay in place and the error names them.
func FetchAll(ctx context.Context, client *http.Client, m Manifest, platform, dir string) ([]string, error) {
	names := make([]string, 0, len(m.Tools))
	for name := range m.Tools {
		names = append(names, name)
	}
	slices.Sort(names)

	binaries := make([][]byte, 0, len(names))
	for _, name := range names {
		tool, asset, err := m.Lookup(name, platform)
		if err != nil {
			return nil, err
		}
		binary, err := prepare(ctx, client, asset)
		if err != nil {
			return nil, fmt.Errorf("cannot fetch %s %s: %w", name, tool.Version, err)
		}
		binaries = append(binaries, binary)
	}

	waiting := make([]staged, 0, len(names))
	for i, name := range names {
		ready, err := stage(binaries[i], dir, name)
		if err == nil {
			err = ready.refuseIfNotReplaceable()
			if err != nil {
				err = discard(ready, err)
			}
		}
		if err != nil {
			for _, earlier := range waiting {
				err = discard(earlier, err)
			}

			return nil, fmt.Errorf("cannot place %s: %w", name, err)
		}
		waiting = append(waiting, ready)
	}

	paths := make([]string, 0, len(names))
	for i, ready := range waiting {
		placed, err := commit(ready)
		if err != nil {
			for _, later := range waiting[i+1:] {
				err = discard(later, err)
			}

			return nil, fmt.Errorf("cannot place %s (already placed: %s): %w", names[i], strings.Join(paths, ", "), err)
		}
		paths = append(paths, placed)
	}

	return paths, nil
}
