package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

var bumpNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func shellcheckTarball(t *testing.T, version, content string) []byte {
	t.Helper()

	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	member := "shellcheck-" + version + "/shellcheck"
	if err := writer.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// bumpProject is a manifest file pinning shellcheck v0.11.0 from a fake GitHub whose latest
// release is the given one, with the arguments that bump it.
type bumpProject struct {
	manifest string
	args     []string
	server   *httptest.Server
	tokens   []string
}

func newBumpProject(t *testing.T, latest string) *bumpProject {
	t.Helper()

	p := &bumpProject{}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/koalaman/shellcheck/releases/latest":
			p.tokens = append(p.tokens, r.Header.Get("Authorization"))
			if _, err := w.Write([]byte(latest)); err != nil {
				t.Error(err)
			}
		case strings.Contains(r.URL.Path, "v0.12.0"):
			if _, err := w.Write(shellcheckTarball(t, "v0.12.0", "new "+r.URL.Path)); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)

	asset := func(arch string) toolchain.Asset {
		return toolchain.Asset{
			URL:           p.server.URL + "/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux." + arch + ".tar.gz",
			ArchiveSHA256: strings.Repeat("a", 64),
			Member:        "shellcheck-v0.11.0/shellcheck",
			BinarySHA256:  strings.Repeat("b", 64),
		}
	}
	written, err := bump.Marshal(toolchain.Manifest{Tools: map[string]toolchain.Tool{
		"shellcheck": {Version: "v0.11.0", Assets: map[string]toolchain.Asset{"linux_amd64": asset("x86_64"), "linux_arm64": asset("aarch64")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p.manifest = filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p.manifest, written, 0o640); err != nil {
		t.Fatal(err)
	}
	p.args = []string{"tools", "bump", "--manifest", p.manifest, "--api", p.server.URL, "--cooldown", "72h"}

	return p
}

func (p *bumpProject) run(token string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := toolsBumpCommand(p.server.Client(), token, bumpNow, args[2:], &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func releaseJSON(tag string, age time.Duration) string {
	return `{"tag_name":"` + tag + `","published_at":"` + bumpNow.Add(-age).Format(time.RFC3339) + `","draft":false,"prerelease":false}`
}

func TestToolsBumpRewritesTheManifestAndSaysWhatMoved(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.12.0", 5*24*time.Hour))
	if err := os.Chmod(p.manifest, 0o640); err != nil {
		t.Fatal(err)
	}

	got := p.run("", p.args...)

	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, "shellcheck v0.11.0 -> v0.12.0") {
		t.Fatalf("got %+v", got)
	}
	written, err := os.ReadFile(p.manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := toolchain.Parse(written)
	if err != nil {
		t.Fatalf("the rewritten manifest is not one the toolchain accepts: %v\n%s", err, written)
	}
	if got := manifest.Tools["shellcheck"]; got.Version != "v0.12.0" || got.Assets["linux_arm64"].Member != "shellcheck-v0.12.0/shellcheck" {
		t.Errorf("the manifest was not moved: %+v", got)
	}
	info, err := os.Stat(p.manifest)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Errorf("mode %v, %v; the file's mode must be kept", info, err)
	}
	if entries, err := os.ReadDir(filepath.Dir(p.manifest)); err != nil || len(entries) != 1 {
		t.Errorf("entries %v, %v; no temporary file may be left beside the manifest", entries, err)
	}
}

func TestToolsBumpWithNothingNewLeavesTheFileAloneAndSaysSo(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.11.0", 40*24*time.Hour))
	before, err := os.ReadFile(p.manifest)
	if err != nil {
		t.Fatal(err)
	}

	got := p.run("", p.args...)

	after, readErr := os.ReadFile(p.manifest)
	if got.code != 0 || !strings.Contains(got.stdout, "up to date") || readErr != nil || !bytes.Equal(before, after) {
		t.Errorf("got %+v, read error %v", got, readErr)
	}
}

func TestToolsBumpInsideTheCooldownSaysWhyAndLeavesTheFileAlone(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.12.0", 24*time.Hour))
	before, err := os.ReadFile(p.manifest)
	if err != nil {
		t.Fatal(err)
	}

	got := p.run("", p.args...)

	after, readErr := os.ReadFile(p.manifest)
	if got.code != 0 || !strings.Contains(got.stdout, "cooldown") || readErr != nil || !bytes.Equal(before, after) {
		t.Errorf("got %+v, read error %v", got, readErr)
	}
}

func TestToolsBumpGivesTheTokenToTheReleaseLookupOnly(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.11.0", 40*24*time.Hour))

	got := p.run("a-token", p.args...)

	if got.code != 0 || len(p.tokens) != 1 || p.tokens[0] != "Bearer a-token" {
		t.Errorf("got %+v, tokens %q", got, p.tokens)
	}
}

func TestToolsBumpThatCannotBeTrustedChangesNothingAndIsAToolchainFault(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.10.0", 40*24*time.Hour))
	before, err := os.ReadFile(p.manifest)
	if err != nil {
		t.Fatal(err)
	}

	got := p.run("", p.args...)

	after, readErr := os.ReadFile(p.manifest)
	if got.code != 3 || !strings.Contains(got.stderr, "older") || readErr != nil || !bytes.Equal(before, after) {
		t.Errorf("got %+v, read error %v", got, readErr)
	}
}

func TestToolsBumpNeedsItsOptionsAndKnowsItsWords(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.11.0", 40*24*time.Hour))

	for name, args := range map[string][]string{
		"no manifest":         {"tools", "bump", "--api", p.server.URL},
		"an unknown option":   {"tools", "bump", "--manifest", p.manifest, "--frobnicate", "1"},
		"a stray argument":    {"tools", "bump", "--manifest", p.manifest, "extra"},
		"a bad cooldown":      {"tools", "bump", "--manifest", p.manifest, "--cooldown", "three days"},
		"a negative cooldown": {"tools", "bump", "--manifest", p.manifest, "--cooldown", "-1h"},
	} {
		if got := p.run("", args...); got.code != 2 {
			t.Errorf("%s: got %+v, want exit 2", name, got)
		}
	}
}

func TestToolsBumpOfAManifestItCannotReadIsAToolchainFault(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.11.0", 40*24*time.Hour))
	dir := t.TempDir()
	broken := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"missing": filepath.Join(dir, "none.json"), "invalid": broken, "a directory": dir} {
		got := p.run("", "tools", "bump", "--manifest", path, "--api", p.server.URL)

		if got.code != 3 {
			t.Errorf("%s: got %+v, want exit 3", name, got)
		}
	}
}

const unreleasedChangelog = "# Changelog\n\n## Unreleased\n\n### Added\n\n- A thing.\n"

// changelogFile writes a changelog in a directory of its own and returns its path.
func changelogFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestToolsBumpRecordsTheMoveInTheChangelogItIsGiven(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.12.0", 5*24*time.Hour))
	changelog := changelogFile(t, unreleasedChangelog)

	got := p.run("", append(p.args, "--changelog", changelog)...)

	written, err := os.ReadFile(changelog)
	if err != nil {
		t.Fatal(err)
	}
	if got.code != 0 || !strings.Contains(string(written), "- `shellcheck` moves from v0.11.0 to v0.12.0.") {
		t.Errorf("got %+v, changelog:\n%s", got, written)
	}
	if info, err := os.Stat(changelog); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, %v; the file's mode must be kept", info, err)
	}
	if entries, err := os.ReadDir(filepath.Dir(changelog)); err != nil || len(entries) != 1 {
		t.Errorf("entries %v, %v; no temporary file may be left beside the changelog", entries, err)
	}
}

func TestToolsBumpWithNothingToMoveLeavesTheChangelogAlone(t *testing.T) {
	p := newBumpProject(t, releaseJSON("v0.11.0", 40*24*time.Hour))
	changelog := changelogFile(t, unreleasedChangelog)

	got := p.run("", append(p.args, "--changelog", changelog)...)

	written, err := os.ReadFile(changelog)
	if err != nil {
		t.Fatal(err)
	}
	if got.code != 0 || string(written) != unreleasedChangelog {
		t.Errorf("got %+v, changelog:\n%s", got, written)
	}
}

func TestToolsBumpThatCannotRecordTheMoveChangesNeitherFile(t *testing.T) {
	for name, content := range map[string]string{
		"a changelog with no Unreleased section": "# Changelog\n\n## 1.0.0\n\n- x\n",
		"a changelog that is not there":          "",
	} {
		t.Run(name, func(t *testing.T) {
			p := newBumpProject(t, releaseJSON("v0.12.0", 5*24*time.Hour))
			path := filepath.Join(t.TempDir(), "absent.md")
			if content != "" {
				path = changelogFile(t, content)
			}
			before, err := os.ReadFile(p.manifest)
			if err != nil {
				t.Fatal(err)
			}

			got := p.run("", append(p.args, "--changelog", path)...)

			after, readErr := os.ReadFile(p.manifest)
			if got.code != 3 || readErr != nil || !bytes.Equal(before, after) {
				t.Errorf("got %+v, read error %v; the manifest must not move when the changelog cannot say so", got, readErr)
			}
			if content != "" {
				if kept, err := os.ReadFile(path); err != nil || string(kept) != content {
					t.Errorf("the changelog changed: %q, %v", kept, err)
				}
			}
		})
	}
}

func TestToolsIsACommandOfTheProgram(t *testing.T) {
	for _, args := range [][]string{{"tools"}, {"tools", "frobnicate"}} {
		got := invoke(args...)

		if got.code != 2 || !strings.Contains(got.stderr, "tools bump") {
			t.Errorf("%v: got %+v", args, got)
		}
	}
}
