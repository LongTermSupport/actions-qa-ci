package scope_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/scope"
)

const (
	regular = 0o100644
	link    = 0o120000
)

func project(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("on: push\njobs: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func tracked(paths ...string) []gitindex.Entry {
	entries := make([]gitindex.Entry, len(paths))
	for i, path := range paths {
		entries[i] = gitindex.Entry{Path: path, Mode: regular}
	}

	return entries
}

func patterns(paths ...string) []record.Included {
	included := make([]record.Included, len(paths))
	for i, path := range paths {
		included[i] = record.Included{Path: path, Line: i + 1}
	}

	return included
}

func TestResolveFindsTheTrackedYamlFilesThePatternsNameAndSaysWhichKindEachIs(t *testing.T) {
	files := []string{"templates/ci.yml", "templates/deploy.yaml", "templates/lib/action.yml", "templates/README.md", "other/x.yml"}
	root := project(t, files...)

	got, err := scope.Resolve(root, tracked(files...), patterns("templates/**"))

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"templates/ci.yml", "templates/deploy.yaml"}; !slices.Equal(got.Included.Workflows, want) {
		t.Errorf("workflows = %q; want %q", got.Included.Workflows, want)
	}
	if want := []string{"templates/lib/action.yml"}; !slices.Equal(got.Included.Actions, want) {
		t.Errorf("actions = %q; want %q", got.Included.Actions, want)
	}
	if !slices.Equal(got.Used, []bool{true}) {
		t.Errorf("used = %v", got.Used)
	}
}

func TestResolveSaysWhichPatternsNamedNothing(t *testing.T) {
	files := []string{"templates/ci.yml", "docs/notes.md"}
	root := project(t, files...)

	got, err := scope.Resolve(root, tracked(files...), patterns("templates/**", "docs/**", "missing/**"))

	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Used, []bool{true, false, false}) {
		t.Errorf("used = %v; docs/** names only a markdown file and missing/** names nothing", got.Used)
	}
}

func TestResolveLeavesTheDefaultSetToDiscovery(t *testing.T) {
	files := []string{".github/workflows/ci.yml", ".github/dependabot.yml", ".github/actions-qa-ci.yml", "action.yml", ".github/zizmor.yml"}
	root := project(t, files...)

	got, err := scope.Resolve(root, tracked(files...), patterns("**", ".github/**"))

	if err != nil {
		t.Fatal(err)
	}
	if len(got.Included.Files()) != 0 {
		t.Errorf("included %q; a file of the default set is analysed already", got.Included.Files())
	}
	if !slices.Equal(got.Used, []bool{false, false}) {
		t.Errorf("used = %v; a pattern that adds nothing new names nothing", got.Used)
	}
}

func TestResolveSkipsFilesNotTracked(t *testing.T) {
	root := project(t, "templates/ci.yml", "templates/untracked.yml")

	got, err := scope.Resolve(root, tracked("templates/ci.yml"), patterns("templates/*.yml"))

	if err != nil || !slices.Equal(got.Included.Workflows, []string{"templates/ci.yml"}) {
		t.Errorf("included = %+v, err = %v", got.Included, err)
	}
}

func TestResolveRefusesAnIncludedPathThatIsNotAPlainFile(t *testing.T) {
	root := project(t, "templates/real.yml")
	if err := os.Symlink("real.yml", filepath.Join(root, "templates", "alias.yml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "templates", "dir.yml"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]gitindex.Entry{
		"a link on disk":      tracked("templates/alias.yml"),
		"a directory on disk": tracked("templates/dir.yml"),
		"missing on disk":     tracked("templates/gone.yml"),
		"a link in the index": {{Path: "templates/real.yml", Mode: link}},
	}
	for name, entries := range cases {
		_, err := scope.Resolve(root, entries, patterns("templates/**"))

		if !errors.Is(err, check.ErrNotAnalysable) {
			t.Errorf("%s: error = %v; want one wrapping ErrNotAnalysable (a file asked for and skipped is a file nobody checked)", name, err)
		}
	}
}

func TestResolveRefusesAnIncludedNameThatCannotBePrinted(t *testing.T) {
	root := project(t, "templates/a\nb.yml")

	_, err := scope.Resolve(root, tracked("templates/a\nb.yml"), patterns("templates/**"))

	if !errors.Is(err, check.ErrNotAnalysable) {
		t.Errorf("error = %v", err)
	}
}

func TestResolveIgnoresAnEntryThatIsNotUsable(t *testing.T) {
	root := project(t, "templates/ci.yml")

	got, err := scope.Resolve(root, tracked("templates/ci.yml"), []record.Included{{Path: "", Line: 1}})

	if err != nil || len(got.Included.Files()) != 0 || len(got.Used) != 1 || got.Used[0] {
		t.Errorf("got %+v, err = %v; a record with such an entry is refused before this is reached", got, err)
	}
}

func TestResolveListsAFileOnceWhateverNumberOfPatternsNameIt(t *testing.T) {
	root := project(t, "templates/ci.yml")

	got, err := scope.Resolve(root, tracked("templates/ci.yml"), patterns("templates/**", "templates/*.yml"))

	if err != nil || !slices.Equal(got.Included.Workflows, []string{"templates/ci.yml"}) || !slices.Equal(got.Used, []bool{true, true}) {
		t.Errorf("got %+v, err = %v", got, err)
	}
}
