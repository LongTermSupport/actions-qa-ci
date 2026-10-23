// Package bundle builds the release archive of one platform: the program, every tool the
// manifest pins for the platform, and the licence of each. The same inputs always give the same
// bytes, so the checksum a release publishes can be reproduced by anyone who rebuilds it.
package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// ProgramName is the name of the program in the bundle, and of its licence beside the tools'.
const ProgramName = "actions-qa"

// ProjectLicence is the licence file the bundle needs for the program itself.
const ProjectLicence = "actions-qa-ci.txt"

// maxFile bounds one file read into a bundle, so a wrong path cannot fill the memory.
const maxFile = 256 << 20

var (
	versionPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	platformPattern = regexp.MustCompile(`^[a-z0-9]+_[a-z0-9]+$`)
)

// Options are what a bundle is built from.
type Options struct {
	// Manifest pins the tools; every tool it pins for the platform is bundled.
	Manifest toolchain.Manifest
	// Platform is the manifest's platform key, such as linux_amd64.
	Platform string
	// Version is the release, as three numbers with no leading v.
	Version string
	// Program is the actions-qa executable built for the platform.
	Program string
	// ToolDirs are the absolute directories the pinned tools are looked for in and verified.
	ToolDirs []string
	// Licences is a directory of licence files. It must hold ProjectLicence and <tool>.txt for
	// each bundled tool; every regular file in it is bundled.
	Licences string
}

// Bundle is a built archive.
type Bundle struct {
	// Name is the release asset name.
	Name string
	// Data is the gzip-compressed tar archive.
	Data []byte
	// SHA256 is the checksum of Data, in lower-case hex.
	SHA256 string
}

type entry struct {
	name    string
	mode    int64
	content []byte
}

// Build makes the bundle. A tool that is not pinned for the platform, is missing or differs
// from its pin, a missing licence and a program that is not a plain file each refuse the build:
// nothing is shipped that was not accounted for.
func Build(options Options) (Bundle, error) {
	if !versionPattern.MatchString(options.Version) {
		return Bundle{}, fmt.Errorf("the version %q must be three numbers such as 1.2.3, with no v", options.Version)
	}
	if !platformPattern.MatchString(options.Platform) {
		return Bundle{}, fmt.Errorf("the platform %q must be of the form linux_amd64", options.Platform)
	}

	tools := make([]string, 0, len(options.Manifest.Tools))
	for name := range options.Manifest.Tools {
		tools = append(tools, name)
	}
	slices.Sort(tools)

	program, err := readPlainFile(options.Program, "the program")
	if err != nil {
		return Bundle{}, err
	}
	entries := []entry{{name: "bin/" + ProgramName, mode: 0o755, content: program}}

	for _, name := range tools {
		binary, err := toolchain.Resolve(options.Manifest, name, options.Platform, options.ToolDirs)
		if err != nil {
			return Bundle{}, fmt.Errorf("cannot bundle %s: %w", name, err)
		}
		content, err := readPlainFile(binary, name)
		if err != nil {
			return Bundle{}, err
		}
		entries = append(entries, entry{name: "bin/" + name, mode: 0o755, content: content})
	}

	licences, err := readLicences(options.Licences, append([]string{strings.TrimSuffix(ProjectLicence, ".txt")}, tools...))
	if err != nil {
		return Bundle{}, err
	}
	entries = append(entries, licences...)

	data, err := archive(entries)
	if err != nil {
		return Bundle{}, err
	}
	digest := sha256.Sum256(data)

	return Bundle{
		Name:   fmt.Sprintf("actions-qa-ci_%s_%s.tar.gz", options.Version, options.Platform),
		Data:   data,
		SHA256: hex.EncodeToString(digest[:]),
	}, nil
}

// Checksums is the text of checksums.txt for the bundles: one line each, ordered by name, in
// the form sha256sum -c reads.
func Checksums(bundles []Bundle) string {
	sorted := slices.Clone(bundles)
	slices.SortFunc(sorted, func(a, b Bundle) int { return strings.Compare(a.Name, b.Name) })

	var text strings.Builder
	for _, b := range sorted {
		text.WriteString(b.SHA256 + "  " + b.Name + "\n")
	}

	return text.String()
}

// readLicences reads every regular file of the directory as LICENSES/<name>, after checking
// that the licence of each of the given subjects is among them.
func readLicences(dir string, subjects []string) ([]entry, error) {
	listing, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read the licences directory: %w", err)
	}

	var entries []entry
	present := map[string]bool{}
	for _, item := range listing {
		path := filepath.Join(dir, item.Name())
		content, err := readPlainFile(path, "the licence "+item.Name())
		if err != nil {
			return nil, err
		}
		present[item.Name()] = true
		entries = append(entries, entry{name: "LICENSES/" + item.Name(), mode: 0o644, content: content})
	}

	for _, subject := range subjects {
		if !present[subject+".txt"] {
			return nil, fmt.Errorf("the licences directory has no %s.txt, so the licence of %s would not ship", subject, subject)
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.name, b.name) })

	return entries, nil
}

// readPlainFile reads a regular file that is not a symbolic link, within a size bound.
func readPlainFile(path, what string) ([]byte, error) {
	link, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", what, err)
	}
	if !link.Mode().IsRegular() {
		return nil, fmt.Errorf("%s (%s) is not a plain file", what, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", what, err)
	}
	defer closeQuietly(file)

	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect %s: %w", what, err)
	}
	if !os.SameFile(link, opened) {
		return nil, fmt.Errorf("%s (%s) changed while it was being read", what, path)
	}

	content, err := io.ReadAll(io.LimitReader(file, maxFile+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", what, err)
	}
	if len(content) > maxFile {
		return nil, fmt.Errorf("%s (%s) is larger than %d bytes", what, path, maxFile)
	}

	return content, nil
}

// archive writes the entries as a gzip-compressed tar with nothing of the machine that built it:
// no time, owner, group or name in the gzip header, and a zero time and owner on each member.
func archive(entries []entry) ([]byte, error) {
	var out bytes.Buffer

	zipped, err := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("cannot start the archive: %w", err)
	}
	writer := tar.NewWriter(zipped)

	for _, e := range entries {
		header := &tar.Header{Typeflag: tar.TypeReg, Name: e.name, Mode: e.mode, Size: int64(len(e.content)), Format: tar.FormatPAX}
		if err := writer.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("cannot write %s to the archive: %w", e.name, err)
		}
		if _, err := writer.Write(e.content); err != nil {
			return nil, fmt.Errorf("cannot write %s to the archive: %w", e.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("cannot finish the archive: %w", err)
	}
	if err := zipped.Close(); err != nil {
		return nil, fmt.Errorf("cannot finish the archive: %w", err)
	}

	return out.Bytes(), nil
}

func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close %s: %v\n", file.Name(), err)
	}
}
