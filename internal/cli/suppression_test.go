package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

func TestACommentThatSuppressesAFindingIsAFindingOfCheck(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow + "# zizmor: ignore[artipacked]\n"})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "actionsqaci.inlineSuppression") || !strings.Contains(result.stdout, workflowPath) {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestAShellCheckDisableInARunScriptIsAFindingOfCheck(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	script := func(directive string) string {
		return strings.Replace(cleanWorkflow, "      - run: echo hello\n", "      - run: |\n"+directive+"          rm -rf \"$DIR\"/*\n", 1)
	}

	repository(t, map[string]string{workflowPath: script("")})
	control := runWith(t, realEnv(t, nil), "check", "--format", "agent")
	if control.code != cli.ExitFindings || !strings.Contains(control.stdout, "actionsqaci.shellcheck.SC2115") {
		t.Fatalf("without the directive ShellCheck should report the unsafe rm: exit = %d; stdout %.400q; stderr %.300q", control.code, control.stdout, control.stderr)
	}

	repository(t, map[string]string{workflowPath: script("          # shellcheck disable=SC2115\n")})
	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "actionsqaci.inlineSuppression") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
	if strings.Contains(result.stdout, "actionsqaci.shellcheck") {
		t.Errorf("the directive was honoured by ShellCheck, so only the guard reports: %.400q", result.stdout)
	}
}

func TestAConfigurationFileThatSuppressesIsAFindingOfCheckAndNotNamedAsUnanalysed(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, ".github/zizmor.yml": "rules: {}\n"})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "actionsqaci.nativeSuppressionConfig") || !strings.Contains(result.stdout, ".github/zizmor.yml") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
	if strings.Contains(result.stderr, "zizmor.yml") {
		t.Errorf("a file the guard covers is not one nothing analyses: %.300q", result.stderr)
	}
}

const cleanAction = "name: x\ndescription: y\nruns:\n  using: composite\n  steps: []\n"

func TestAnActionFileOnlyTheGuardReadsIsStillNamedAsNotLinted(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "action.yml": cleanAction})
	env := realEnv(t, nil)
	env.Lanes = slices.DeleteFunc(slices.Clone(env.Lanes), func(lane check.Lane) bool { return lane.Name == "zizmor" })

	result := runWith(t, env, "check", "--format", "agent")

	if result.code != cli.ExitOK || !strings.Contains(result.stderr, "no lane in this build analyses action.yml") {
		t.Errorf("exit = %d; stderr %.300q; the guard is not a lint of the action, so it is still named", result.code, result.stderr)
	}
}

func TestAnActionFileIsAuditedByZizmorAndSoNotNamedAsNotLinted(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "action.yml": cleanAction})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitOK || strings.Contains(result.stderr, "action.yml") {
		t.Errorf("exit = %d; stderr %.300q; an action zizmor audits is not one no lane analyses", result.code, result.stderr)
	}
}

func TestAnActionZizmorFindsSomethingInIsAFindingOfCheck(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "action.yml": "name: x\ndescription: y\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v4\n"})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "action.yml:") || !strings.Contains(result.stdout, "actionsqaci.zizmor.unpinned-uses") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestACommentInAnActionFileIsFoundByTheGuard(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "action.yml": "name: x # ratchet:exclude\ndescription: y\nruns:\n  using: composite\n  steps: []\n"})

	result := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "action.yml") || !strings.Contains(result.stdout, "actionsqaci.inlineSuppression") {
		t.Errorf("exit = %d; stdout %.400q; stderr %.300q", result.code, result.stdout, result.stderr)
	}
}

func TestOnlyTheNamedGuardRunsWithT(t *testing.T) {
	repository(t, map[string]string{workflowPath: cleanWorkflow + "# ratchet:exclude\n", "zizmor.yml": "rules: {}\n"})

	result := runWith(t, realEnv(t, nil), "check", "-t", "nativeSuppressionConfig", "--format", "agent")

	if result.code != cli.ExitFindings || !strings.Contains(result.stdout, "zizmor.yml") || strings.Contains(result.stdout, "inlineSuppression") {
		t.Errorf("exit = %d; stdout %.400q", result.code, result.stdout)
	}
}

func TestTheGuardsAreListedByRules(t *testing.T) {
	result := runWith(t, realEnv(t, nil), "rules")

	for _, id := range []string{"actionsqaci.inlineSuppression", "actionsqaci.nativeSuppressionConfig"} {
		if result.code != cli.ExitOK || !strings.Contains(result.stdout, id) {
			t.Errorf("rules does not list %s: exit %d, %.300q", id, result.code, result.stdout)
		}
	}
}
