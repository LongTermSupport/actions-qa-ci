package actionsqaci_test

import (
	"os"
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

// A tool move is recorded in the changelog by the same command that moves it, so the changelog
// has to travel with the manifest: kept before the new binaries run, and committed with it.
func TestTheUpdateWorkflowProposesTheChangelogEntryWithTheManifest(t *testing.T) {
	steps := map[string][2]string{
		"the bump records the move":      {"Move the tools that have a release past the cooldown", "--changelog CHANGELOG.md"},
		"the proposal keeps the file":    {"Keep the proposal", `cp CHANGELOG.md "$RUNNER_TEMP/proposal/CHANGELOG.md"`},
		"the proposal restores the file": {"Commit the proposal to chore/update-tools", `cp "$RUNNER_TEMP/proposal/CHANGELOG.md" CHANGELOG.md`},
		"the proposal commits the file":  {"Commit the proposal to chore/update-tools", "git add internal/toolchain/manifest.json CHANGELOG.md"},
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
