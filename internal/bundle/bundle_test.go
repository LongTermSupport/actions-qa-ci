package bundle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/bundle"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const (
	programContent = "the actions-qa program"
	toolContent    = "the pinned shellcheck"
)

func sum(content string) string {
	digest := sha256.Sum256([]byte(content))

	return hex.EncodeToString(digest[:])
}

type member struct {
	mode    int64
	content string
	header  tar.Header
}

// inputs is a program, a pinned tool in a tools directory and a licence directory, with the
// options that build a bundle from them.
func inputs(t *testing.T) bundle.Options {
	t.Helper()

	dir := t.TempDir()
	write := func(path, content string, mode os.FileMode) string {
		t.Helper()

		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(full, mode); err != nil {
			t.Fatal(err)
		}

		return full
	}

	program := write("build/actions-qa", programContent, 0o755)
	write("tools/shellcheck", toolContent, 0o755)
	write("licences/actions-qa-ci.txt", "the project licence", 0o644)
	write("licences/shellcheck.txt", "the shellcheck licence", 0o600)

	return bundle.Options{
		Manifest: toolchain.Manifest{Tools: map[string]toolchain.Tool{
			"shellcheck": {Version: "v0.11.0", Assets: map[string]toolchain.Asset{
				"linux_amd64": {BinarySHA256: sum(toolContent)},
			}},
		}},
		Platform: "linux_amd64",
		Version:  "1.2.3",
		Program:  program,
		ToolDirs: []string{filepath.Join(dir, "tools")},
		Licences: filepath.Join(dir, "licences"),
	}
}

func members(t *testing.T, archive []byte) map[string]member {
	t.Helper()

	unzipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]member{}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		found[header.Name] = member{mode: header.Mode, content: string(content), header: *header}
	}

	return found
}

func TestABundleHoldsTheProgramTheToolsAndTheLicences(t *testing.T) {
	built, err := bundle.Build(inputs(t))
	if err != nil {
		t.Fatal(err)
	}

	if built.Name != "actions-qa-ci_1.2.3_linux_amd64.tar.gz" {
		t.Errorf("name = %q", built.Name)
	}
	got := members(t, built.Data)
	want := map[string]member{
		"bin/actions-qa":             {mode: 0o755, content: programContent},
		"bin/shellcheck":             {mode: 0o755, content: toolContent},
		"LICENSES/actions-qa-ci.txt": {mode: 0o644, content: "the project licence"},
		"LICENSES/shellcheck.txt":    {mode: 0o644, content: "the shellcheck licence"},
	}
	if len(got) != len(want) {
		t.Errorf("members = %d, want %d", len(got), len(want))
	}
	for name, expected := range want {
		actual, ok := got[name]
		if !ok {
			t.Errorf("%s is not in the bundle", name)

			continue
		}
		if actual.mode != expected.mode || actual.content != expected.content {
			t.Errorf("%s: mode %o content %q, want mode %o content %q", name, actual.mode, actual.content, expected.mode, expected.content)
		}
	}
}

func TestABundleIsTheSameBytesEveryTimeAndCarriesNothingOfTheBuilder(t *testing.T) {
	options := inputs(t)

	first, err := bundle.Build(options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bundle.Build(options)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first.Data, second.Data) || first.SHA256 != second.SHA256 {
		t.Error("two builds of the same inputs differ")
	}
	if first.SHA256 != sum(string(first.Data)) {
		t.Errorf("SHA256 = %s, the archive is %s", first.SHA256, sum(string(first.Data)))
	}
	for name, m := range members(t, first.Data) {
		h := m.header
		if !h.ModTime.IsZero() && h.ModTime.Unix() != 0 || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" {
			t.Errorf("%s carries the builder: mtime %v uid %d gid %d user %q group %q", name, h.ModTime, h.Uid, h.Gid, h.Uname, h.Gname)
		}
	}
}

func TestABuildRefusesWhatCannotBeShipped(t *testing.T) {
	tamper := func(t *testing.T, options bundle.Options) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(options.ToolDirs[0], "shellcheck"), []byte("swapped"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]struct {
		change func(*testing.T, *bundle.Options)
		want   string
		is     error
	}{
		"a version with a v":           {func(t *testing.T, o *bundle.Options) { o.Version = "v1.2.3" }, "version", nil},
		"a version of two parts":       {func(t *testing.T, o *bundle.Options) { o.Version = "1.2" }, "version", nil},
		"no version":                   {func(t *testing.T, o *bundle.Options) { o.Version = "" }, "version", nil},
		"a platform nothing is pinned": {func(t *testing.T, o *bundle.Options) { o.Platform = "linux_riscv64" }, "linux_riscv64", nil},
		"a platform with a slash":      {func(t *testing.T, o *bundle.Options) { o.Platform = "../x" }, "platform", nil},
		"a swapped tool":               {func(t *testing.T, o *bundle.Options) { tamper(t, *o) }, "shellcheck", toolchain.ErrMismatch},
		"a missing tool": {func(t *testing.T, o *bundle.Options) {
			if err := os.Remove(filepath.Join(o.ToolDirs[0], "shellcheck")); err != nil {
				t.Fatal(err)
			}
		}, "shellcheck", toolchain.ErrNotFound},
		"a tool without a licence": {func(t *testing.T, o *bundle.Options) {
			if err := os.Remove(filepath.Join(o.Licences, "shellcheck.txt")); err != nil {
				t.Fatal(err)
			}
		}, "shellcheck.txt", nil},
		"no licence for the project": {func(t *testing.T, o *bundle.Options) {
			if err := os.Remove(filepath.Join(o.Licences, "actions-qa-ci.txt")); err != nil {
				t.Fatal(err)
			}
		}, "actions-qa-ci.txt", nil},
		"a program that is not a file": {func(t *testing.T, o *bundle.Options) { o.Program = filepath.Dir(o.Program) }, "program", nil},
		"a program that is a link": {func(t *testing.T, o *bundle.Options) {
			link := filepath.Join(filepath.Dir(o.Program), "link")
			if err := os.Symlink(o.Program, link); err != nil {
				t.Fatal(err)
			}
			o.Program = link
		}, "link", nil},
		"a licence directory that is missing": {func(t *testing.T, o *bundle.Options) { o.Licences = filepath.Join(o.Licences, "nowhere") }, "licen", nil},
		"a licence that is a directory": {func(t *testing.T, o *bundle.Options) {
			if err := os.Mkdir(filepath.Join(o.Licences, "extra.txt"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "extra.txt", nil},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			options := inputs(t)
			c.change(t, &options)

			built, err := bundle.Build(options)

			if err == nil {
				t.Fatalf("built %s", built.Name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not name %q", err, c.want)
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Errorf("error %q is not %v", err, c.is)
			}
			if len(built.Data) != 0 {
				t.Error("a refused build returned an archive")
			}
		})
	}
}

func TestEveryToolOfThePlatformIsBundledAndNoOtherPlatformsTool(t *testing.T) {
	options := inputs(t)
	options.Manifest.Tools["zizmor"] = toolchain.Tool{Version: "v1", Assets: map[string]toolchain.Asset{
		"linux_arm64": {BinarySHA256: sum("an arm tool")},
	}}

	built, err := bundle.Build(options)

	if err == nil || !strings.Contains(err.Error(), "zizmor") {
		t.Errorf("a tool pinned for another platform only: built %q, error %v; a bundle must not ship without a tool the manifest pins", built.Name, err)
	}
}

func TestTheChecksumsNameEachBundleBySha256InOrder(t *testing.T) {
	text := bundle.Checksums([]bundle.Bundle{
		{Name: "b.tar.gz", SHA256: strings.Repeat("b", 64)},
		{Name: "a.tar.gz", SHA256: strings.Repeat("a", 64)},
	})

	want := strings.Repeat("a", 64) + "  a.tar.gz\n" + strings.Repeat("b", 64) + "  b.tar.gz\n"
	if text != want {
		t.Errorf("checksums = %q, want %q", text, want)
	}
}
