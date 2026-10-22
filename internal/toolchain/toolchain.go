// Package toolchain pins the third-party binaries the toolchain runs, finds them on disk and
// refuses any whose content is not the pinned one.
package toolchain

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// ErrMismatch marks a binary whose content is not the one the manifest pins.
var ErrMismatch = errors.New("does not match the pinned checksum")

// ErrNotFound marks a binary that is absent.
var ErrNotFound = errors.New("not found")

//go:embed manifest.json
var embedded []byte

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Asset is one release download for one platform.
type Asset struct {
	// URL is the release archive.
	URL string `json:"url"`
	// ArchiveSHA256 pins the archive as downloaded.
	ArchiveSHA256 string `json:"archiveSha256"`
	// Member is the archive path of the executable.
	Member string `json:"member"`
	// BinarySHA256 pins the executable as it sits on disk after extraction.
	BinarySHA256 string `json:"binarySha256"`
}

// Tool is one pinned release.
type Tool struct {
	Version string `json:"version"`
	// Assets are keyed by platform, in the form GOOS_GOARCH.
	Assets map[string]Asset `json:"assets"`
}

// Manifest is every pinned tool, keyed by name.
type Manifest struct {
	Tools map[string]Tool `json:"tools"`
}

// Platform names the platform this binary runs on, in the form manifests key their assets by.
func Platform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// Load reads the manifest compiled into the binary.
func Load() (Manifest, error) {
	return Parse(embedded)
}

// Parse reads a manifest and refuses anything that is not exactly well-formed: an unknown field,
// trailing data, a pin that is not a lower-case sha256, a download that is not https or a
// member path that could leave the extraction directory.
func Parse(data []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("the toolchain manifest is not valid: %w", err)
	}
	if decoder.More() {
		return Manifest{}, errors.New("the toolchain manifest has data after its end")
	}
	if len(manifest.Tools) == 0 {
		return Manifest{}, errors.New("the toolchain manifest pins no tool")
	}

	for name, tool := range manifest.Tools {
		if err := tool.validate(name); err != nil {
			return Manifest{}, err
		}
	}

	return manifest, nil
}

func (t Tool) validate(name string) error {
	if t.Version == "" {
		return fmt.Errorf("tool %s has no version", name)
	}
	if len(t.Assets) == 0 {
		return fmt.Errorf("tool %s has no asset", name)
	}

	for platform, asset := range t.Assets {
		where := name + " " + platform
		switch {
		case !strings.HasPrefix(asset.URL, "https://"):
			return fmt.Errorf("%s: the download must be an https URL, got %q", where, asset.URL)
		case !sha256Hex.MatchString(asset.ArchiveSHA256):
			return fmt.Errorf("%s: archiveSha256 must be 64 lower-case hex digits", where)
		case !sha256Hex.MatchString(asset.BinarySHA256):
			return fmt.Errorf("%s: binarySha256 must be 64 lower-case hex digits", where)
		case !safeMember(asset.Member):
			return fmt.Errorf("%s: member %q must be a relative path inside the archive", where, asset.Member)
		}
	}

	return nil
}

func safeMember(member string) bool {
	if member == "" || path.IsAbs(member) || strings.ContainsRune(member, '\\') {
		return false
	}

	return !slices.Contains(strings.Split(member, "/"), "..") && path.Clean(member) == member
}

// Lookup returns the pinned release of a tool and its asset for a platform. The error names
// whatever is not pinned.
func (m Manifest) Lookup(tool, platform string) (Tool, Asset, error) {
	pinned, ok := m.Tools[tool]
	if !ok {
		return Tool{}, Asset{}, fmt.Errorf("the toolchain manifest does not pin %s", tool)
	}
	asset, ok := pinned.Assets[platform]
	if !ok {
		return Tool{}, Asset{}, fmt.Errorf("the toolchain manifest does not pin %s %s for %s", tool, pinned.Version, platform)
	}

	return pinned, asset, nil
}

// Verify checks that the file at binary is a regular file whose content is the pinned one.
func Verify(binary string, asset Asset) error {
	info, err := os.Stat(binary)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", binary, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", binary, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", binary)
	}

	file, err := os.Open(binary)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", binary, err)
	}
	defer closeQuietly(file)

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("cannot read %s: %w", binary, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != asset.BinarySHA256 {
		return fmt.Errorf("%s %w: it is %s and the manifest pins %s", binary, ErrMismatch, got, asset.BinarySHA256)
	}

	return nil
}

// Resolve finds the executable for a tool in the first directory that holds a file of its name
// and verifies it. A file that fails verification ends the search: a later directory is never
// consulted to cover for a tampered binary, and PATH is never consulted at all.
func Resolve(m Manifest, tool, platform string, dirs []string) (string, error) {
	_, asset, err := m.Lookup(tool, platform)
	if err != nil {
		return "", err
	}

	for _, dir := range dirs {
		candidate := filepath.Join(dir, tool)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("cannot inspect %s: %w", candidate, err)
		}
		if err := Verify(candidate, asset); err != nil {
			return "", err
		}

		return candidate, nil
	}

	return "", fmt.Errorf("%s %w in %s", tool, ErrNotFound, strings.Join(dirs, ", "))
}

func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close %s: %v\n", file.Name(), err)
	}
}
