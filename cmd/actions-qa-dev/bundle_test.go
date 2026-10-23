package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))

	return hex.EncodeToString(sum[:])
}

// project is a program, a pinned tool and the licences laid out in a directory, with the
// arguments that build a bundle from them into a directory that does not exist yet.
type project struct {
	manifest toolchain.Manifest
	args     []string
	out      string
}

func release(t *testing.T) project {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"build/actions-qa":           "program",
		"tools/shellcheck":           "tool",
		"licences/actions-qa-ci.txt": "project licence",
		"licences/shellcheck.txt":    "tool licence",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "out")

	return project{
		manifest: toolchain.Manifest{Tools: map[string]toolchain.Tool{
			"shellcheck": {Version: "v1", Assets: map[string]toolchain.Asset{"linux_amd64": {BinarySHA256: digest("tool")}}},
		}},
		args: []string{
			"--version", "1.2.3", "--platform", "linux_amd64",
			"--program", filepath.Join(dir, "build/actions-qa"),
			"--tools", filepath.Join(dir, "tools"),
			"--licences", filepath.Join(dir, "licences"),
			"--out", out,
		},
		out: out,
	}
}

func build(manifest toolchain.Manifest, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := bundleCommand(manifest, args, &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestBundleWritesTheArchiveAndItsChecksumAndNamesBoth(t *testing.T) {
	p := release(t)

	got := build(p.manifest, p.args...)

	if got.code != 0 || got.stderr != "" {
		t.Fatalf("got %+v", got)
	}
	archive, err := os.ReadFile(filepath.Join(p.out, "actions-qa-ci_1.2.3_linux_amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(p.out, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := digest(string(archive)) + "  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n"; string(checksums) != want {
		t.Errorf("checksums.txt = %q, want %q", checksums, want)
	}
	if !strings.Contains(got.stdout, "actions-qa-ci_1.2.3_linux_amd64.tar.gz") || !strings.Contains(got.stdout, "checksums.txt") {
		t.Errorf("stdout %q names neither file", got.stdout)
	}
}

func TestBundleNeedsEveryOptionAndRefusesAnUnknownOne(t *testing.T) {
	p := release(t)

	for i := 0; i < len(p.args); i += 2 {
		without := append(append([]string{}, p.args[:i]...), p.args[i+2:]...)
		got := build(p.manifest, without...)

		if got.code != 2 || !strings.Contains(got.stderr, p.args[i]+" is needed") {
			t.Errorf("without %s: got %+v, want exit 2 naming it", p.args[i], got)
		}
	}
	if got := build(p.manifest, append(append([]string{}, p.args...), "--sign", "yes")...); got.code != 2 {
		t.Errorf("an unknown option: got %+v, want exit 2", got)
	}
	if got := build(p.manifest, append(append([]string{}, p.args...), "extra")...); got.code != 2 {
		t.Errorf("a stray argument: got %+v, want exit 2", got)
	}
}

func TestBundleThatCannotBeBuiltWritesNothingAndIsAToolchainFault(t *testing.T) {
	p := release(t)
	p.manifest.Tools["shellcheck"] = toolchain.Tool{Version: "v1", Assets: map[string]toolchain.Asset{"linux_amd64": {BinarySHA256: digest("another")}}}

	got := build(p.manifest, p.args...)

	if got.code != 3 || !strings.Contains(got.stderr, "shellcheck") {
		t.Errorf("got %+v, want exit 3 naming the tool", got)
	}
	if _, err := os.Stat(p.out); !os.IsNotExist(err) {
		t.Errorf("the output directory exists after a refused build: %v", err)
	}
}

func TestBundleIsACommandOfTheProgramAndWithoutOptionsIsAUsageError(t *testing.T) {
	got := invoke("bundle")

	if got.code != 2 || !strings.Contains(got.stderr, "--version is needed") || !strings.Contains(got.stderr, "actions-qa-dev bundle") {
		t.Errorf("got %+v, want exit 2 and the bundle usage", got)
	}
}

func TestBundleLeavesNoHalfReleaseWhenTheChecksumsAreAlreadyThere(t *testing.T) {
	p := release(t)
	if err := os.MkdirAll(p.out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.out, "checksums.txt"), []byte("an earlier release\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := build(p.manifest, p.args...)

	if got.code != 3 || !strings.Contains(got.stderr, "checksums.txt") {
		t.Errorf("got %+v, want exit 3 naming checksums.txt", got)
	}
	if _, err := os.Stat(filepath.Join(p.out, "actions-qa-ci_1.2.3_linux_amd64.tar.gz")); !os.IsNotExist(err) {
		t.Errorf("the archive was left beside a release that was refused: %v", err)
	}
	if kept, err := os.ReadFile(filepath.Join(p.out, "checksums.txt")); err != nil || string(kept) != "an earlier release\n" {
		t.Errorf("the earlier checksums were changed: %q, %v", kept, err)
	}
}

func TestBundleWillNotWriteThroughASymbolicLinkAsTheOutputDirectory(t *testing.T) {
	p := release(t)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, p.out); err != nil {
		t.Fatal(err)
	}

	got := build(p.manifest, p.args...)

	if got.code != 3 || !strings.Contains(got.stderr, "symbolic link") {
		t.Errorf("got %+v, want exit 3 naming the link", got)
	}
	if entries, err := os.ReadDir(elsewhere); err != nil || len(entries) != 0 {
		t.Errorf("something was written through the link: %v, %v", entries, err)
	}
}

func TestBundleWritesFilesNobodyElseCanChange(t *testing.T) {
	p := release(t)

	got := build(p.manifest, p.args...)

	if got.code != 0 {
		t.Fatalf("got %+v", got)
	}
	for _, name := range []string{"actions-qa-ci_1.2.3_linux_amd64.tar.gz", "checksums.txt"} {
		info, err := os.Stat(filepath.Join(p.out, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o133 != 0 {
			t.Errorf("%s has mode %o; it must not be executable or writable by others", name, info.Mode().Perm())
		}
	}
}

func TestBundleDoesNotOverwriteAReleaseAlreadyThere(t *testing.T) {
	p := release(t)

	first := build(p.manifest, p.args...)
	second := build(p.manifest, p.args...)

	if first.code != 0 || second.code != 3 || !strings.Contains(second.stderr, "exists") {
		t.Errorf("first %+v, second %+v; a second build must not replace the first", first, second)
	}
}
