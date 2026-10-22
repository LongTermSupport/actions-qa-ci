package cli_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

const requiredRecordFor = "version: 1\nrequiredChecks:\n  - context: build\n"

func TestARequiredCheckNothingProducesIsAFindingOfCheck(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": requiredRecordFor,
		".github/workflows/ci.yml":  cleanWorkflow,
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "actionsqaci.requiredChecks") || !strings.Contains(result.stdout, ".github/actions-qa-ci.yml:3") || !strings.Contains(result.stdout, "pull_request") {
		t.Errorf("exit = %d; want a finding at the record's entry saying the workflow is not a pull request one: stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestARequiredCheckAPullRequestJobProducesMakesCheckClean(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": requiredRecordFor,
		".github/workflows/ci.yml":  strings.Replace(cleanWorkflow, "on: push", "on: pull_request", 1),
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitOK {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestAStaleRequiredCheckCanBeAcceptedByAnExceptionOfTheRecord(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	record := requiredRecordFor + "exceptions:\n  - id: actionsqaci.requiredChecks\n    path: .github/actions-qa-ci.yml\n" +
		"    hazard: \"The required job is produced by a workflow that is being migrated and is not yet merged\"\n" +
		"    scope: \"Only this context, until the migration lands on the default branch\"\n" +
		"    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123\n"
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": record,
		".github/workflows/ci.yml":  cleanWorkflow,
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitOK {
		t.Errorf("exit = %d; an accepted finding is counted, not failing: stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestRequiredChecksIsListedByRules(t *testing.T) {
	result := runWith(t, realEnv(t, nil), "rules")

	if result.code != cli.ExitOK || !strings.Contains(result.stdout, "actionsqaci.requiredChecks") {
		t.Errorf("exit %d, %.300q", result.code, result.stdout)
	}
}
