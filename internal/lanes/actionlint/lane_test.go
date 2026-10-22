package actionlint_test

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// inProject writes the fixture as a repository in a temporary directory, makes it the working
// directory, and returns the repository-relative path of the workflow to lint.
func inProject(t *testing.T, fx fixture) string {
	t.Helper()

	root := t.TempDir()
	files := map[string]string{".git/HEAD": "ref: refs/heads/main\n"}
	for path, content := range fx.files {
		files[path] = content
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	return fx.target
}

func kindsOf(findings []finding.Finding) []string {
	var kinds []string
	for _, f := range findings {
		if !slices.Contains(kinds, f.UpstreamID) {
			kinds = append(kinds, f.UpstreamID)
		}
	}
	slices.Sort(kinds)

	return kinds
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func TestEveryKindHasAFixturePairAndEveryPairIsForAKind(t *testing.T) {
	var kinds []string
	for _, kind := range actionlint.Kinds() {
		kinds = append(kinds, kind.Name)
	}
	slices.Sort(kinds)

	if got, want := strings.Join(sortedKeys(fixtures), ","), strings.Join(kinds, ","); got != want {
		t.Errorf("fixtures cover %s, the kinds are %s", got, want)
	}
}

func TestEachRedFixtureFiresItsKindAndNothingElse(t *testing.T) {
	for _, kind := range sortedKeys(fixtures) {
		target := inProject(t, fixtures[kind].red)

		found, err := actionlint.Lint(target)
		if err != nil {
			t.Errorf("%s: %v", kind, err)

			continue
		}
		if got := kindsOf(found); strings.Join(got, ",") != kind {
			t.Errorf("%s: the red fixture reported %v", kind, got)
		}
		for _, f := range found {
			if f.ID != actionlint.ID(kind) {
				t.Errorf("%s: finding has identifier %s, want %s", kind, f.ID, actionlint.ID(kind))
			}
		}
	}
}

func TestEachGreenFixtureIsClean(t *testing.T) {
	for _, kind := range sortedKeys(fixtures) {
		target := inProject(t, fixtures[kind].green)

		found, err := actionlint.Lint(target)
		if err != nil {
			t.Errorf("%s: %v", kind, err)

			continue
		}
		if len(found) != 0 {
			t.Errorf("%s: the green fixture reported %+v", kind, found)
		}
	}
}

func TestTheKindsAreTheOnesActionlintCanReport(t *testing.T) {
	var reported []string
	linter, err := upstream.NewLinter(io.Discard, &upstream.LinterOptions{
		OnRulesCreated: func(rules []upstream.Rule) []upstream.Rule {
			for _, rule := range rules {
				reported = append(reported, rule.Name())
			}

			return rules
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linter.Lint("w.yml", []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"), nil); err != nil {
		t.Fatal(err)
	}
	// syntax-check is reported by the parser, not by a rule.
	reported = append(reported, "syntax-check")
	slices.Sort(reported)

	var ours []string
	for _, kind := range actionlint.Kinds() {
		ours = append(ours, kind.Name)
	}
	slices.Sort(ours)

	if strings.Join(reported, ",") != strings.Join(ours, ",") {
		t.Errorf("actionlint reports %v but the lane knows %v: a new upstream kind needs a page and a fixture pair", reported, ours)
	}
}

func TestAProjectsOwnActionlintConfigDoesNotSuppressAFinding(t *testing.T) {
	fx := fixtures["syntax-check"].red
	fx.files = map[string]string{
		workflowPath:              fixtures["syntax-check"].red.files[workflowPath],
		".github/actionlint.yaml": "paths:\n  \".github/workflows/**/*.yml\":\n    ignore:\n      - 'unexpected key'\n",
		".github/actionlint.yml":  "ignore:\n  - 'unexpected key'\n",
	}
	target := inProject(t, fx)

	found, err := actionlint.Lint(target)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(kindsOf(found), ",") != "syntax-check" {
		t.Errorf("a native ignore suppressed the finding: %+v", found)
	}
}

func TestFindingsCarryTheLaneIdentity(t *testing.T) {
	target := inProject(t, fixtures["glob"].red)

	found, err := actionlint.Lint(target)
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, err %v", found, err)
	}
	f := found[0]

	if f.ID != "actionsqaci.actionlint.glob" || f.UpstreamID != "glob" || f.Tool != "actionlint "+actionlint.Version {
		t.Errorf("identity: %+v", f)
	}
	if f.Path != workflowPath || f.Line < 1 || f.Col < 1 || f.Severity != finding.SeverityError {
		t.Errorf("position: %+v", f)
	}
	if f.DocPath != registry.DocPath(f.ID) || f.Excepted {
		t.Errorf("page or exception: %+v", f)
	}
	if err := f.Validate(); err != nil {
		t.Errorf("the finding is not valid: %v", err)
	}
}

func TestConvertRefusesAKindItDoesNotKnow(t *testing.T) {
	for _, kind := range []string{"brand-new", "", "shellcheck", "pyflakes"} {
		_, err := actionlint.Convert(workflowPath, []*upstream.Error{{Message: "m", Line: 1, Column: 1, Kind: kind}})

		if err == nil || !strings.Contains(err.Error(), "kind") {
			t.Errorf("kind %q: err = %v; an unknown kind must be a toolchain fault, never silently dropped or relabelled", kind, err)
		}
	}
}

func TestConvertOrdersFindingsByPositionWhateverOrderActionlintReportedThem(t *testing.T) {
	found, err := actionlint.Convert(workflowPath, []*upstream.Error{
		{Message: "later", Line: 9, Column: 1, Kind: "glob"},
		{Message: "earlier", Line: 2, Column: 1, Kind: "glob"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 2 || found[0].Message != "earlier" || found[1].Message != "later" {
		t.Errorf("order = %+v", found)
	}
}

func TestConvertRefusesAFindingThatCannotBeReported(t *testing.T) {
	_, err := actionlint.Convert("", []*upstream.Error{{Message: "m", Line: 1, Column: 1, Kind: "glob"}})

	if err == nil || !strings.Contains(err.Error(), "cannot be reported") {
		t.Errorf("err = %v; a finding with no path must not be printed", err)
	}
}

func TestLintLeavesNothingBehindInTheTemporaryDirectory(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	target := inProject(t, fixtures["glob"].red)

	if _, err := actionlint.Lint(target); err != nil {
		t.Fatal(err)
	}

	left, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the lane left %d entries in the temporary directory", len(left))
	}
}

func TestAMalformedProjectConfigFailsTheLaneRatherThanBeingHonouredOrSkipped(t *testing.T) {
	fx := fixtures["glob"].green
	fx.files = map[string]string{
		workflowPath:              fixtures["glob"].green.files[workflowPath],
		".github/actionlint.yaml": "paths: [this is: not valid",
	}
	target := inProject(t, fx)

	if _, err := actionlint.Lint(target); err == nil {
		t.Error("a malformed project configuration was ignored without a word")
	}
}

func TestConvertTurnsAnUnknownPositionIntoTheStartOfTheFile(t *testing.T) {
	found, err := actionlint.Convert(workflowPath, []*upstream.Error{{Message: "m", Line: 0, Column: 0, Kind: "glob"}})
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, err %v", found, err)
	}

	if found[0].Line != 1 || found[0].Col != 1 {
		t.Errorf("position = %d:%d, want 1:1", found[0].Line, found[0].Col)
	}
}

func TestTheVersionIsTheOneGoModRequires(t *testing.T) {
	gomod, err := os.ReadFile("../../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`github\.com/rhysd/actionlint (v[0-9][^\s]*)`).FindSubmatch(gomod)
	if match == nil {
		t.Fatal("go.mod does not require actionlint")
	}

	if string(match[1]) != actionlint.Version {
		t.Errorf("Version = %s, go.mod requires %s", actionlint.Version, match[1])
	}
}

func TestLintRefusesAWorkingDirectoryThatIsNotARepositoryRoot(t *testing.T) {
	// Without a .git here, actionlint walks up to whatever enclosing project it finds, so a
	// local reusable workflow is looked for in the wrong place and findings go missing.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, workflowPath), []byte(fixtures["glob"].green.files[workflowPath]), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	_, err := actionlint.Lint(workflowPath)

	if err == nil || !strings.Contains(err.Error(), ".git") {
		t.Errorf("err = %v; a directory with no .git must be refused, naming .git", err)
	}
}

func TestLintRefusesARepositoryRootWithNoWorkflowsDirectory(t *testing.T) {
	// actionlint finds its project by .git together with .github/workflows; with only .git it
	// resolves local reusable workflows against an enclosing project.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "w.yml"), []byte(fixtures["glob"].green.files[workflowPath]), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	_, err := actionlint.Lint("w.yml")

	if err == nil || !strings.Contains(err.Error(), ".github/workflows") {
		t.Errorf("err = %v; a root without .github/workflows must be refused, naming it", err)
	}
}

func TestLintAcceptsARepositoryWhoseGitEntryIsAFile(t *testing.T) {
	// A linked worktree and a submodule have a .git file, not a directory.
	target := inProject(t, fixtures["glob"].green)
	if err := os.RemoveAll(".git"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".git", []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := actionlint.Lint(target)

	if err != nil || len(found) != 0 {
		t.Errorf("found %+v, err %v", found, err)
	}
}

func TestLintOfAFileThatIsNotThereIsAnError(t *testing.T) {
	inProject(t, fixtures["glob"].green)

	if _, err := actionlint.Lint(".github/workflows/missing.yml"); err == nil {
		t.Error("a missing file was linted as clean")
	}
}

func TestLintOfAFileThatIsNotYAMLReportsASyntaxFindingNotAFault(t *testing.T) {
	target := inProject(t, fixture{target: workflowPath, files: map[string]string{workflowPath: "on: [push\njobs: {"}})

	found, err := actionlint.Lint(target)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(kindsOf(found), ",") != "syntax-check" {
		t.Errorf("found %+v", found)
	}
}

func TestEachRuleChecksOnlyItsOwnKind(t *testing.T) {
	rules := actionlint.Rules()
	if len(rules) != len(fixtures) {
		t.Fatalf("%d rules for %d kinds", len(rules), len(fixtures))
	}

	for _, rule := range rules {
		if rule.Check == nil {
			t.Errorf("%s cannot run on its own", rule.ID)

			continue
		}
		kind := strings.TrimPrefix(rule.ID, "actionsqaci.actionlint.")

		for _, other := range sortedKeys(fixtures) {
			target := inProject(t, fixtures[other].red)

			found, err := rule.Check(target)
			if err != nil {
				t.Errorf("%s on the %s fixture: %v", rule.ID, other, err)

				continue
			}
			if other == kind && len(found) == 0 {
				t.Errorf("%s did not fire on its own red fixture", rule.ID)
			}
			if other != kind && len(found) != 0 {
				t.Errorf("%s reported %+v on the %s fixture", rule.ID, found, other)
			}
		}
	}
}

func TestEveryRuleStatesItsInstructionAndWhereItComesFrom(t *testing.T) {
	for _, rule := range actionlint.Rules() {
		if strings.TrimSpace(rule.Summary) == "" {
			t.Errorf("%s has no summary", rule.ID)
		}
		if rule.Upstream != "actionlint "+actionlint.Version {
			t.Errorf("%s upstream = %q", rule.ID, rule.Upstream)
		}
	}

	if _, err := registry.New(actionlint.Rules()...); err != nil {
		t.Errorf("the rules do not form a registry: %v", err)
	}
}
