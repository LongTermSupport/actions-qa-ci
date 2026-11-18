package cli_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

// A ledger that has to see every conclusion of CI, on a schedule faster than hourly: two
// trigger-hygiene findings, each accepted by an exception of the record with its reason.
const ledgerWorkflow = `name: ledger
on:
  workflow_run:
    workflows: [CI]
    types: [completed]
  schedule:
    - cron: "0,30 * * * *"
permissions: {}
jobs:
  record:
    runs-on: ubuntu-latest
    steps:
      - run: echo "$CONCLUSION"
        env:
          CONCLUSION: ${{ github.event.workflow_run.conclusion }}
`

const ledgerExceptions = "version: 1\nexceptions:\n" +
	"  - id: actionsqaci.workflowRunFanout\n    path: .github/workflows/ci.yml\n    job: record\n" +
	"    hazard: \"Every cancelled or skipped CI run starts a ledger job on a runner\"\n" +
	"    scope: \"Only the record job, whose ledger must hold the conclusion of every run\"\n" +
	"    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123\n" +
	"  - id: actionsqaci.scheduleBudget\n    path: .github/workflows/ci.yml\n" +
	"    hazard: \"The ledger workflow starts forty-eight scheduled runs a day\"\n" +
	"    scope: \"Only the ledger workflow, while the half-hourly reconciliation is trialled\"\n" +
	"    decision: https://github.com/LongTermSupport/php-qa-ci/issues/124\n"

func TestTriggerFindingsAreReportedAndAcceptedByExceptionsOfTheRecord(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))

	repository(t, map[string]string{".github/workflows/ci.yml": ledgerWorkflow})
	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")
	if result.code != cli.ExitFindings {
		t.Fatalf("without exceptions: exit = %d, want findings; stdout %.600q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
	for _, want := range []string{"ci.yml:10:3 actionsqaci.workflowRunFanout", "ci.yml:7:1 actionsqaci.scheduleBudget"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("stdout %q lacks %q", result.stdout, want)
		}
	}

	repository(t, map[string]string{".github/workflows/ci.yml": ledgerWorkflow, ".github/actions-qa-ci.yml": ledgerExceptions})
	result = runWith(t, realEnv(t, nil), "check", "--format", "agent")
	if result.code != cli.ExitOK || !strings.Contains(result.stdout, "2 excepted") {
		t.Errorf("with exceptions: exit = %d, want clean with 2 excepted; stdout %.600q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}
