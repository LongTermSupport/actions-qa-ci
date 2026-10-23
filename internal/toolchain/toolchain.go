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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"unicode"
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

// Pinned returns the release the manifest compiled into the binary pins for a tool, so that a
// version is written in the manifest and nowhere else. A test loads the manifest, so one that
// does not load, or a tool it does not pin, is a defect of the build and not of the run: it
// stops the program with the reason, rather than answering with a version nobody pinned.
func Pinned(tool string) string {
	manifest, err := Load()
	if err != nil {
		panic(fmt.Sprintf("the embedded toolchain manifest does not load: %v", err))
	}
	pinned, ok := manifest.Tools[tool]
	if !ok {
		panic(fmt.Sprintf("the embedded toolchain manifest does not pin %s", tool))
	}

	return pinned.Version
}

// Parse reads a manifest and refuses anything that is not exactly well-formed: a key written
// twice or in the wrong case, an unknown field, data after the end, a tool or platform name
// that is not one the toolchain uses, a pin that is not a lower-case sha256, a download that
// is not https, or a member path that could leave the extraction directory.
func Parse(data []byte) (Manifest, error) {
	if err := checkKeys(data); err != nil {
		return Manifest{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("the toolchain manifest is not valid: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
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

// allowedKeys are the field names each object depth admits; a depth with no entry holds names
// the data chooses (tools, platforms), which validate checks instead.
var allowedKeys = map[int][]string{
	0: {"tools"},
	2: {"version", "assets"},
	4: {"url", "archiveSha256", "member", "binarySha256"},
}

// checkKeys walks the manifest and refuses a key that appears twice in one object or that is
// not exactly a field name: encoding/json would take the last duplicate and match names
// without regard to case, so a reviewer would read one pin and the loader would use another.
func checkKeys(data []byte) error {
	type frame struct {
		isObject  bool
		expectKey bool
		seen      map[string]bool
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var stack []frame

	valueDone := func() {
		if len(stack) > 0 && stack[len(stack)-1].isObject {
			stack[len(stack)-1].expectKey = true
		}
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the toolchain manifest is not valid: %w", err)
		}

		if delim, isDelim := token.(json.Delim); isDelim {
			switch delim {
			case '{':
				stack = append(stack, frame{isObject: true, expectKey: true, seen: map[string]bool{}})
			case '[':
				stack = append(stack, frame{})
			default:
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					// Whatever follows the manifest is Parse's to refuse.
					return nil
				}
				valueDone()
			}

			continue
		}

		top := len(stack) - 1
		if top < 0 || !stack[top].isObject || !stack[top].expectKey {
			valueDone()

			continue
		}

		key, isString := token.(string)
		if !isString {
			return fmt.Errorf("the toolchain manifest is not valid: unexpected key %v", token)
		}
		if stack[top].seen[key] {
			return fmt.Errorf("the toolchain manifest has the key %q twice in one object", key)
		}
		stack[top].seen[key] = true
		if allowed, restricted := allowedKeys[top]; restricted && !slices.Contains(allowed, key) {
			return fmt.Errorf("the toolchain manifest has the key %q where only %s are allowed", key, strings.Join(allowed, ", "))
		}
		stack[top].expectKey = false
	}
}

var (
	toolName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

	// platforms are the targets the shipped bundle is built for.
	platforms = []string{"linux_amd64", "linux_arm64"}
)

func (t Tool) validate(name string) error {
	if !toolName.MatchString(name) {
		return fmt.Errorf("tool name %q must be lower-case letters, digits and hyphens, starting with a letter", name)
	}
	if strings.TrimSpace(t.Version) == "" {
		return fmt.Errorf("tool %s has no version", name)
	}
	if len(t.Assets) == 0 {
		return fmt.Errorf("tool %s has no asset", name)
	}

	for platform, asset := range t.Assets {
		where := name + " " + platform
		if !slices.Contains(platforms, platform) {
			return fmt.Errorf("%s: %q is not a platform the toolchain ships for (%s)", name, platform, strings.Join(platforms, ", "))
		}
		if err := asset.validate(where); err != nil {
			return err
		}
	}

	return nil
}

func (a Asset) validate(where string) error {
	address, err := url.Parse(a.URL)
	switch {
	case err != nil || address.Scheme != "https":
		return fmt.Errorf("%s: the download must be an https URL, got %q", where, a.URL)
	case address.Host == "":
		return fmt.Errorf("%s: the download URL %q has no host", where, a.URL)
	case address.User != nil:
		return fmt.Errorf("%s: the download URL must not carry credentials", where)
	case !sha256Hex.MatchString(a.ArchiveSHA256):
		return fmt.Errorf("%s: archiveSha256 must be 64 lower-case hex digits", where)
	case !sha256Hex.MatchString(a.BinarySHA256):
		return fmt.Errorf("%s: binarySha256 must be 64 lower-case hex digits", where)
	case !safeMember(a.Member):
		return fmt.Errorf("%s: member %q must be a relative path inside the archive", where, a.Member)
	}

	return nil
}

func safeMember(member string) bool {
	if member == "" || member == "." || path.IsAbs(member) || strings.ContainsRune(member, '\\') ||
		strings.ContainsFunc(member, unicode.IsControl) {
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

// Verify checks that the file at binary is a regular file, not a symbolic link, whose content
// is the pinned one. The file is opened before it is inspected, so what is hashed is what was
// inspected. It does not make the path safe to run later: the directory holding it must be one
// no other user can write to.
func Verify(binary string, asset Asset) error {
	link, err := os.Lstat(binary)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", binary, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", binary, err)
	}
	if link.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link; the pinned binary must be the file itself", binary)
	}
	if !link.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", binary)
	}

	// The kernel's answer for this user, which is the one actionlint's lookup of the executable
	// gets: a mode with an execute bit that is not this user's does not make it runnable, and
	// actionlint answers an executable it cannot find by dropping its ShellCheck rule.
	if err := syscall.Access(binary, 1); err != nil {
		return fmt.Errorf("%s is not executable by this user: %w", binary, err)
	}

	// O_NONBLOCK keeps an entry swapped for a FIFO after the check above from blocking the open.
	file, err := os.OpenFile(binary, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", binary, err)
	}
	defer closeQuietly(file)

	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", binary, err)
	}
	if !opened.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", binary)
	}
	if !os.SameFile(link, opened) {
		return fmt.Errorf("%s changed while it was being verified", binary)
	}

	hash := sha256.New()
	if written, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("cannot read %s after %d bytes: %w", binary, written, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != asset.BinarySHA256 {
		return fmt.Errorf("%s %w: it is %s and the manifest pins %s", binary, ErrMismatch, got, asset.BinarySHA256)
	}

	return nil
}

// Resolve finds the executable for a tool in the first directory that holds an entry of its
// name and verifies it. Every directory must be absolute. An entry that fails verification ends
// the search: a later directory is never consulted to cover for a tampered binary, and PATH is
// never consulted at all.
func Resolve(m Manifest, tool, platform string, dirs []string) (string, error) {
	pinned, asset, err := m.Lookup(tool, platform)
	if err != nil {
		return "", err
	}
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("the directory %q to look for %s %s in is not absolute", dir, tool, pinned.Version)
		}
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

	return "", fmt.Errorf("%s %s %w in %s", tool, pinned.Version, ErrNotFound, strings.Join(dirs, ", "))
}

func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close %s: %v\n", file.Name(), err)
	}
}
