package cli_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

const templateRecord = "version: 1\ntemplates:\n  - source: templates/qa.yml\n    copies: [.github/workflows/qa.yml]\n"

func TestACopyThatDriftedFromItsTemplateIsAFindingOfCheck(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": templateRecord,
		"templates/qa.yml":          cleanWorkflow,
		".github/workflows/qa.yml":  cleanWorkflow + "# edited by hand\n",
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "actionsqaci.templateDrift") || !strings.Contains(result.stdout, ".github/workflows/qa.yml") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestIdenticalCopiesMakeCheckClean(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": templateRecord,
		"templates/qa.yml":          cleanWorkflow,
		".github/workflows/qa.yml":  cleanWorkflow,
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitOK {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestADeletedCopyIsAFindingAndNotSilence(t *testing.T) {
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": templateRecord,
		"templates/qa.yml":          cleanWorkflow,
		".github/workflows/ci.yml":  cleanWorkflow,
	})
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))

	result := runWith(t, realEnv(t, nil), "check", "-t", "templateDrift", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "missing") || !strings.Contains(result.stdout, ".github/actions-qa-ci.yml") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestADriftCanBeAcceptedByAnExceptionOfTheRecord(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	record := templateRecord + "exceptions:\n  - id: actionsqaci.templateDrift\n    path: .github/workflows/qa.yml\n" +
		"    hazard: \"The dogfood copy pins the workflow to a branch the template cannot name\"\n" +
		"    scope: \"Only this copy, which is regenerated from the template by hand at each release\"\n" +
		"    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123\n"
	repository(t, map[string]string{
		".github/actions-qa-ci.yml": record,
		"templates/qa.yml":          cleanWorkflow,
		".github/workflows/qa.yml":  cleanWorkflow + "# differs\n",
	})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitOK {
		t.Errorf("exit = %d; an accepted drift is counted, not failing: stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestTemplateDriftIsListedByRules(t *testing.T) {
	result := runWith(t, realEnv(t, nil), "rules")

	if result.code != cli.ExitOK || !strings.Contains(result.stdout, "actionsqaci.templateDrift") {
		t.Errorf("exit %d, %.300q", result.code, result.stdout)
	}
}
