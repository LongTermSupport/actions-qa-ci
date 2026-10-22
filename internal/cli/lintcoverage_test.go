package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
)

const strayWorkflow = "name: t\non: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n"

// damage changes one byte in the middle of a file.
func damage(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAWorkflowOutsideTheDefaultSetIsAFindingOfCheckEvenWithNoRecord(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "templates/ci.yml": strayWorkflow})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, "templates/ci.yml")...)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "templates/ci.yml:4:1 actionsqaci.lintCoverage") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestADeclarationMakesCheckCleanAndTheLaneCanBeRunAlone(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	record := "version: 1\nanalyse:\n  unanalysed:\n    - path: 'templates/**'\n      justification:\n" +
		"        hazard: \"Templates are copied to consumers and are not run by this project\"\n" +
		"        scope: \"Only the templates directory, which holds no workflow this project runs\"\n"
	repository(t, map[string]string{workflowPath: cleanWorkflow, "templates/ci.yml": strayWorkflow, ".github/actions-qa-ci.yml": record})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, "templates/ci.yml", ".github/actions-qa-ci.yml")...)

	all := runWith(t, realEnv(t, nil), "check", "--format", "agent")
	alone := runWith(t, realEnv(t, nil), "check", "-t", "lintCoverage", "--format", "agent")

	if all.code != cli.ExitOK || alone.code != cli.ExitOK || alone.stdout != "0 findings\n" {
		t.Errorf("all: exit %d, stdout %q; alone: exit %d, stdout %q, stderr %q", all.code, all.stdout, alone.code, alone.stdout, alone.stderr)
	}
}

func TestANamedFileIsNotARunOverTheProject(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "templates/ci.yml": strayWorkflow})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, "templates/ci.yml")...)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent", workflowPath)

	if got.code != cli.ExitOK || strings.Contains(got.stdout, "lintCoverage") {
		t.Errorf("exit %d, stdout %q; checking one file says nothing about the others", got.code, got.stdout)
	}
}

func TestAnIndexThatCannotBeReadIsAFaultNotACleanRun(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath)...)
	damage(t, ".git/index")

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitToolchainFault || got.stdout != "" || !strings.Contains(got.stderr, "git index") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAFileWhoseNameCannotBePrintedIsNoVerdictNamingTheFile(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow, "bad\nname.yml": strayWorkflow})
	gitindextest.Write(t, ".", gitindextest.Files(workflowPath, "bad\nname.yml")...)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitNoVerdict || got.stdout != "" || !strings.Contains(got.stderr, `bad\nname.yml`) || strings.Count(got.stderr, "\n") != 1 {
		t.Errorf("exit %d, stdout %q, stderr %q; a file that cannot be reported is no verdict, as in the default set", got.code, got.stdout, got.stderr)
	}
}

func TestLintCoverageIsListedByRules(t *testing.T) {
	got := runWith(t, realEnv(t, nil), "rules")

	if got.code != cli.ExitOK || !strings.Contains(got.stdout, "actionsqaci.lintCoverage") {
		t.Errorf("exit %d, %.300q", got.code, got.stdout)
	}
}
