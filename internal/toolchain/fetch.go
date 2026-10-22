package toolchain

import (
	"archive/tar"
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

func extract(archive []byte, member string) ([]byte, error) {
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

// archiveFault says why reading the archive failed: running into the expansion bound looks to
// the tar reader like a truncated archive, so the bound is checked first.
func archiveFault(expanding *io.LimitedReader, err error) error {
	if expanding.N <= 0 {
		return fmt.Errorf("the archive is larger than %d bytes when expanded", int64(maxExpanded))
	}

	return fmt.Errorf("cannot read the archive: %w", err)
}

func install(binary []byte, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", dir, err)
	}

	staged, err := os.CreateTemp(dir, "."+name+"-")
	if err != nil {
		return "", fmt.Errorf("cannot stage %s in %s: %w", name, dir, err)
	}
	stagedPath := staged.Name()

	fail := func(cause error) (string, error) {
		if removeErr := os.Remove(stagedPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cause = errors.Join(cause, removeErr)
		}

		return "", cause
	}

	written, err := staged.Write(binary)
	if err != nil || written != len(binary) {
		return fail(errors.Join(fmt.Errorf("cannot write %s", stagedPath), err, staged.Close()))
	}
	if err := staged.Close(); err != nil {
		return fail(fmt.Errorf("cannot write %s: %w", stagedPath, err))
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return fail(fmt.Errorf("cannot make %s executable: %w", stagedPath, err))
	}

	final := filepath.Join(dir, name)
	if err := os.Rename(stagedPath, final); err != nil {
		return fail(fmt.Errorf("cannot place %s: %w", final, err))
	}

	return final, nil
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
// Every tool is downloaded and checked before any is written, and a failure while writing
// removes what this call had written, so a failed call leaves nothing behind.
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

	paths := make([]string, 0, len(names))
	for i, name := range names {
		placed, err := install(binaries[i], dir, name)
		if err != nil {
			for _, earlier := range paths {
				if removeErr := os.Remove(earlier); removeErr != nil {
					err = errors.Join(err, removeErr)
				}
			}

			return nil, fmt.Errorf("cannot place %s: %w", name, err)
		}
		paths = append(paths, placed)
	}

	return paths, nil
}
