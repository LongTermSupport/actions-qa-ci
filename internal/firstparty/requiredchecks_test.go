package firstparty_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
)

func requiredRecord(contexts ...string) string {
	var text strings.Builder
	text.WriteString("version: 1\nrequiredChecks:\n")
	for _, context := range contexts {
		text.WriteString("  - context: " + context + "\n")
	}

	return text.String()
}

func checksIn(t *testing.T, files map[string]string) ([]finding.Finding, error) {
	t.Helper()

	inTree(t, files)

	return firstparty.CheckRequiredChecks(everythingIsKnown)(recordPath)
}

const pullRequestWorkflow = `name: CI
on:
  pull_request:
  push:
    branches: [main]
jobs:
  qa:
    name: QA Pipeline
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
  unnamed:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`

func TestARequiredContextThatAPullRequestJobProducesIsClean(t *testing.T) {
	got, err := checksIn(t, map[string]string{
		recordPath:                 requiredRecord("QA Pipeline", "unnamed"),
		".github/workflows/ci.yml": pullRequestWorkflow,
	})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none: a named job and an unnamed one (its id) both produce their context", got, err)
	}
}

func TestEveryWayOfSayingPullRequestTriggersTheWorkflow(t *testing.T) {
	forms := map[string]string{
		"a word":          "on: pull_request\n",
		"a list":          "on: [push, pull_request]\n",
		"a block list":    "on:\n  - push\n  - pull_request\n",
		"a map":           "on:\n  push: {}\n  pull_request:\n    branches: [main]\n",
		"a quoted key":    "\"on\":\n  pull_request:\n",
		"a map with null": "on:\n  pull_request: null\n",
	}

	for name, trigger := range forms {
		got, err := checksIn(t, map[string]string{
			recordPath:                 requiredRecord("QA"),
			".github/workflows/ci.yml": trigger + "jobs:\n  qa:\n    name: QA\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
		})

		if err != nil || len(got) != 0 {
			t.Errorf("%s: findings %+v, error %v; want none", name, got, err)
		}
	}
}

func TestAContextOnlyANonPullRequestWorkflowProducesIsAFindingSayingSo(t *testing.T) {
	for name, trigger := range map[string]string{
		"push":                "on: push\n",
		"a schedule":          "on:\n  schedule:\n    - cron: '0 0 * * *'\n",
		"pull_request_target": "on: pull_request_target\n",
		"no triggers":         "name: x\n",
	} {
		got, err := checksIn(t, map[string]string{
			recordPath:                 requiredRecord("QA"),
			".github/workflows/ci.yml": trigger + "jobs:\n  qa:\n    name: QA\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
		})

		if err != nil || len(got) != 1 {
			t.Errorf("%s: findings %+v, error %v; want one", name, got, err)

			continue
		}
		f := got[0]
		if f.ID != firstparty.RequiredChecks || f.Path != recordPath || f.Line != 3 || f.Severity != finding.SeverityError || f.Tool != firstparty.Tool ||
			!strings.Contains(f.Message, `"QA"`) || !strings.Contains(f.Message, ".github/workflows/ci.yml") || !strings.Contains(f.Message, "pull_request") {
			t.Errorf("%s: finding %+v; want an error at the record's line 3 naming the context, the workflow and pull_request", name, f)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAContextNothingProducesIsAFinding(t *testing.T) {
	got, err := checksIn(t, map[string]string{
		recordPath:                 requiredRecord("QA Pipeline", "ShellCheck (severity=warning)"),
		".github/workflows/ci.yml": pullRequestWorkflow,
	})

	if err != nil || len(got) != 1 || got[0].Line != 4 || !strings.Contains(got[0].Message, "ShellCheck (severity=warning)") || !strings.Contains(got[0].Message, "no job") {
		t.Errorf("findings %+v, error %v; want one, for the second entry on line 4 (the stale required context of task 2.1)", got, err)
	}
}

func TestARequiredContextWithNoWorkflowsAtAllIsAFindingNotAnError(t *testing.T) {
	got, err := checksIn(t, map[string]string{recordPath: requiredRecord("QA")})

	if err != nil || len(got) != 1 || !strings.Contains(got[0].Message, "no job") {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestAJobNameWithAnExpressionCannotBeAStableRequiredCheck(t *testing.T) {
	workflow := "on: pull_request\njobs:\n  build:\n    name: Build ${{ matrix.os }}\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"

	byID, err := checksIn(t, map[string]string{recordPath: requiredRecord("build"), ".github/workflows/ci.yml": workflow})
	if err != nil || len(byID) != 1 || !strings.Contains(byID[0].Message, "expression") || !strings.Contains(byID[0].Message, "build") {
		t.Errorf("by the job id: findings %+v, error %v; want one that says the name has an expression", byID, err)
	}

	byName, err := checksIn(t, map[string]string{recordPath: requiredRecord("'Build ${{ matrix.os }}'"), ".github/workflows/ci.yml": workflow})
	if err != nil || len(byName) != 1 || !strings.Contains(byName[0].Message, "expression") {
		t.Errorf("by the written name: findings %+v, error %v; want one: the check run never carries that text", byName, err)
	}
}

func TestAMatrixJobsContextCarriesASuffixSoThePlainNameIsNotStable(t *testing.T) {
	workflow := "on: pull_request\njobs:\n  test:\n    name: Tests\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        php: ['8.4', '8.5']\n    steps:\n      - run: echo\n"

	got, err := checksIn(t, map[string]string{recordPath: requiredRecord("Tests"), ".github/workflows/ci.yml": workflow})

	if err != nil || len(got) != 1 || !strings.Contains(got[0].Message, "matrix") {
		t.Errorf("findings %+v, error %v; want one that says the matrix adds a suffix", got, err)
	}
}

func TestAReusableWorkflowJobProducesContextsUnderItsName(t *testing.T) {
	workflow := "on: pull_request\njobs:\n  call:\n    uses: ./.github/workflows/inner.yml\n"

	got, err := checksIn(t, map[string]string{recordPath: requiredRecord("call / inner"), ".github/workflows/ci.yml": workflow})
	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; a context under the calling job's name is possible", got, err)
	}

	got, err = checksIn(t, map[string]string{recordPath: requiredRecord("other / inner"), ".github/workflows/ci.yml": workflow})
	if err != nil || len(got) != 1 {
		t.Errorf("a context under another name: findings %+v, error %v; want one", got, err)
	}
}

func TestAWorkflowThatIsNotYAMLIsAFindingNamingItNotAFailureOfTheRun(t *testing.T) {
	got, err := checksIn(t, map[string]string{
		recordPath:                  requiredRecord("QA"),
		".github/workflows/ci.yml":  pullRequestWorkflow,
		".github/workflows/bad.yml": "on: [unclosed\n",
	})

	if err != nil || len(got) < 1 {
		t.Fatalf("findings %+v, error %v", got, err)
	}
	var named bool
	for _, f := range got {
		named = named || (f.Path == ".github/workflows/bad.yml" && strings.Contains(f.Message, "YAML"))
	}
	if !named {
		t.Errorf("findings %+v; want one at .github/workflows/bad.yml saying it is not valid YAML, so the checks it produces are unknown", got)
	}
}

func TestAWorkflowThatIsALinkIsReportedAndNotRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, recordPath), []byte(requiredRecord("QA")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(root, ".github", "workflows", "link.yml")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	got, err := firstparty.CheckRequiredChecks(everythingIsKnown)(recordPath)

	var named bool
	for _, f := range got {
		named = named || (f.Path == ".github/workflows/link.yml" && strings.Contains(f.Message, "regular file"))
	}
	if err != nil || !named {
		t.Errorf("findings %+v, error %v; want one at the link saying it is not a regular file", got, err)
	}
}

func TestARecordWithoutRequiredChecksNeedsNoWorkflows(t *testing.T) {
	got, err := checksIn(t, map[string]string{recordPath: "version: 1\n"})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestOnlyTheWorkflowsGitHubReadsAreConsidered(t *testing.T) {
	got, err := checksIn(t, map[string]string{
		recordPath:                       requiredRecord("QA"),
		".github/workflows/sub/ci.yml":   pullRequestWorkflow,
		".github/workflows/notes.md":     "# not a workflow\n",
		"other/.github/workflows/ci.yml": pullRequestWorkflow,
	})

	if err != nil || len(got) != 1 || !strings.Contains(got[0].Message, "no job") {
		t.Errorf("findings %+v, error %v; a workflow GitHub does not read produces no check", got, err)
	}
}

func TestAMatrixJobsSuffixedContextIsReportedWithTheMatrixReason(t *testing.T) {
	workflow := "on: pull_request\njobs:\n  test:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        os: [ubuntu]\n    steps:\n      - run: echo\n"

	got, err := checksIn(t, map[string]string{recordPath: requiredRecord("'test (ubuntu)'"), ".github/workflows/ci.yml": workflow})

	if err != nil || len(got) != 1 || !strings.Contains(got[0].Message, "matrix") {
		t.Errorf("findings %+v, error %v; want one that says the matrix is why a name with a suffix is not stable", got, err)
	}
}

func TestAReusableCallerWithAnExpressionNameIsReportedWithTheExpressionReason(t *testing.T) {
	workflow := "on: pull_request\njobs:\n  call:\n    name: ${{ github.ref }}\n    uses: ./.github/workflows/inner.yml\n"

	got, err := checksIn(t, map[string]string{recordPath: requiredRecord("'${{ github.ref }} / inner'"), ".github/workflows/ci.yml": workflow})

	if err != nil || len(got) != 1 || !strings.Contains(got[0].Message, "expression") {
		t.Errorf("findings %+v, error %v; want one that says the name has an expression", got, err)
	}
}

func TestAliasesAreFollowedWhereverAJobValueIsRead(t *testing.T) {
	cases := map[string]struct{ workflow, context, mention string }{
		"a name": {
			"on: pull_request\nenv:\n  N: &n Build\njobs:\n  b:\n    name: *n\n    runs-on: x\n", "Build", "",
		},
		"a matrix through strategy": {
			"on: pull_request\nx: &s\n  matrix:\n    os: [a, b]\njobs:\n  m:\n    strategy: *s\n    runs-on: x\n", "m", "matrix",
		},
		"the jobs": {
			"on: pull_request\nx: &j\n  b:\n    runs-on: x\njobs: *j\n", "b", "",
		},
	}

	for name, c := range cases {
		got, err := checksIn(t, map[string]string{recordPath: requiredRecord(c.context), ".github/workflows/ci.yml": c.workflow})

		switch {
		case err != nil:
			t.Errorf("%s: %v", name, err)
		case c.mention == "" && len(got) != 0:
			t.Errorf("%s: findings %+v; want none", name, got)
		case c.mention != "" && (len(got) != 1 || !strings.Contains(got[0].Message, c.mention)):
			t.Errorf("%s: findings %+v; want one mentioning %q", name, got, c.mention)
		}
	}
}

func TestANullJobNameFallsBackToTheJobID(t *testing.T) {
	for _, written := range []string{"~", "null", ""} {
		workflow := "on: pull_request\njobs:\n  build:\n    name: " + written + "\n    runs-on: x\n"

		got, err := checksIn(t, map[string]string{recordPath: requiredRecord("build"), ".github/workflows/ci.yml": workflow})

		if err != nil || len(got) != 0 {
			t.Errorf("name %q: findings %+v, error %v; want none: the check run is named by the id", written, got, err)
		}
	}
}

func TestAWorkflowThatIsNotAWorkflowIsAFindingNamingIt(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"a list":           "- a\n",
		"a scalar":         "just text\n",
		"no jobs":          "on: pull_request\n",
		"null jobs":        "on: pull_request\njobs:\n",
		"jobs as a list":   "on: pull_request\njobs:\n  - a\n",
		"jobs as a scalar": "on: pull_request\njobs: x\n",
	}

	for name, workflow := range cases {
		got, err := checksIn(t, map[string]string{
			recordPath:                  requiredRecord("QA"),
			".github/workflows/ci.yml":  pullRequestWorkflow,
			".github/workflows/odd.yml": workflow,
		})

		var named bool
		for _, f := range got {
			named = named || (f.Path == ".github/workflows/odd.yml" && strings.Contains(f.Message, "jobs"))
		}
		if err != nil || !named {
			t.Errorf("%s: findings %+v, error %v; want one at the workflow saying it has no jobs", name, got, err)
		}
	}
}

func TestRequiredChecksIsARegisteredRuleWithASummary(t *testing.T) {
	for _, rule := range firstparty.Rules(everythingIsKnown) {
		if rule.ID == firstparty.RequiredChecks {
			if rule.Summary == "" || rule.Check == nil {
				t.Errorf("rule %+v needs a summary and a check", rule)
			}

			return
		}
	}
	t.Errorf("%s is not a registered first-party rule", firstparty.RequiredChecks)
}
