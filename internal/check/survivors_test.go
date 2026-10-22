package check_test

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestDiscoverTakesAWorkflowByItsExtensionAloneAndNothingThatMerelyContainsOne(t *testing.T) {
	tree := fstest.MapFS{
		".github/workflows/.yaml":      file("on: push\n"),
		".github/workflows/.yml":       file("on: push\n"),
		".github/workflows/ci.yml":     file("on: push\n"),
		".github/workflows/ci.yml.bak": file("not a workflow\n"),
		".github/workflows/ciyml":      file("not a workflow\n"),
		".github/workflows/ci.yamlx":   file("not a workflow\n"),
	}

	got, err := check.Discover(tree)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{".github/workflows/.yaml", ".github/workflows/.yml", ".github/workflows/ci.yml"}
	if !slices.Equal(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

// vanishingFS reports a directory it listed as gone when it is walked.
type vanishingFS struct {
	fstest.MapFS
	gone string
}

func (v vanishingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == v.gone {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return v.MapFS.ReadDir(name)
}

func TestDiscoverStopsWhenADirectoryItWasWalkingDisappears(t *testing.T) {
	tree := fstest.MapFS{".github/actions/x/action.yml": file("runs: {}\n")}

	got, err := check.Discover(vanishingFS{tree, ".github/actions/x"})

	if err == nil {
		t.Errorf("Discover = %v; a directory that vanished mid-walk is a file set nobody saw, not an empty one", got)
	}
}

func TestRunRefusesALaneWhoseNameIsOnlySpaces(t *testing.T) {
	run := func(string) ([]finding.Finding, error) { return nil, nil }
	lanes := []check.Lane{{Name: "  ", Selects: func(string) bool { return true }, Run: run}}

	got, err := check.Run(lanes, []string{"a.yml"})

	if err == nil || got != nil {
		t.Errorf("Run = %v, %v", got, err)
	}
}
