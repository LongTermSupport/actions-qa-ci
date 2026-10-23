package bundle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/bundle"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// order is the member names of an archive in the order they were written.
func order(t *testing.T, archive []byte) []string {
	t.Helper()

	unzipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
}

func TestTheMembersAreInOneFixedOrderWhateverOrderTheManifestIsRead(t *testing.T) {
	options := inputs(t)
	for _, name := range []string{"zizmor", "pinact", "aaa"} {
		if err := os.WriteFile(filepath.Join(options.ToolDirs[0], name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(options.Licences, name+".txt"), []byte("licence of "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		options.Manifest.Tools[name] = toolchain.Tool{Version: "v1", Assets: map[string]toolchain.Asset{"linux_amd64": {BinarySHA256: sum(name)}}}
	}

	for range 20 {
		built, err := bundle.Build(options)
		if err != nil {
			t.Fatal(err)
		}

		want := []string{
			"bin/actions-qa", "bin/aaa", "bin/pinact", "bin/shellcheck", "bin/zizmor",
			"LICENSES/aaa.txt", "LICENSES/actions-qa-ci.txt", "LICENSES/pinact.txt", "LICENSES/shellcheck.txt", "LICENSES/zizmor.txt",
		}
		if got := order(t, built.Data); !slices.Equal(got, want) {
			t.Fatalf("members %v, want %v", got, want)
		}
	}
}

func TestTheArchiveCarriesNoTimeNameOrCommentOfTheBuilder(t *testing.T) {
	built, err := bundle.Build(inputs(t))
	if err != nil {
		t.Fatal(err)
	}

	// The gzip header is ten bytes: magic, method, flags, four of modification time, extra
	// flags and the operating system. No flag means no name, comment or extra field.
	header := built.Data[:10]
	if header[3] != 0 || !bytes.Equal(header[4:8], []byte{0, 0, 0, 0}) {
		t.Errorf("gzip header % x holds flags or a time", header)
	}
	for name, m := range members(t, built.Data) {
		h := m.header
		if !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() || len(h.PAXRecords) != 0 || h.Linkname != "" || h.Typeflag != tar.TypeReg {
			t.Errorf("%s carries more than a name, mode and size: %+v", name, h)
		}
	}
}

func TestAVersionIsExactlyThreeNumbers(t *testing.T) {
	for _, version := range []string{"01.2.3", "1.02.3", "1.2.3-rc1", "1.2.3\n", " 1.2.3", "1.2.3.4", "1.2.x", "١.٢.٣"} {
		options := inputs(t)
		options.Version = version

		if built, err := bundle.Build(options); err == nil {
			t.Errorf("built %q for the version %q", built.Name, version)
		}
	}
}

func TestAProgramThatIsNotAPlainFileIsRefusedWithoutBlocking(t *testing.T) {
	options := inputs(t)
	fifo := filepath.Join(filepath.Dir(options.Program), "fifo")
	if err := syscall.Mkfifo(fifo, 0o755); err != nil {
		t.Fatal(err)
	}
	options.Program = fifo

	if _, err := bundle.Build(options); err == nil || !strings.Contains(err.Error(), "plain file") {
		t.Errorf("error %v, want a refusal that names a plain file", err)
	}
}

func TestALicenceNameThatCouldBeMisreadIsRefused(t *testing.T) {
	for _, name := range []string{"a\nb.txt", "a\\b.txt", "a\x01b.txt", "\xff.txt", ".hidden.txt", "a b.txt", "-flag.txt"} {
		t.Run(strings.ToValidUTF8(name, "?"), func(t *testing.T) {
			options := inputs(t)
			if err := os.WriteFile(filepath.Join(options.Licences, name), []byte("x"), 0o644); err != nil {
				t.Skipf("this file system cannot hold the name: %v", err)
			}

			if built, err := bundle.Build(options); err == nil {
				t.Errorf("built %q with a licence file named %q", built.Name, name)
			}
		})
	}
}

func TestAToolWhoseNameIsTheProgramsOrNotAPlainNameIsRefused(t *testing.T) {
	for _, name := range []string{"actions-qa", "../x", "a/b", "", ".hidden", "a b"} {
		options := inputs(t)
		options.Manifest.Tools[name] = toolchain.Tool{Version: "v1", Assets: map[string]toolchain.Asset{"linux_amd64": {BinarySHA256: sum(name)}}}

		built, err := bundle.Build(options)

		if err == nil || !strings.Contains(err.Error(), "pins a tool named") {
			t.Errorf("built %q, error %v, for a tool named %q; the refusal must be of the name, not of a missing file", built.Name, err, name)
		}
	}
}

func TestATooLongNameOrAnUnanchoredPlatformIsRefused(t *testing.T) {
	long := inputs(t)
	if err := os.WriteFile(filepath.Join(long.Licences, strings.Repeat("a", 101)+".txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if built, err := bundle.Build(long); err == nil || !strings.Contains(err.Error(), "not plain letters") {
		t.Errorf("built %q, error %v; a licence name of 105 characters", built.Name, err)
	}

	for _, platform := range []string{" linux_amd64", "linux_amd64\n", "linux_amd64 ", "Linux_amd64"} {
		options := inputs(t)
		options.Platform = platform

		if built, err := bundle.Build(options); err == nil || !strings.Contains(err.Error(), "must be of the form") {
			t.Errorf("built %q, error %v, for the platform %q", built.Name, err, platform)
		}
	}
}

func TestALinkInPlaceOfAToolOrALicenceIsRefusedEvenWhenItLeadsToTheRightBytes(t *testing.T) {
	linked := map[string]func(*testing.T, bundle.Options) string{
		"a tool": func(t *testing.T, o bundle.Options) string {
			t.Helper()

			return filepath.Join(o.ToolDirs[0], "shellcheck")
		},
		"a licence": func(t *testing.T, o bundle.Options) string {
			t.Helper()

			return filepath.Join(o.Licences, "shellcheck.txt")
		},
	}
	for name, place := range linked {
		t.Run(name, func(t *testing.T) {
			options := inputs(t)
			target := place(t, options)
			content, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			real := filepath.Join(t.TempDir(), "real")
			if err := os.WriteFile(real, content, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, target); err != nil {
				t.Fatal(err)
			}

			if built, err := bundle.Build(options); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Errorf("built %q, error %v; a link is not a plain file", built.Name, err)
			}
		})
	}
}

func TestOnlyTheVerifiedBytesAreBundled(t *testing.T) {
	options := inputs(t)
	asset := options.Manifest.Tools["shellcheck"].Assets["linux_amd64"]
	asset.BinarySHA256 = sum("what the pin says")
	options.Manifest.Tools["shellcheck"] = toolchain.Tool{Version: "v0.11.0", Assets: map[string]toolchain.Asset{"linux_amd64": asset}}

	built, err := bundle.Build(options)

	if !errors.Is(err, toolchain.ErrMismatch) || len(built.Data) != 0 {
		t.Errorf("built %q, error %v; the bytes that are bundled must be the ones that were checked against the pin", built.Name, err)
	}
}

func TestAToolDirectoryThatIsNotAbsoluteIsRefused(t *testing.T) {
	options := inputs(t)
	options.ToolDirs = []string{"tools"}

	if _, err := bundle.Build(options); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error %v, want a refusal that names absolute", err)
	}
}

func TestAToolIsTakenFromTheFirstDirectoryThatHoldsItAndNeverFromALaterOne(t *testing.T) {
	options := inputs(t)
	earlier := t.TempDir()
	if err := os.WriteFile(filepath.Join(earlier, "shellcheck"), []byte("a tampered one"), 0o755); err != nil {
		t.Fatal(err)
	}
	options.ToolDirs = []string{earlier, options.ToolDirs[0]}

	built, err := bundle.Build(options)

	if !errors.Is(err, toolchain.ErrMismatch) || len(built.Data) != 0 {
		t.Errorf("built %q, error %v; a tampered tool must not be covered for by a later directory", built.Name, err)
	}
}
