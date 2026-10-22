package catalogue_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
)

// Whatever Discover offers as a workflow, the actionlint lane must analyse: the two are one
// definition, so a workflow cannot be found and then passed over.
func TestEveryWorkflowDiscoverOffersTheActionlintLaneSelects(t *testing.T) {
	lanes := catalogue.Lanes(func(string) bool { return true })
	var actionlint check.Lane
	for _, lane := range lanes {
		if lane.Name == "actionlint" {
			actionlint = lane
		}
	}

	for _, name := range []string{"ci.yml", "ci.yaml", ".yml", ".yaml", "a.b.yml", "ünï.yaml"} {
		path := ".github/workflows/" + name
		if !check.IsWorkflow(path) || !actionlint.Selects(path) {
			t.Errorf("%s: IsWorkflow = %v, lane selects = %v; both must say yes", path, check.IsWorkflow(path), actionlint.Selects(path))
		}
	}
	for _, path := range []string{
		".github/workflows/ci.yml.bak", ".github/workflows/ciyml", ".github/workflows/ci.YML",
		".github/workflows/sub/ci.yml", "workflows/ci.yml", "x/.github/workflows/ci.yml", ".github/workflows",
	} {
		if check.IsWorkflow(path) || actionlint.Selects(path) {
			t.Errorf("%s: IsWorkflow = %v, lane selects = %v; neither may", path, check.IsWorkflow(path), actionlint.Selects(path))
		}
	}
}

func TestTheRecordLaneSelectsTheRecordAndNothingNearIt(t *testing.T) {
	var selects func(string) bool
	for _, lane := range catalogue.Lanes(func(string) bool { return true }) {
		if lane.Name == "firstParty" {
			selects = lane.Selects
		}
	}

	for path, want := range map[string]bool{
		".github/actions-qa-ci.yml":           true,
		"action.yml":                          false,
		".github/actions/x/action.yml":        false,
		".github/actions-qa-ci.yaml":          false,
		".github/dependabot.yml":              false,
		".github/dependabot.yaml":             false,
		".github/actions-qa-ci.yml.bak":       false,
		"actions-qa-ci.yml":                   false,
		".github/workflows/actions-qa-ci.yml": false,
	} {
		if got := selects(path); got != want {
			t.Errorf("firstParty selects %s = %v, want %v", path, got, want)
		}
	}
}
