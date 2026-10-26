package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
)

const concurrencyLimits = "actionsqaci.zizmor.concurrency-limits"

// justifiedConcurrency accepts the finding of concurrency-limits in cleanWorkflow.
const justifiedConcurrency = "exceptions:\n" +
	"  - id: " + concurrencyLimits + "\n" +
	"    path: " + workflowPath + "\n" +
	"    hazard: \"A workflow with no concurrency limit can run many copies of itself at once on one push\"\n" +
	"    scope: \"Only this workflow, which runs once per push and is never queued behind another run of it\"\n" +
	"    decision: https://github.com/LongTermSupport/php-qa-ci/issues/310\n"

// enablingRepository is a repository of one workflow, which states no concurrency limit, and a
// record made of the given text after its version.
func enablingRepository(t *testing.T, record string) {
	t.Helper()
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))

	files := map[string]string{workflowPath: cleanWorkflow, ".github/actions-qa-ci.yml": "version: 1\n" + record}
	repository(t, files)
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, ".github/actions-qa-ci.yml")...)
}

func TestAnAuditTheRegularPersonaLeavesOutIsNotReportedUntilTheRecordEnablesIt(t *testing.T) {
	enablingRepository(t, "")

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAnEnabledAuditIsReportedLikeAnyOtherFinding(t *testing.T) {
	enablingRepository(t, "calibrations:\n  enable:\n    - "+concurrencyLimits+"\n")

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, workflowPath) || !strings.Contains(got.stdout, concurrencyLimits) {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAnEnabledAuditsFindingIsAcceptedOnlyByAJustifiedException(t *testing.T) {
	enablingRepository(t, "calibrations:\n  enable:\n    - "+concurrencyLimits+"\n"+justifiedConcurrency)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitOK || strings.Contains(got.stdout, "stale") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAnExceptionForAnAuditNotEnabledIsStale(t *testing.T) {
	enablingRepository(t, justifiedConcurrency)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "actionsqaci.staleException") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestEnablingAnAuditOfALaneNotRunLeavesItAlone(t *testing.T) {
	enablingRepository(t, "calibrations:\n  enable:\n    - "+concurrencyLimits+"\n")

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent", "-t", "actionlint")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestARecordCannotEnableWhatTheLanesAlreadyRunHoldAsideOrDoNotHave(t *testing.T) {
	cases := map[string]string{
		"an audit that already runs":    "actionsqaci.zizmor.artipacked",
		"an audit this build holds":     "actionsqaci.zizmor.impostor-commit",
		"a rule of another lane":        "actionsqaci.actionlint.glob",
		"a check on the record itself":  "actionsqaci.unjustifiedException",
		"an identifier no rule has":     "actionsqaci.zizmor.nonsense",
		"a family of rules as a member": "actionsqaci.shellcheck.SC2086",
	}

	for name, id := range cases {
		enablingRepository(t, "calibrations:\n  enable:\n    - "+id+"\n")

		for _, command := range [][]string{{"check", "--format", "agent"}, {"rules"}} {
			got := runWith(t, realEnv(t, nil), command...)

			if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "enable entry 1 (line 4)") || !strings.Contains(got.stderr, id) {
				t.Errorf("%s, %s: exit %d, stdout %q, stderr %q; want the record refused at its entry", name, command[0], got.code, got.stdout, got.stderr)
			}
		}
	}
}

func TestTheRulesListShowsWhichDefencesTheRecordEnables(t *testing.T) {
	enablingRepository(t, "calibrations:\n  enable:\n    - "+concurrencyLimits+"\n")

	got := runWith(t, realEnv(t, nil), "rules")

	if got.code != cli.ExitOK || !strings.Contains(got.stdout, "    enabled: "+concurrencyLimits+" is enabled by .github/actions-qa-ci.yml:4\n") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if strings.Count(got.stdout, "    enabled: ") != 1 {
		t.Errorf("only the defence asked for is shown as enabled:\n%s", got.stdout)
	}
}

func TestTheAgentSummaryShowsWhichDefencesTheRecordEnablesAndGoesStaleWithoutIt(t *testing.T) {
	enablingRepository(t, "calibrations:\n  enable:\n    - "+concurrencyLimits+"\n")
	summary := "CLAUDE.md"

	written := runWith(t, realEnv(t, nil), "rules", "--write-agent-summary", summary)
	if written.code != cli.ExitOK {
		t.Fatalf("exit %d, stdout %q, stderr %q", written.code, written.stdout, written.stderr)
	}
	current := runWith(t, realEnv(t, nil), "check", "--format", "agent", "-t", "agentSummaryCurrent")

	if current.code != cli.ExitOK || current.stdout != "0 findings\n" {
		t.Errorf("a summary written from the record is current: exit %d, stdout %q, stderr %q", current.code, current.stdout, current.stderr)
	}
	body, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "  - enabled: "+concurrencyLimits+" is enabled by .github/actions-qa-ci.yml:4\n") {
		t.Errorf("the summary does not show the enabled audit:\n%s", body)
	}

	if err := os.WriteFile(".github/actions-qa-ci.yml", []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := runWith(t, realEnv(t, nil), "check", "--format", "agent", "-t", "agentSummaryCurrent")

	if stale.code != cli.ExitFindings || !strings.Contains(stale.stdout, "actionsqaci.agentSummaryCurrent") {
		t.Errorf("a summary that still shows an audit the record no longer enables is stale: exit %d, stdout %q, stderr %q", stale.code, stale.stdout, stale.stderr)
	}
}
