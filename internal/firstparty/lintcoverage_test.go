package firstparty_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
)

const shapedWorkflow = "name: x\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"

const shapedAction = "name: a\ndescription: d\nruns:\n  using: composite\n  steps: []\n"

// covered writes the files, makes them all tracked, and runs the check on the record path.
func covered(t *testing.T, files map[string]string, tracked ...string) ([]finding.Finding, error) {
	t.Helper()

	inTree(t, files)
	gitindextest.Write(t, ".", gitindextest.Files(tracked...)...)

	return firstparty.CheckLintCoverage(everythingIsKnown)(recordPath)
}

func unanalysedRecord(patterns ...string) string {
	text := "version: 1\nanalyse:\n  unanalysed:\n"
	for _, pattern := range patterns {
		text += "    - path: '" + pattern + "'\n" +
			"      justification:\n" +
			"        hazard: \"Fixture workflows hold the very mistakes the rules are tested against\"\n" +
			"        scope: \"Only these files, which no workflow of the project ever runs\"\n"
	}

	return text
}

func TestAWorkflowShapedFileNoLaneAnalysesIsAFindingAtTheFile(t *testing.T) {
	got, err := covered(t, map[string]string{"templates/ci.yml": shapedWorkflow}, "templates/ci.yml")

	if err != nil || len(got) != 1 {
		t.Fatalf("findings %+v, error %v; want one", got, err)
	}
	f := got[0]
	if f.ID != firstparty.LintCoverage || f.Path != "templates/ci.yml" || f.Line != 3 || f.Severity != finding.SeverityError || f.Tool != firstparty.Tool {
		t.Errorf("finding %+v; want an error at the file, on the line of its jobs key", f)
	}
	for _, want := range []string{"templates/ci.yml", "workflow-shaped", "analyse.unanalysed"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q should mention %q", f.Message, want)
		}
	}
	if err := f.Validate(); err != nil {
		t.Error(err)
	}
}

func TestTheDefaultSetNeedsNoDeclaration(t *testing.T) {
	files := map[string]string{
		".github/workflows/ci.yml":     shapedWorkflow,
		".github/workflows/other.yaml": shapedWorkflow,
		"action.yml":                   shapedAction,
		".github/actions/x/action.yml": shapedAction,
	}

	got, err := covered(t, files, ".github/workflows/ci.yml", ".github/workflows/other.yaml", "action.yml", ".github/actions/x/action.yml")

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none", got, err)
	}
}

func TestACompositeActionOutsideTheDefaultSetIsAFindingAndOnlyAnActionIsOne(t *testing.T) {
	files := map[string]string{
		"tools/x/action.yml":  shapedAction,
		"data/action.yml":     "name: a\nitems: [1, 2]\n",
		"tools/runs-file.yml": shapedAction,
	}

	got, err := covered(t, files, "tools/x/action.yml", "data/action.yml", "tools/runs-file.yml")

	if err != nil || len(got) != 1 || got[0].Path != "tools/x/action.yml" || got[0].Line != 3 || !strings.Contains(got[0].Message, "action-shaped") {
		t.Errorf("findings %+v, error %v; want one, for the action.yml that has runs", got, err)
	}
}

func TestFilesThatAreNotWorkflowShapedAreLeftAlone(t *testing.T) {
	files := map[string]string{
		"on-only.yml":     "on: push\n",
		"jobs-only.yml":   "jobs:\n  a: {}\n",
		"jobs-scalar.yml": "on: push\njobs: x\n",
		"list.yml":        "- a\n- b\n",
		"empty.yml":       "",
		"broken.yml":      "on: [unclosed\njobs:\n",
		"data.yml":        "name: x\nitems: [1]\n",
		"notes.txt":       shapedWorkflow,
		"Upper.YML":       shapedWorkflow,
	}
	var tracked []string
	for name := range files {
		tracked = append(tracked, name)
	}

	got, err := covered(t, files, tracked...)

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none", got, err)
	}
}

func TestOnlyTheFilesTheRepositoryTracksAreConsidered(t *testing.T) {
	files := map[string]string{
		"untracked.yml": shapedWorkflow,
		"link.yml":      shapedWorkflow,
		"deleted.yml":   shapedWorkflow,
		"tracked.yml":   shapedWorkflow,
	}
	inTree(t, files)
	if err := os.Remove("deleted.yml"); err != nil {
		t.Fatal(err)
	}
	gitindextest.Write(t, ".", gitindextest.File("tracked.yml"), gitindextest.Link("link.yml"), gitindextest.File("deleted.yml"))

	got, err := firstparty.CheckLintCoverage(everythingIsKnown)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != "tracked.yml" {
		t.Errorf("findings %+v, error %v; want one, for tracked.yml: an untracked file, a link and a deleted file have nothing to analyse", got, err)
	}
}

func TestANestedWorkflowGitHubDoesNotReadIsStillWorkflowShaped(t *testing.T) {
	got, err := covered(t, map[string]string{".github/workflows/sub/ci.yml": shapedWorkflow}, ".github/workflows/sub/ci.yml")

	if err != nil || len(got) != 1 || got[0].Path != ".github/workflows/sub/ci.yml" {
		t.Errorf("findings %+v, error %v; a file there is never run and never checked", got, err)
	}
}

func TestADeclarationExcusesTheFilesItNamesAndIsHeldToBeUsed(t *testing.T) {
	files := map[string]string{
		recordPath:                 unanalysedRecord("tests/fixtures/**"),
		"tests/fixtures/a/ci.yml":  shapedWorkflow,
		"tests/fixtures/b.yml":     shapedWorkflow,
		"tests/other/ci.yml":       shapedWorkflow,
		".github/workflows/ci.yml": shapedWorkflow,
	}

	got, err := covered(t, files, "tests/fixtures/a/ci.yml", "tests/fixtures/b.yml", "tests/other/ci.yml", ".github/workflows/ci.yml")

	if err != nil || len(got) != 1 || got[0].Path != "tests/other/ci.yml" {
		t.Errorf("findings %+v, error %v; want one, for the file the declaration does not name", got, err)
	}
}

func TestADeclarationThatExcusesNothingIsAFindingAtItsEntry(t *testing.T) {
	files := map[string]string{
		recordPath:                 unanalysedRecord("gone/**", ".github/workflows/*.yml"),
		".github/workflows/ci.yml": shapedWorkflow,
	}

	got, err := covered(t, files, ".github/workflows/ci.yml")

	if err != nil || len(got) != 2 {
		t.Fatalf("findings %+v, error %v; want one for each entry", got, err)
	}
	if got[0].Path != recordPath || got[0].Line != 4 || !strings.Contains(got[0].Message, "gone/**") || !strings.Contains(got[0].Message, "excuses no") {
		t.Errorf("first finding %+v", got[0])
	}
	if got[1].Path != recordPath || got[1].Line != 8 || !strings.Contains(got[1].Message, ".github/workflows/*.yml") {
		t.Errorf("second finding %+v; an entry that names only files that are analysed anyway turns nothing off and excuses nothing", got[1])
	}
}

func includeRecord(patterns ...string) string {
	text := "version: 1\nanalyse:\n  include:\n"
	for _, pattern := range patterns {
		text += "    - '" + pattern + "'\n"
	}

	return text
}

func TestAnIncludedFileIsAnalysedSoItNeedsNoDeclaration(t *testing.T) {
	files := map[string]string{
		recordPath:               includeRecord("templates/**"),
		"templates/ci.yml":       shapedWorkflow,
		"templates/x/action.yml": shapedAction,
		"other/ci.yml":           shapedWorkflow,
	}

	got, err := covered(t, files, "templates/ci.yml", "templates/x/action.yml", "other/ci.yml")

	if err != nil || len(got) != 1 || got[0].Path != "other/ci.yml" {
		t.Errorf("findings %+v, error %v; want one, for the file no pattern names", got, err)
	}
}

func TestAnIncludedPatternThatNamesNothingIsAFindingAtItsEntry(t *testing.T) {
	files := map[string]string{
		recordPath:                 includeRecord("gone/**", "docs/*.md", ".github/workflows/*.yml"),
		".github/workflows/ci.yml": shapedWorkflow,
		"docs/notes.md":            "# notes\n",
	}

	got, err := covered(t, files, ".github/workflows/ci.yml", "docs/notes.md")

	if err != nil || len(got) != 3 {
		t.Fatalf("findings %+v, error %v; want one for each entry", got, err)
	}
	for i, pattern := range []string{"gone/**", "docs/*.md", ".github/workflows/*.yml"} {
		if got[i].Path != recordPath || got[i].Line != 4+i || !strings.Contains(got[i].Message, pattern) || !strings.Contains(got[i].Message, "includes no") {
			t.Errorf("finding %d: %+v; want one at line %d naming %s", i, got[i], 4+i, pattern)
		}
	}
}

func TestAnIncludedFileIsAnalysedWhateverItLooksLike(t *testing.T) {
	files := map[string]string{
		recordPath:        includeRecord("templates/*.yml"),
		"templates/a.yml": "name: broken\n",
	}

	got, err := covered(t, files, "templates/a.yml")

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; the lanes judge a file the project asked for, not the shape check", got, err)
	}
}

func TestADeclarationThatNamesOnlyAnIncludedFileExcusesNothing(t *testing.T) {
	files := map[string]string{
		recordPath: "version: 1\nanalyse:\n  include: [templates/**]\n  unanalysed:\n    - path: 'templates/**'\n" +
			"      justification:\n        hazard: \"Fixture workflows hold the very mistakes the rules are tested against\"\n" +
			"        scope: \"Only these files, which no workflow of the project ever runs\"\n",
		"templates/ci.yml": shapedWorkflow,
	}

	got, err := covered(t, files, "templates/ci.yml")

	if err != nil || len(got) != 1 || got[0].Path != recordPath || got[0].Line != 5 || !strings.Contains(got[0].Message, "excuses no") {
		t.Errorf("findings %+v, error %v; an included file is analysed, so excusing it is contradictory", got, err)
	}
}

func TestAnIncludedFileThatCannotBeAnalysedStopsTheCheck(t *testing.T) {
	files := map[string]string{recordPath: includeRecord("templates/**")}
	inTree(t, files)
	gitindextest.Write(t, ".", gitindextest.Files("templates/gone.yml")...)

	_, err := firstparty.CheckLintCoverage(everythingIsKnown)(recordPath)

	if !errors.Is(err, check.ErrNotAnalysable) {
		t.Errorf("error = %v; a file asked for and absent is no verdict", err)
	}
}

func TestTheCheckRunsWhenThereIsNoRecord(t *testing.T) {
	got, err := covered(t, map[string]string{"t.yml": shapedWorkflow}, "t.yml")

	if err != nil || len(got) != 1 {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestAFileOverTheLimitIsNotExaminedAndItsSizeIsTheReason(t *testing.T) {
	big := shapedWorkflow + "# " + strings.Repeat("x", 1<<20) + "\n"

	got, err := covered(t, map[string]string{"big.yml": big}, "big.yml")

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; a file over 1 MiB is not examined", got, err)
	}
}

func TestWhatCannotBeKnownIsAnErrorNotACleanResult(t *testing.T) {
	inTree(t, map[string]string{"a.yml": shapedWorkflow})
	if _, err := firstparty.CheckLintCoverage(everythingIsKnown)(recordPath); err == nil {
		t.Error("a directory with no .git was accepted")
	}

	gitindextest.Write(t, ".", gitindextest.File("a.yml"))
	if err := os.WriteFile(filepath.Join(".git", "index"), []byte("not an index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := firstparty.CheckLintCoverage(everythingIsKnown)(recordPath); err == nil {
		t.Error("a damaged index was accepted")
	}
}

func TestATrackedNameThatCannotBePrintedIsAnErrorWhenTheFileIsWorkflowShaped(t *testing.T) {
	name := "bad\nname.yml"
	inTree(t, map[string]string{name: shapedWorkflow})
	gitindextest.Write(t, ".", gitindextest.File(name))

	_, err := firstparty.CheckLintCoverage(everythingIsKnown)(recordPath)

	if err == nil || strings.Contains(err.Error(), "\n") {
		t.Errorf("error %q; want an error whose text holds no line break", err)
	}
}

func TestARecordThatIsNotValidStopsTheCheck(t *testing.T) {
	_, err := covered(t, map[string]string{recordPath: "version: 1\nanalyse:\n  unanalysed:\n    - path: '[bad'\n"})

	if err == nil {
		t.Error("an invalid record was accepted")
	}
}

func TestAnUnanalysedEntryWithoutAJustificationIsAnUnjustifiedFinding(t *testing.T) {
	inTree(t, map[string]string{recordPath: "version: 1\nanalyse:\n  unanalysed:\n    - path: 'a/**'\n" + strings.TrimPrefix(unanalysedRecord("b/**"), "version: 1\nanalyse:\n  unanalysed:\n")})

	got, err := firstparty.CheckRecord(everythingIsKnown)(recordPath)

	if err != nil || len(got) != 2 {
		t.Fatalf("findings %+v, error %v; want a hazard and a scope fault for the first entry only", got, err)
	}
	for i, field := range []string{"hazard", "scope"} {
		if got[i].ID != firstparty.UnjustifiedException || got[i].Line != 4 || !strings.HasPrefix(got[i].Message, field+" of unanalysed entry 1 (a/**)") {
			t.Errorf("finding %d = %+v", i, got[i])
		}
	}
}

func TestLintCoverageIsARegisteredRuleWithASummary(t *testing.T) {
	for _, rule := range firstparty.Rules(everythingIsKnown) {
		if rule.ID == firstparty.LintCoverage {
			if rule.Summary == "" || rule.Check == nil {
				t.Errorf("rule %+v needs a summary and a check", rule)
			}

			return
		}
	}
	t.Errorf("%s is not a registered first-party rule", firstparty.LintCoverage)
}
