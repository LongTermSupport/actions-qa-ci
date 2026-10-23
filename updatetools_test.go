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
