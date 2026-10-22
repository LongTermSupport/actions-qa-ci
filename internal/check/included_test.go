package check_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func widenable(name string, covers check.Coverage, selects func(string) bool) check.Lane {
	return check.Lane{
		Name:    name,
		Selects: selects,
		Run:     func(string) ([]finding.Finding, error) { return nil, nil },
		Covers:  covers,
	}
}

func TestWidenMakesALaneAnalyseTheIncludedFilesOfTheKindsItCovers(t *testing.T) {
	never := func(string) bool { return false }
	included := check.Included{
		Workflows: []string{"templates/ci.yml"},
		Actions:   []string{"templates/lib/action.yml"},
	}
	lanes := []check.Lane{
		widenable("workflows", check.Workflows, never),
		widenable("both", check.Workflows|check.Actions, never),
		widenable("neither", 0, never),
	}

	widened := check.Widen(lanes, included)

	want := map[string]map[string]bool{
		"workflows": {"templates/ci.yml": true, "templates/lib/action.yml": false, "templates/other.yml": false},
		"both":      {"templates/ci.yml": true, "templates/lib/action.yml": true, "templates/other.yml": false},
		"neither":   {"templates/ci.yml": false, "templates/lib/action.yml": false, "templates/other.yml": false},
	}
	for _, lane := range widened {
		for file, expected := range want[lane.Name] {
			if got := lane.Selects(file); got != expected {
				t.Errorf("lane %s selects %s = %v; want %v", lane.Name, file, got, expected)
			}
		}
	}
}

func TestWidenKeepsWhatALaneSelectedBefore(t *testing.T) {
	lane := widenable("workflows", check.Workflows, check.IsWorkflow)

	widened := check.Widen([]check.Lane{lane}, check.Included{Workflows: []string{"templates/ci.yml"}})

	if !widened[0].Selects(".github/workflows/ci.yml") || widened[0].Selects("docs/ci.yml") {
		t.Error("a widened lane must keep selecting what it did, and nothing else")
	}
}

func TestWidenLeavesTheLanesItWasGivenAlone(t *testing.T) {
	lane := widenable("workflows", check.Workflows, func(string) bool { return false })

	widened := check.Widen([]check.Lane{lane}, check.Included{Workflows: []string{"templates/ci.yml"}})

	if !widened[0].Selects("templates/ci.yml") || lane.Selects("templates/ci.yml") {
		t.Error("Widen changed the lane it was given")
	}
}

func TestWidenWithNothingIncludedChangesNothing(t *testing.T) {
	lane := widenable("workflows", check.Workflows, check.IsWorkflow)

	widened := check.Widen([]check.Lane{lane}, check.Included{})

	if widened[0].Selects("templates/ci.yml") || !widened[0].Selects(".github/workflows/ci.yml") {
		t.Error("a widened lane with nothing included must select what it did")
	}
}

func TestIncludedFilesListsEveryKindSorted(t *testing.T) {
	included := check.Included{Workflows: []string{"b.yml", "a.yml"}, Actions: []string{"c/action.yml"}}

	got := included.Files()

	if len(got) != 3 || got[0] != "a.yml" || got[1] != "b.yml" || got[2] != "c/action.yml" {
		t.Errorf("files = %q", got)
	}
}

func TestIsDefaultFileKnowsEveryFileDiscoveryAdds(t *testing.T) {
	for _, file := range []string{
		".github/workflows/ci.yml", ".github/workflows/x.yaml", "action.yml", "action.yaml",
		".github/actions/a/action.yml", ".github/dependabot.yml", ".github/dependabot.yaml",
		".github/actions-qa-ci.yml", ".github/zizmor.yml", ".pinact.yaml",
	} {
		if !check.IsDefaultFile(file) {
			t.Errorf("%s is analysed by default and IsDefaultFile says it is not", file)
		}
	}
	for _, file := range []string{"templates/ci.yml", ".github/workflows/sub/ci.yml", "sub/action.yml", ".github/other.yml"} {
		if check.IsDefaultFile(file) {
			t.Errorf("%s is not analysed by default and IsDefaultFile says it is", file)
		}
	}
}
