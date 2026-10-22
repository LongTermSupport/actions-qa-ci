package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const releaseWorkflow = `name: Release
on: push
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Tag
        run: echo tag
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: echo verify
`

// workflowAndRecord is every file of these tests' repositories, so that no file goes unanalysed.
func workflowAndRecord(path string) bool { return path == workflowPath || path == record.Path }

// excepting makes a record with these exception entries.
func excepting(entries ...string) string {
	return "version: 1\nexceptions:\n" + strings.Join(entries, "")
}

func entry(id, path, extra string) string {
	text := "  - id: " + id + "\n"
	if path != "" {
		text += "    path: " + path + "\n"
	}

	return text + extra + "    hazard: h\n    scope: s\n    decision: d\n"
}

// findsAtLine is a lane that reports a drift finding at a line of the file.
func findsAtLine(line int) func(string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		if path == record.Path {
			return nil, nil
		}
		f := finds(templateDrift, path)
		f.Line = line

		return []finding.Finding{f}, nil
	}
}

func TestAnExceptionTurnsAFindingIntoACountedOneAndTheRunIsClean(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, record.Path: excepting(entry(templateDrift, workflowPath, ""))})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, findsAtLine(8))), "check")

	if got.code != cli.ExitOK || got.stdout != "0 findings (1 excepted)\n" || got.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAFindingNoExceptionCoversStaysAFindingBesideOneThatIs(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow, ".github/workflows/other.yml": releaseWorkflow,
		record.Path: excepting(entry(templateDrift, workflowPath, "")),
	})
	var run laneRun
	both := func(p string) bool { return strings.HasPrefix(p, ".github/workflows/") || p == record.Path }

	got := runWith(t, checkEnv(t, run.lane("fake", both, findsAtLine(8))), "check")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, ".github/workflows/other.yml:8") ||
		strings.Contains(got.stdout, workflowPath+":8") || !strings.HasSuffix(got.stdout, "1 finding (1 excepted)\n") {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestAJobAndStepNarrowTheExceptionToThePlaceItNames(t *testing.T) {
	for name, c := range map[string]struct {
		extra string
		line  int
		want  int
	}{
		"the job it names":            {"    job: release\n", 8, cli.ExitOK},
		"the step it names":           {"    job: release\n    step: Tag\n", 8, cli.ExitOK},
		"another job":                 {"    job: verify\n", 8, cli.ExitFindings},
		"another step":                {"    job: release\n    step: Other\n", 8, cli.ExitFindings},
		"a line in the other job":     {"    job: release\n", 12, cli.ExitFindings},
		"the job, from its own place": {"    job: verify\n", 12, cli.ExitOK},
	} {
		repository(t, map[string]string{workflowPath: releaseWorkflow, record.Path: excepting(entry(templateDrift, workflowPath, c.extra))})
		var run laneRun

		got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, findsAtLine(c.line))), "check")

		if got.code != c.want {
			t.Errorf("%s: exit %d, want %d; stdout %q", name, got.code, c.want, got.stdout)
		}
	}
}

func TestAnExceptionWithNoPathDisablesTheDefenceInEveryFile(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow, ".github/workflows/other.yml": releaseWorkflow,
		record.Path: excepting(entry(templateDrift, "", "")),
	})
	var run laneRun
	both := func(p string) bool { return strings.HasPrefix(p, ".github/workflows/") || p == record.Path }

	got := runWith(t, checkEnv(t, run.lane("fake", both, findsAtLine(8))), "check")

	if got.code != cli.ExitOK || got.stdout != "0 findings (2 excepted)\n" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestAnExceptionThatMatchesNothingInAFullRunIsAFindingAtItsOwnLine(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry(templateDrift, workflowPath, ""), entry(templateInj, workflowPath, "")),
	})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, findsAtLine(8))), "check")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, record.Path+":8:1 actionsqaci.staleException exception 2 (") ||
		!strings.Contains(got.stdout, "rule-doc actionsqaci.staleException\n") || !strings.HasSuffix(got.stdout, "1 finding (1 excepted)\n") {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestNoExceptionIsStaleInARunThatWasNotAFullOne(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry(templateInj, workflowPath, "")),
	})
	var run laneRun
	env := checkEnv(t, run.lane("fake", workflowAndRecord, findsNothing), run.lane("other", func(string) bool { return false }, findsNothing))

	for name, args := range map[string][]string{
		"one lane":    {"check", "-t", "fake"},
		"named files": {"check", workflowPath},
	} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitOK || strings.Contains(got.stdout, "staleException") {
			t.Errorf("%s: exit %d, stdout %q; what a partial run did not see cannot be called stale", name, got.code, got.stdout)
		}
	}

	got := runWith(t, env, "check")
	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "staleException") {
		t.Errorf("the full run: exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestTheJSONFormatCarriesWhatExceptedAFinding(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, record.Path: excepting(entry(templateDrift, workflowPath, ""))})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, findsAtLine(8))), "check", "--format", "json")

	var parsed []finding.Finding
	if err := json.Unmarshal([]byte(got.stdout), &parsed); err != nil {
		t.Fatalf("%v\n%s", err, got.stdout)
	}
	if got.code != cli.ExitOK || len(parsed) != 1 || !parsed[0].Excepted || parsed[0].ExceptionKey != record.Path+":3" {
		t.Errorf("exit %d, findings %+v", got.code, parsed)
	}
}

func TestAFindingThatCannotBePlacedBecauseItsFileCannotBeReadIsAFaultNotAPass(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry(templateDrift, "gone.yml", "    job: release\n")),
	})
	var run laneRun
	gone := func(path string) ([]finding.Finding, error) {
		if path == record.Path {
			return nil, nil
		}

		return []finding.Finding{finds(templateDrift, "gone.yml")}, nil
	}

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, gone)), "check")

	if got.code != cli.ExitToolchainFault || got.stdout != "" || !strings.Contains(got.stderr, "gone.yml") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheRecordCannotExceptTheChecksOnTheRecord(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry("actionsqaci.unjustifiedException", "", "")),
	})
	var run laneRun
	env := envWith(t, append(twoRules(), registry.Rule{ID: "actionsqaci.unjustifiedException", Summary: "s"})...)
	env.Lanes = []check.Lane{run.lane("fake", workflowAndRecord, findsNothing)}

	got := runWith(t, env, "check")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "cannot be excepted") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}
