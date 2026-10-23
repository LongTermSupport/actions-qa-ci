package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const modulesGoMod = "module example.com/project\n\ngo 1.25\n\nrequire github.com/rhysd/actionlint v1.7.12\n"

// modulesProject is a go.mod requiring actionlint v1.7.12 and a fake module proxy whose latest
// answer is the given one.
type modulesProject struct {
	gomod, changelog string
	server           *httptest.Server
	args             []string
}

func newModulesProject(t *testing.T, version string, age time.Duration) *modulesProject {
	t.Helper()

	p := &modulesProject{}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/github.com/rhysd/actionlint/@latest" {
			http.NotFound(w, r)

			return
		}
		if _, err := w.Write([]byte(`{"Version":"` + version + `","Time":"` + bumpNow.Add(-age).Format(time.RFC3339) + `"}`)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(p.server.Close)

	dir := t.TempDir()
	p.gomod = filepath.Join(dir, "go.mod")
	if err := os.WriteFile(p.gomod, []byte(modulesGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	p.changelog = changelogFile(t, unreleasedChangelog)
	p.args = []string{"--gomod", p.gomod, "--module", "github.com/rhysd/actionlint", "--proxy", p.server.URL, "--cooldown", "72h"}

	return p
}

func (p *modulesProject) run(args ...string) result {
	var stdout, stderr bytes.Buffer
	code := modulesBumpCommand(p.server.Client(), bumpNow, args, &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestModulesBumpSaysWhatToMoveAndRecordsItInTheChangelog(t *testing.T) {
	p := newModulesProject(t, "v1.7.13", 5*24*time.Hour)

	got := p.run(append(p.args, "--changelog", p.changelog)...)

	if got.code != 0 || got.stderr != "" || strings.TrimSpace(got.stdout) != "github.com/rhysd/actionlint v1.7.12 -> v1.7.13" {
		t.Fatalf("got %+v", got)
	}
	written, err := os.ReadFile(p.changelog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "- `github.com/rhysd/actionlint` moves from v1.7.12 to v1.7.13.") {
		t.Errorf("the move is not in the changelog:\n%s", written)
	}
	if kept, err := os.ReadFile(p.gomod); err != nil || string(kept) != modulesGoMod {
		t.Errorf("go.mod is go get's to change, not this command's: %q, %v", kept, err)
	}
}

func TestModulesBumpWithNothingToMoveSaysSoAndWritesNothing(t *testing.T) {
	for name, p := range map[string]*modulesProject{
		"up to date":      newModulesProject(t, "v1.7.12", 40*24*time.Hour),
		"inside cooldown": newModulesProject(t, "v1.7.13", time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			got := p.run(append(p.args, "--changelog", p.changelog)...)

			written, err := os.ReadFile(p.changelog)
			if err != nil {
				t.Fatal(err)
			}
			if got.code != 0 || strings.Contains(got.stdout, "->") || string(written) != unreleasedChangelog {
				t.Errorf("got %+v, changelog:\n%s", got, written)
			}
			if !strings.Contains(got.stdout, "up to date") && !strings.Contains(got.stdout, "cooldown") {
				t.Errorf("the output says neither that it is up to date nor why it waits: %q", got.stdout)
			}
		})
	}
}

func TestModulesBumpThatCannotRecordTheMoveIsAToolchainFaultAndWritesNothing(t *testing.T) {
	p := newModulesProject(t, "v1.7.13", 5*24*time.Hour)
	noSection := changelogFile(t, "# Changelog\n\n## 1.0.0\n\n- x\n")

	got := p.run(append(p.args, "--changelog", noSection)...)

	if kept, err := os.ReadFile(noSection); got.code != 3 || err != nil || string(kept) != "# Changelog\n\n## 1.0.0\n\n- x\n" {
		t.Errorf("got %+v, changelog %q, %v", got, kept, err)
	}
}

func TestModulesBumpThatCannotBeTrustedIsAToolchainFault(t *testing.T) {
	p := newModulesProject(t, "v1.7.11", 40*24*time.Hour)

	got := p.run(p.args...)

	if got.code != 3 || !strings.Contains(got.stderr, "older") {
		t.Errorf("got %+v", got)
	}
}

func TestModulesBumpNeedsItsOptionsAndKnowsItsWords(t *testing.T) {
	p := newModulesProject(t, "v1.7.13", 5*24*time.Hour)

	for name, args := range map[string][]string{
		"no go.mod":           {"--module", "github.com/rhysd/actionlint"},
		"no module":           {"--gomod", p.gomod},
		"an unknown option":   {"--gomod", p.gomod, "--module", "github.com/rhysd/actionlint", "--frobnicate", "1"},
		"a stray argument":    {"--gomod", p.gomod, "--module", "github.com/rhysd/actionlint", "extra"},
		"a bad cooldown":      {"--gomod", p.gomod, "--module", "github.com/rhysd/actionlint", "--cooldown", "three days"},
		"a negative cooldown": {"--gomod", p.gomod, "--module", "github.com/rhysd/actionlint", "--cooldown", "-1h"},
	} {
		if got := p.run(args...); got.code != 2 {
			t.Errorf("%s: got %+v, want exit 2", name, got)
		}
	}
}

func TestModulesBumpOfAGoModItCannotReadIsAToolchainFault(t *testing.T) {
	p := newModulesProject(t, "v1.7.13", 5*24*time.Hour)
	dir := t.TempDir()

	for name, path := range map[string]string{"missing": filepath.Join(dir, "none"), "a directory": dir} {
		got := p.run("--gomod", path, "--module", "github.com/rhysd/actionlint", "--proxy", p.server.URL)

		if got.code != 3 {
			t.Errorf("%s: got %+v, want exit 3", name, got)
		}
	}
}

func TestModulesIsACommandOfTheProgram(t *testing.T) {
	for _, args := range [][]string{{"modules"}, {"modules", "frobnicate"}} {
		got := invoke(args...)

		if got.code != 2 || !strings.Contains(got.stderr, "modules bump") {
			t.Errorf("%v: got %+v", args, got)
		}
	}
}
