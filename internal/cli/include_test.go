package cli_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
)

const includeRecord = "version: 1\nanalyse:\n  include:\n    - 'templates/**'\n"

const includedAction = "name: a\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: echo hi # zizmor: ignore[template-injection]\n      shell: bash\n"

func includingRepository(t *testing.T, files map[string]string) {
	t.Helper()
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))

	files[workflowPath] = cleanWorkflow
	files[".github/actions-qa-ci.yml"] = includeRecord
	repository(t, files)

	tracked := []string{}
	for path := range files {
		tracked = append(tracked, path)
	}
	gitindextest.Write(t, ".", gitindextest.Files(tracked...)...)
}

func TestAnIncludedWorkflowIsLintedByActionlintInItsChildProcess(t *testing.T) {
	includingRepository(t, map[string]string{"templates/ci.yml": cleanWorkflow + "unknown-key: 1\n"})

	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, roomy)), "check", "--format", "agent")

	if code != cli.ExitFindings || !strings.Contains(stdout, "templates/ci.yml") {
		t.Errorf("exit = %d, want %d; stdout %q, stderr %q", code, cli.ExitFindings, stdout, stderr)
	}
}

func TestACleanIncludedWorkflowIsCleanAndNeedsNoDeclaration(t *testing.T) {
	includingRepository(t, map[string]string{"templates/ci.yml": cleanWorkflow})

	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, roomy)), "check", "--format", "agent")

	if code != cli.ExitOK || stdout != "0 findings\n" {
		t.Errorf("exit = %d; stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestAnIncludedFileCanBeNamedOnTheCommandLineAndRunByLaneAlone(t *testing.T) {
	includingRepository(t, map[string]string{"templates/ci.yml": cleanWorkflow + "unknown-key: 1\n"})

	named := runWith(t, realEnv(t, nil), "check", "--format", "agent", "templates/ci.yml")
	alone := runWith(t, realEnv(t, nil), "check", "-t", "actionlint", "--format", "agent")

	for name, got := range map[string]result{"named": named, "lane alone": alone} {
		if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "templates/ci.yml") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", name, got.code, got.stdout, got.stderr)
		}
	}
}

func TestAFileNotIncludedStillCannotBeNamed(t *testing.T) {
	includingRepository(t, map[string]string{"other/ci.yml": cleanWorkflow})

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent", "other/ci.yml")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "no lane analyses") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAnIncludedActionIsGuardedAgainstSuppressionAndNamedAsNotLinted(t *testing.T) {
	includingRepository(t, map[string]string{"templates/lib/action.yml": includedAction})

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "templates/lib/action.yml:6") || !strings.Contains(got.stdout, "inlineSuppression") {
		t.Errorf("exit %d, stdout %q, stderr %q; the suppression comment in an included action must be found", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "no lane in this build analyses templates/lib/action.yml") {
		t.Errorf("stderr %q; an action no lane lints must be named, so a clean result is not read as covering it", got.stderr)
	}
}

func TestAnIncludedPatternThatNamesNothingFailsTheRun(t *testing.T) {
	includingRepository(t, map[string]string{"elsewhere/ci.yml": cleanWorkflow + "unknown-key: 1\n"})

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, ".github/actions-qa-ci.yml:4:1 actionsqaci.lintCoverage") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAnIncludedFileThatIsNotTrackedIsNotAnalysedAndTheRunSaysSo(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, ".github/actions-qa-ci.yml": includeRecord, "templates/new.yml": cleanWorkflow + "unknown-key: 1\n"})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, ".github/actions-qa-ci.yml")...)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || strings.Contains(got.stdout, "templates/new.yml") || !strings.Contains(got.stdout, "includes no") {
		t.Errorf("exit %d, stdout %q, stderr %q; a file not yet added is not seen, and the pattern naming nothing is reported", got.code, got.stdout, got.stderr)
	}
}
