package check_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func file(content string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(content)} }

func TestDiscoverFindsTheDefaultSetAndNothingElse(t *testing.T) {
	tree := fstest.MapFS{
		".github/workflows/b.yaml":               file("on: push\n"),
		".github/workflows/a.yml":                file("on: push\n"),
		".github/workflows/notes.txt":            file("not a workflow\n"),
		".github/workflows/sub/c.yml":            file("GitHub reads only the top level\n"),
		".github/workflows/UPPER.YML":            file("GitHub reads only lower case\n"),
		".github/actions/x/action.yml":           file("runs: {}\n"),
		".github/actions/x/y/action.yaml":        file("runs: {}\n"),
		".github/actions/x/readme.md":            file("# x\n"),
		".github/actions/x/y/other.yml":          file("not an action\n"),
		"action.yml":                             file("runs: {}\n"),
		".github/dependabot.yml":                 file("version: 2\n"),
		".github/actions-qa-ci.yml":              file("version: 1\n"),
		"README.md":                              file("# repo\n"),
		"other/action.yml":                       file("outside the default set\n"),
		"templates/github-actions/php-qa-ci.yml": file("on: push\n"),
	}

	got, err := check.Discover(tree)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		".github/actions-qa-ci.yml",
		".github/actions/x/action.yml",
		".github/actions/x/y/action.yaml",
		".github/dependabot.yml",
		".github/workflows/a.yml",
		".github/workflows/b.yaml",
		"action.yml",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Discover = %v\nwant       %v", got, want)
	}
}

func TestDiscoverInARepositoryWithNoActionsCodeFindsNothingAndSaysNothingIsWrong(t *testing.T) {
	got, err := check.Discover(fstest.MapFS{"README.md": file("# repo\n")})
	if err != nil || len(got) != 0 {
		t.Errorf("Discover = %v, %v", got, err)
	}
}

func TestDiscoverRefusesWhatItCannotReadAsAFileInsteadOfSkippingIt(t *testing.T) {
	link := &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("elsewhere.yml")}
	cases := map[string]fstest.MapFS{
		"a symlinked workflow":              {".github/workflows/ci.yml": link},
		"a symlink with another name":       {".github/workflows/ci": link},
		"a directory named like a workflow": {".github/workflows/ci.yml/inner.txt": file("x\n")},
		"a symlinked workflows directory":   {".github/workflows": link},
		"a symlinked actions directory":     {".github/actions": link},
		"a symlinked action":                {".github/actions/x/action.yml": link},
		"a symlink anywhere under actions":  {".github/actions/x/shared": link},
		"a symlinked root action":           {"action.yml": link},
		"a symlinked dependabot file":       {".github/dependabot.yml": link},
		"a symlinked record":                {".github/actions-qa-ci.yml": link},
		"a symlinked .github directory":     {".github": link},
		"a .github that is a file":          {".github": file("not a directory\n")},
	}

	for name, tree := range cases {
		got, err := check.Discover(tree)
		if err == nil {
			t.Errorf("%s: Discover = %v; a file it cannot analyse must stop the run, not be skipped", name, got)

			continue
		}
		if !errors.Is(err, check.ErrNotAnalysable) {
			t.Errorf("%s: error %v does not wrap ErrNotAnalysable", name, err)
		}
	}
}

func TestDiscoverNamesTheFileItCannotRead(t *testing.T) {
	tree := fstest.MapFS{".github/workflows/ci.yml": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("x")}}

	_, err := check.Discover(tree)

	if err == nil || !strings.Contains(err.Error(), ".github/workflows/ci.yml") {
		t.Errorf("error = %v; it must name the file", err)
	}
}

// recording is a lane that notes what it was asked to analyse.
func recording(name string, selects func(string) bool, ran *[]string) check.Lane {
	return check.Lane{
		Name:    name,
		Selects: selects,
		Run: func(path string) ([]finding.Finding, error) {
			*ran = append(*ran, name+":"+path)

			return []finding.Finding{{
				ID: "actionsqaci.actionlint.glob", Tool: "t", Path: path, Line: 1, Col: 1,
				Severity: finding.SeverityError, Message: "m from " + name,
			}}, nil
		},
	}
}

func suffix(s string) func(string) bool {
	return func(path string) bool { return strings.HasSuffix(path, s) }
}

func TestRunGivesEachLaneTheFilesItSelectsInOrderAndMergesWhatTheyFind(t *testing.T) {
	var ran []string
	lanes := []check.Lane{
		recording("first", suffix(".yml"), &ran),
		recording("second", suffix("a.yml"), &ran),
	}

	got, err := check.Run(lanes, []string{"b.yml", "a.yml", "c.txt", "a.yml"})
	if err != nil {
		t.Fatal(err)
	}

	wantRan := []string{"first:a.yml", "first:b.yml", "second:a.yml"}
	if !slices.Equal(ran, wantRan) {
		t.Errorf("ran %v, want %v (each file once, sorted, in lane order; c.txt selected by none)", ran, wantRan)
	}
	if len(got) != 3 {
		t.Errorf("findings = %d, want 3", len(got))
	}
	if !slices.IsSortedFunc(got, func(a, b finding.Finding) int { return strings.Compare(a.Path, b.Path) }) {
		t.Errorf("findings are not sorted: %+v", got)
	}
}

func TestRunStopsAtTheFirstFailureAndReturnsNoFindingsAtAll(t *testing.T) {
	boom := errors.New("tool exploded")
	var ran []string
	lanes := []check.Lane{
		{Name: "broken", Selects: suffix(".yml"), Run: func(path string) ([]finding.Finding, error) {
			ran = append(ran, path)

			return nil, boom
		}},
		recording("never", suffix(".yml"), &ran),
	}

	got, err := check.Run(lanes, []string{"a.yml", "b.yml"})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("error = %v, want one wrapping the tool's", err)
	}
	if got != nil {
		t.Errorf("findings = %v; a run that failed has no verdict, so no partial findings", got)
	}
	if !slices.Equal(ran, []string{"a.yml"}) {
		t.Errorf("ran %v; nothing may run after a failure", ran)
	}
	var laneErr *check.LaneError
	if !errors.As(err, &laneErr) || laneErr.Lane != "broken" || laneErr.Path != "a.yml" {
		t.Errorf("error %v must name the lane and the file", err)
	}
}

func TestRunRefusesALaneThatCannotWork(t *testing.T) {
	run := func(string) ([]finding.Finding, error) { return nil, nil }
	cases := map[string][]check.Lane{
		"no name":      {{Selects: suffix(".yml"), Run: run}},
		"no selector":  {{Name: "x", Run: run}},
		"nothing runs": {{Name: "x", Selects: suffix(".yml")}},
		"repeated":     {{Name: "x", Selects: suffix(".yml"), Run: run}, {Name: "x", Selects: suffix(".yml"), Run: run}},
	}

	for name, lanes := range cases {
		if _, err := check.Run(lanes, []string{"a.yml"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSelectingSaysWhichLanesAnalyseAFile(t *testing.T) {
	var ran []string
	lanes := []check.Lane{
		recording("yml", suffix(".yml"), &ran),
		recording("txt", suffix(".txt"), &ran),
		recording("all", func(string) bool { return true }, &ran),
	}

	var names []string
	for _, lane := range check.Selecting(lanes, "a.yml") {
		names = append(names, lane.Name)
	}

	if !slices.Equal(names, []string{"yml", "all"}) {
		t.Errorf("lanes = %v", names)
	}
}
