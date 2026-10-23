package actionsqaci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type workflowStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

type workflowFile struct {
	Jobs map[string]struct {
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

// The update job runs the newly pinned binaries (the fetch, the tests, the check) and then
// proposes the manifest it holds. What is proposed has to be copied before any of them runs, or
// a binary that edits the working tree decides what is merged.
func TestTheUpdateWorkflowKeepsTheProposalBeforeItRunsAnyNewlyPinnedBinary(t *testing.T) {
	content, err := os.ReadFile(".github/workflows/update-tools.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}

	steps := workflow.Jobs["bump"].Steps
	kept := -1
	for i, step := range steps {
		if step.Name == "Keep the proposal" {
			kept = i
		}
	}
	if kept < 0 {
		t.Fatal("the bump job has no step named \"Keep the proposal\"")
	}

	runsNewBinary := []string{"actions-qa-dev fetch", "go test", "cmd/actions-qa check"}
	for i := 0; i < kept; i++ {
		for _, command := range runsNewBinary {
			if strings.Contains(steps[i].Run, command) {
				t.Errorf("step %q runs %q before the proposal is kept", steps[i].Name, command)
			}
		}
	}
}

// stepRun is the script of the named step of a job of the update workflow.
func stepRun(t *testing.T, job, name string) string {
	t.Helper()

	content, err := os.ReadFile(".github/workflows/update-tools.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, step := range workflow.Jobs[job].Steps {
		if step.Name == name {
			return step.Run
		}
	}
	t.Fatalf("the %s job has no step named %q", job, name)

	return ""
}

const libraryStep = "Move the actionlint library when a release is past the cooldown"

// The library move happens before the workflow decides whether anything moved and before the
// proposal is kept, or a move of the library alone would be neither noticed nor proposed.
func TestTheLibraryMoveComesBeforeTheWorkflowDecidesWhatMoved(t *testing.T) {
	content, err := os.ReadFile(".github/workflows/update-tools.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}

	position := map[string]int{}
	for i, step := range workflow.Jobs["bump"].Steps {
		position[step.Name] = i
	}
	library, ok := position[libraryStep]
	if !ok {
		t.Fatalf("the bump job has no step named %q", libraryStep)
	}
	for _, later := range []string{"Say what moved", "Keep the proposal"} {
		if position[later] <= library {
			t.Errorf("%q must come after %q", later, libraryStep)
		}
	}
}

// runLibraryStep runs the library step's script the way the runner does (bash -e), with a go that
// answers `run` with the given line and records what it is asked to `get`. It returns what go
// was asked to get, and the report the proposal is made from after the step.
func runLibraryStep(t *testing.T, said string) (asked, report string) {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	bin := t.TempDir()
	goShim := "#!/bin/sh\ncase \"$1\" in\nrun) echo '" + said + "' ;;\nget) echo \"$2\" >> \"" + filepath.Join(temp, "asked") + "\" ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(goShim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "bump-report.txt"), []byte("shellcheck v0.11.0 -> v0.12.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(bash, "-e", "-c", stepRun(t, "bump", libraryStep))
	command.Env = []string{"RUNNER_TEMP=" + temp, "PATH=" + bin + ":" + os.Getenv("PATH")}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("the step failed: %v\n%s", err, output)
	}

	if got, err := os.ReadFile(filepath.Join(temp, "asked")); err == nil {
		asked = string(got)
	}
	written, err := os.ReadFile(filepath.Join(temp, "bump-report.txt"))
	if err != nil {
		t.Fatal(err)
	}

	return asked, string(written)
}

func TestTheLibraryStepAsksGoGetForExactlyTheMoveTheCommandSays(t *testing.T) {
	asked, report := runLibraryStep(t, "github.com/rhysd/actionlint v1.7.12 -> v1.7.13")

	if asked != "github.com/rhysd/actionlint@v1.7.13\n" {
		t.Errorf("go was asked to get %q", asked)
	}
	if !strings.Contains(report, "github.com/rhysd/actionlint v1.7.12 -> v1.7.13") || !strings.Contains(report, "shellcheck v0.11.0 -> v0.12.0") {
		t.Errorf("the proposal's report lost a move:\n%s", report)
	}
}

func TestTheLibraryStepAsksGoGetForNothingWhenThereIsNoMove(t *testing.T) {
	for name, said := range map[string]string{
		"up to date":      "github.com/rhysd/actionlint is up to date",
		"inside cooldown": "github.com/rhysd/actionlint v1.7.13 is 24h0m0s old, inside the 72h0m0s cooldown; v1.7.12 stays required",
	} {
		t.Run(name, func(t *testing.T) {
			asked, report := runLibraryStep(t, said)

			if asked != "" || report != "shellcheck v0.11.0 -> v0.12.0\n" {
				t.Errorf("go was asked to get %q; the report is %q", asked, report)
			}
		})
	}
}

// A tool move is recorded in the changelog by the same command that moves it, so the changelog
// has to travel with the manifest: kept before the new binaries run, and committed with it.
func TestTheUpdateWorkflowProposesTheChangelogEntryWithTheManifest(t *testing.T) {
	steps := map[string][2]string{
		"the bump records the move":      {"Move the tools that have a release past the cooldown", "--changelog CHANGELOG.md"},
		"the proposal keeps the file":    {"Keep the proposal", `cp CHANGELOG.md "$RUNNER_TEMP/proposal/CHANGELOG.md"`},
		"the proposal restores the file": {"Commit the proposal to chore/update-tools", `cp "$RUNNER_TEMP/proposal/CHANGELOG.md" CHANGELOG.md`},
		"the proposal commits the file":  {"Commit the proposal to chore/update-tools", "git add internal/toolchain/manifest.json go.mod go.sum CHANGELOG.md"},
		"the library move is asked for":  {libraryStep, "modules bump --gomod go.mod --module github.com/rhysd/actionlint --changelog CHANGELOG.md"},
		"the library is moved by go get": {libraryStep, `go get "$target"`},
		"the move is noticed in go.mod":  {"Say what moved", "git diff --quiet -- internal/toolchain/manifest.json go.mod"},
		"the proposal keeps go.mod":      {"Keep the proposal", `cp go.mod "$RUNNER_TEMP/proposal/go.mod"`},
		"the proposal keeps go.sum":      {"Keep the proposal", `cp go.sum "$RUNNER_TEMP/proposal/go.sum"`},
		"the proposal restores go.mod":   {"Commit the proposal to chore/update-tools", `cp "$RUNNER_TEMP/proposal/go.mod" go.mod`},
		"the proposal restores go.sum":   {"Commit the proposal to chore/update-tools", `cp "$RUNNER_TEMP/proposal/go.sum" go.sum`},
	}

	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			job := "bump"
			if strings.HasPrefix(step[0], "Commit") {
				job = "propose"
			}

			if run := stepRun(t, job, step[0]); !strings.Contains(run, step[1]) {
				t.Errorf("step %q does not contain %q:\n%s", step[0], step[1], run)
			}
		})
	}
}
