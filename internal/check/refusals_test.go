package check_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestDiscoverRefusesADirectoryNamedLikeAFileOfTheDefaultSetAtAnyDepth(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"under actions":       {".github/actions/x/action.yml/inner.txt": file("x\n")},
		"under actions, yaml": {".github/actions/x/y/action.yaml/inner.txt": file("x\n")},
		"at the root":         {"action.yml/inner.txt": file("x\n")},
		"at the root, yaml":   {"action.yaml/inner.txt": file("x\n")},
		"directly in actions": {".github/actions/action.yml/inner.txt": file("x\n")},
		"a dependabot":        {".github/dependabot.yml/inner.txt": file("x\n")},
		"a yaml dependabot":   {".github/dependabot.yaml/inner.txt": file("x\n")},
		"the record":          {".github/actions-qa-ci.yml/inner.txt": file("x\n")},
	}

	for name, tree := range cases {
		got, err := check.Discover(tree)
		if !errors.Is(err, check.ErrNotAnalysable) {
			t.Errorf("%s: Discover = %v, %v; a directory where a file belongs must stop the run", name, got, err)
		}
	}
}

func TestDiscoverEscapesTheNamesInItsErrorsSoAFileNameCannotForgeALine(t *testing.T) {
	evil := ".github/workflows/a\nactions-qa: forged\x1b[2J.yml"
	tree := fstest.MapFS{evil: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("x")}}

	_, err := check.Discover(tree)

	if err == nil || strings.ContainsAny(err.Error(), "\n\x1b") || !errors.Is(err, check.ErrNotAnalysable) {
		t.Errorf("error = %q; it must be one line with nothing a terminal would act on, and still wrap ErrNotAnalysable", err)
	}
}

// failingFS cannot list one directory.
type failingFS struct {
	fstest.MapFS
	broken string
}

func (f failingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.broken {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}

	return f.MapFS.ReadDir(name)
}

func TestDiscoverStopsOnADirectoryItCannotListAndEscapesItsName(t *testing.T) {
	evil := ".github/actions/evil\nactions-qa: forged\x1b[2J"
	tree := fstest.MapFS{
		".github/workflows/ci.yml": file("on: push\n"),
		evil + "/action.yml":       file("runs: {}\n"),
	}

	for name, broken := range map[string]string{"workflows": ".github/workflows", "actions": ".github/actions", "a nested action directory": evil} {
		got, err := check.Discover(failingFS{tree, broken})

		if err == nil {
			t.Errorf("%s: Discover = %v; a directory that cannot be listed is not an empty one", name, got)

			continue
		}
		if strings.ContainsAny(err.Error(), "\n\x1b") {
			t.Errorf("%s: error = %q; the name must not reach a terminal as it is", name, err)
		}
	}
}

func TestALaneErrorIsOneLineWhateverTheFileOrTheToolSaid(t *testing.T) {
	lanes := []check.Lane{{
		Name:    "broken",
		Selects: func(string) bool { return true },
		Run: func(string) ([]finding.Finding, error) {
			return nil, errors.New("could not check evil\nactions-qa: forged\x1b[2J")
		},
	}}

	_, err := check.Run(lanes, []string{"a\nb.yml"})

	if err == nil || strings.ContainsAny(err.Error(), "\n\x1b") {
		t.Errorf("error = %q", err)
	}
	var laneErr *check.LaneError
	if !errors.As(err, &laneErr) || laneErr.Path != "a\nb.yml" {
		t.Errorf("the error must still carry the path unaltered: %+v", laneErr)
	}
}
