package firstparty_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// The fixtures below write the double star of a filter pattern as %%, which runRule turns back
// into **, so that no pattern in this file reads to a comment scanner as a block comment.
func globs(text string) string { return strings.ReplaceAll(text, "%%", "**") }

// every is the cron step field "every N": a star, a slash and N.
func every(n string) string { return "*" + "/" + n }

// The fixtures of the four trigger-hygiene rules. Each red one breaks exactly one rule and keeps
// the other three, so TestEachTriggerRuleFiresOnItsOwnFixtureAlone can tell them apart.
const (
	greenWorkflow = `on:
  push:
    branches: [main]
    paths:
      - "%%"
      - "!%%.md"
      - "!docs/%%"
      - "%%/CLAUDE.md"
  pull_request:
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	redDocsOnly = `on:
  push:
    branches: [main]
  pull_request:
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	redConcurrency = `on:
  push:
    branches: [main]
    paths-ignore:
      - "%%.md"
      - "docs/%%"
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	redFanout = `on:
  workflow_run:
    workflows: [CI]
    types: [completed]
    branches: [main]
jobs:
  watch:
    if: github.event.workflow_run.conclusion != 'success'
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	greenFanout = `on:
  workflow_run:
    workflows: [CI]
    types: [completed]
jobs:
  watch:
    if: ${{ github.event.workflow_run.conclusion == 'failure' }}
    runs-on: ubuntu-latest
    steps:
      - run: "true"
  report:
    needs: watch
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	redSchedule = `on:
  schedule:
    - cron: "0,30 * * * *"
  workflow_dispatch:
jobs:
  sweep:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
	greenSchedule = `on:
  schedule:
    - cron: "17 * * * *"
  workflow_dispatch:
jobs:
  sweep:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`
)

const oneJob = "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"true\"\n"

func everything(string) bool { return true }

// triggerRules are the four checks, by identifier.
func triggerRules() map[string]func(string) ([]finding.Finding, error) {
	return map[string]func(string) ([]finding.Finding, error){
		firstparty.DocsOnlyPush:      firstparty.CheckDocsOnlyPush(everything),
		firstparty.PushConcurrency:   firstparty.CheckPushConcurrency,
		firstparty.WorkflowRunFanout: firstparty.CheckWorkflowRunFanout,
		firstparty.ScheduleBudget:    firstparty.CheckScheduleBudget(everything),
	}
}

func runRule(t *testing.T, check func(string) ([]finding.Finding, error), files map[string]string) []finding.Finding {
	t.Helper()
	expanded := map[string]string{}
	for name, text := range files {
		expanded[name] = globs(text)
	}
	inTree(t, expanded)
	got, err := check(workflow)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	for _, f := range got {
		if f.Path != workflow {
			t.Errorf("finding %+v is not at the workflow %s", f, workflow)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("finding %+v is not valid: %v", f, err)
		}
	}

	return got
}

// The discrimination proof: each red fixture draws findings from its own rule and from no other,
// and the green fixtures draw none.
func TestEachTriggerRuleFiresOnItsOwnFixtureAlone(t *testing.T) {
	fixtures := map[string]string{
		"green":                      greenWorkflow,
		firstparty.DocsOnlyPush:      redDocsOnly,
		firstparty.PushConcurrency:   redConcurrency,
		firstparty.WorkflowRunFanout: redFanout,
		firstparty.ScheduleBudget:    redSchedule,
		"green fanout":               greenFanout,
		"green schedule":             greenSchedule,
	}

	for fixture, text := range fixtures {
		for id, check := range triggerRules() {
			got := runRule(t, check, map[string]string{workflow: text})
			if want := fixture == id; (len(got) > 0) != want {
				t.Errorf("rule %s on the %s fixture: %d findings %v, want findings = %v", id, fixture, len(got), got, want)
			}
			for _, f := range got {
				if f.ID != id {
					t.Errorf("rule %s reported %s", id, f.ID)
				}
			}
		}
	}
}

func TestDocsOnlyPushNamesTheDocsThatWouldRunAndTheFix(t *testing.T) {
	got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: redDocsOnly})
	if len(got) != 1 {
		t.Fatalf("findings = %v, want one", got)
	}
	f := got[0]
	if f.Line != 2 {
		t.Errorf("line = %d, want 2 (the push trigger)", f.Line)
	}
	for _, want := range []string{"push", "README.md", "no paths or paths-ignore", "paths:", "!**.md", "CLAUDE.md"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q lacks %q", f.Message, want)
		}
	}
}

func TestDocsOnlyPushJudgesTheFilterAsGitHubReadsIt(t *testing.T) {
	cases := map[string]struct {
		on       string
		findings int
		mention  string
	}{
		"no trigger config":                {"on: push\n", 1, "push"},
		"a list of events":                 {"on: [push, pull_request]\n", 1, "push"},
		"a tag push only":                  {"on:\n  push:\n    tags: ['v*']\n", 0, ""},
		"a pull request only":              {"on:\n  pull_request:\n", 0, ""},
		"no push at all":                   {"on: workflow_dispatch\n", 0, ""},
		"markdown excluded but not docs":   {"on:\n  push:\n    paths: ['%%', '!%%.md']\n", 1, "docs/images/diagram.png"},
		"docs excluded but not markdown":   {"on:\n  push:\n    paths-ignore: ['docs/%%']\n", 1, "notes/design.md"},
		"root markdown only excluded":      {"on:\n  push:\n    paths-ignore: ['*.md', 'docs/%%']\n", 1, "notes/design.md"},
		"paths-ignore of docs":             {"on:\n  push:\n    paths-ignore: ['%%.md', 'docs/%%']\n", 0, ""},
		"paths naming the code":            {"on:\n  push:\n    paths: ['src/%%', 'go.mod']\n", 0, ""},
		"a negation re-included too wide":  {"on:\n  push:\n    paths: ['%%', '!%%.md', '!docs/%%', '%%.md']\n", 1, "README.md"},
		"instruction markdown re-included": {"on:\n  push:\n    paths: ['%%', '!%%.md', '!docs/%%', '%%/CLAUDE.md', '%%/AGENTS.md', '.claude/%%', '.github/%%']\n", 0, ""},
	}

	for name, c := range cases {
		got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: c.on + oneJob})
		if len(got) != c.findings {
			t.Errorf("%s: findings %v, want %d", name, got, c.findings)

			continue
		}
		if c.findings > 0 && !strings.Contains(got[0].Message, c.mention) {
			t.Errorf("%s: message %q lacks %q", name, got[0].Message, c.mention)
		}
	}
}

func TestDocsOnlyPushReadsTheRecordForItsEventsAndPaths(t *testing.T) {
	prOnly := "on:\n  pull_request:\n" + oneJob
	recordPR := "version: 1\ndocsOnly:\n  events: [push, pull_request]\n"
	got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: prOnly, record.Path: recordPR})
	if len(got) != 1 || !strings.Contains(got[0].Message, "pull_request") {
		t.Errorf("with pull_request in the record's events: %v, want one finding about pull_request", got)
	}

	readmeOnly := "on:\n  push:\n    paths: ['%%', '!README.md']\n" + oneJob
	recordReadme := "version: 1\ndocsOnly:\n  paths: [README.md]\n"
	if got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: readmeOnly, record.Path: recordReadme}); len(got) != 0 {
		t.Errorf("with the record's own docs paths: %v, want none", got)
	}
	if got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: readmeOnly}); len(got) != 1 {
		t.Errorf("with the default docs paths: %v, want one", got)
	}
}

func TestDocsOnlyPushFailsOnAnInvalidRecordRatherThanUsingDefaults(t *testing.T) {
	inTree(t, map[string]string{workflow: globs(greenWorkflow), record.Path: "version: 1\ndocsOnly:\n  events: []\n"})
	if _, err := firstparty.CheckDocsOnlyPush(everything)(workflow); err == nil {
		t.Error("an invalid record was not an error")
	}
}

func TestPushConcurrencyRequiresAGroupThatCollapsesRunsOnMain(t *testing.T) {
	cases := map[string]struct {
		text     string
		findings int
		mention  string
	}{
		"no concurrency":                 {"on:\n  push:\n    branches: [main]\n" + oneJob, 1, "no concurrency"},
		"push to every branch":           {"on: push\n" + oneJob, 1, "no concurrency"},
		"a group per commit":             {"on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci-push-${{ github.sha }}\n" + oneJob, 1, "github.sha"},
		"a group per run":                {"on: push\nconcurrency: ci-${{ github.run_id }}\n" + oneJob, 1, "github.run_id"},
		"a group by ref":                 {"on: push\nconcurrency:\n  group: ci-${{ github.ref }}\n" + oneJob, 0, ""},
		"a group as a plain string":      {"on: push\nconcurrency: ci-${{ github.ref }}\n" + oneJob, 0, ""},
		"cancel stays the repo's choice": {"on: push\nconcurrency:\n  group: ci-${{ github.ref }}\n  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n" + oneJob, 0, ""},
		"a concurrency with no group":    {"on: push\nconcurrency:\n  cancel-in-progress: true\n" + oneJob, 1, "no group"},
		"pushes to other branches only":  {"on:\n  push:\n    branches: ['release/v1']\n" + oneJob, 0, ""},
		"main ignored":                   {"on:\n  push:\n    branches-ignore: [main]\n" + oneJob, 0, ""},
		"main taken back":                {"on:\n  push:\n    branches: ['%%', '!main']\n" + oneJob, 0, ""},
		"tags only":                      {"on:\n  push:\n    tags: ['v*']\n" + oneJob, 0, ""},
		"no push":                        {"on: pull_request\n" + oneJob, 0, ""},
		"every job has a group":          {"on: push\njobs:\n  a:\n    concurrency: a-${{ github.ref }}\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"true\"\n", 0, ""},
	}

	for name, c := range cases {
		got := runRule(t, firstparty.CheckPushConcurrency, map[string]string{workflow: c.text})
		if len(got) != c.findings {
			t.Errorf("%s: findings %v, want %d", name, got, c.findings)

			continue
		}
		if c.findings > 0 && !strings.Contains(got[0].Message, c.mention) {
			t.Errorf("%s: message %q lacks %q", name, got[0].Message, c.mention)
		}
		if c.findings > 0 && !strings.Contains(got[0].Message, "concurrency:") {
			t.Errorf("%s: message %q does not give the fix", name, got[0].Message)
		}
	}
}

func TestWorkflowRunFanoutRequiresAnAllowlistOfConclusions(t *testing.T) {
	head := "on:\n  workflow_run:\n    workflows: [CI]\n    types: [completed]\njobs:\n"
	job := func(id, extra string) string {
		return "  " + id + ":\n" + extra + "    runs-on: ubuntu-latest\n    steps:\n      - run: \"true\"\n"
	}
	cases := map[string]struct {
		text string
		jobs []string
	}{
		"no gate":                        {head + job("a", ""), []string{"a"}},
		"a denylist":                     {head + job("a", "    if: github.event.workflow_run.conclusion != 'success'\n"), []string{"a"}},
		"an allowlist":                   {head + job("a", "    if: github.event.workflow_run.conclusion == 'failure'\n"), nil},
		"an allowlist written backwards": {head + job("a", "    if: \"'success' == github.event.workflow_run.conclusion\"\n"), nil},
		"an allowlist in an expression":  {head + job("a", "    if: ${{ github.event.workflow_run.conclusion == 'success' && github.event.workflow_run.event == 'push' }}\n"), nil},
		"allow and deny together":        {head + job("a", "    if: github.event.workflow_run.conclusion == 'failure' || github.event.workflow_run.conclusion != 'success'\n"), []string{"a"}},
		"a gate on something else":       {head + job("a", "    if: github.event.workflow_run.event == 'push'\n"), []string{"a"}},
		"a needs chain":                  {head + job("a", "    if: github.event.workflow_run.conclusion == 'failure'\n") + job("b", "    needs: a\n"), nil},
		"a needs chain run always":       {head + job("a", "    if: github.event.workflow_run.conclusion == 'failure'\n") + job("b", "    needs: [a]\n    if: always()\n"), []string{"b"}},
		"only requested runs":            {"on:\n  workflow_run:\n    workflows: [CI]\n    types: [requested]\njobs:\n" + job("a", ""), nil},
		"no workflow_run":                {"on: push\njobs:\n" + job("a", ""), nil},
		"default types":                  {"on:\n  workflow_run:\n    workflows: [CI]\njobs:\n" + job("a", ""), []string{"a"}},
	}

	for name, c := range cases {
		got := runRule(t, firstparty.CheckWorkflowRunFanout, map[string]string{workflow: c.text})
		var jobs []string
		for _, f := range got {
			for _, id := range []string{"a", "b"} {
				if strings.Contains(f.Message, "job "+id+" ") {
					jobs = append(jobs, id)
				}
			}
			if !strings.Contains(f.Message, "cancelled") || !strings.Contains(f.Message, "conclusion == 'failure'") || !strings.Contains(f.Message, record.Path) {
				t.Errorf("%s: message %q does not say the hazard, the fix and where an exception goes", name, f.Message)
			}
		}
		if !slices.Equal(jobs, c.jobs) {
			t.Errorf("%s: findings %v name jobs %q, want %q", name, got, jobs, c.jobs)
		}
	}
}

func TestWorkflowRunFanoutReportsAtTheJobSoAnExceptionCanNameIt(t *testing.T) {
	got := runRule(t, firstparty.CheckWorkflowRunFanout, map[string]string{workflow: redFanout})
	if len(got) != 1 || got[0].Line != 7 {
		t.Errorf("findings = %v, want one at line 7 (the job)", got)
	}
}

func TestScheduleBudgetCountsTheRunsADayOfEveryCron(t *testing.T) {
	tail := "  workflow_dispatch:\n" + oneJob
	schedule := func(crons ...string) string {
		text := "on:\n  schedule:\n"
		for _, c := range crons {
			text += "    - cron: \"" + c + "\"\n"
		}

		return text + tail
	}
	cases := map[string]struct {
		text     string
		findings int
		mention  string
	}{
		"hourly":                     {schedule("17 * * * *"), 0, ""},
		"every six hours":            {schedule("41 " + every("6") + " * * *"), 0, ""},
		"daily":                      {schedule("23 5 * * *"), 0, ""},
		"weekly":                     {schedule("29 4 * * 1"), 0, ""},
		"every half hour":            {schedule(every("30") + " * * * *"), 1, "48 runs a day"},
		"every five minutes":         {schedule(every("5") + " * * * *"), 1, "288 runs a day"},
		"two minutes an hour":        {schedule("7,37 * * * *"), 1, "48 runs a day"},
		"a range of minutes":         {schedule("0-4 9 * * *"), 0, ""},
		"a range with a step":        {schedule("0-59/20 8-17 * * 1-5"), 1, "30 runs a day"},
		"two crons adding up":        {schedule("7 "+every("2")+" * * *", "37 "+every("2")+" * * *"), 0, ""},
		"two crons over the budget":  {schedule("7 * * * *", "37 * * * *"), 1, "48 runs a day"},
		"a start with a step":        {schedule("5/15 * * * *"), 1, "96 runs a day"},
		"a cron that cannot be read": {schedule("@hourly"), 1, "cannot be read"},
		"no schedule":                {"on: push\n" + oneJob, 0, ""},
	}

	for name, c := range cases {
		got := runRule(t, firstparty.CheckScheduleBudget(everything), map[string]string{workflow: c.text})
		if len(got) != c.findings {
			t.Errorf("%s: findings %v, want %d", name, got, c.findings)

			continue
		}
		if c.findings > 0 && (!strings.Contains(got[0].Message, c.mention) || !strings.Contains(got[0].Message, record.Path)) {
			t.Errorf("%s: message %q lacks %q or the record", name, got[0].Message, c.mention)
		}
	}
}

func TestScheduleBudgetReadsTheBudgetFromTheRecord(t *testing.T) {
	sixHourly := "on:\n  schedule:\n    - cron: \"41 " + every("6") + " * * *\"\n" + oneJob
	got := runRule(t, firstparty.CheckScheduleBudget(everything), map[string]string{workflow: sixHourly, record.Path: "version: 1\nscheduleBudget:\n  maxRunsPerDay: 2\n"})
	if len(got) != 1 || !strings.Contains(got[0].Message, "budget of 2") {
		t.Fatalf("findings = %v, want one naming the budget of 2", got)
	}
	if got[0].Line != 3 {
		t.Errorf("line = %d, want 3 (the cron)", got[0].Line)
	}
}

// A workflow that is not YAML, or not a mapping, is actionsqaci.actionlint.syntax-check's to
// report (one owner per hazard class); the trigger rules have nothing to judge in it.
func TestATriggerRuleLeavesAWorkflowThatIsNotYAMLToTheSyntaxCheck(t *testing.T) {
	for id, check := range triggerRules() {
		for _, text := range []string{"on: [\n", "- a list\n", ""} {
			if got := runRule(t, check, map[string]string{workflow: text}); len(got) != 0 {
				t.Errorf("%s on %q: findings %v, want none", id, text, got)
			}
		}
	}
}

func TestATriggerRuleFailsOnAWorkflowItCannotOpen(t *testing.T) {
	for id, check := range triggerRules() {
		inTree(t, map[string]string{"other.yml": "x: 1\n"})
		if _, err := check(workflow); err == nil {
			t.Errorf("%s: a missing workflow was not an error", id)
		}
	}
}
