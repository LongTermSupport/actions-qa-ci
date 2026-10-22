package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

// hangEnvironment makes the lane process of this test binary never answer.
const hangEnvironment = "ACTIONS_QA_TEST_LANE_HANGS"

// TestMain turns the test binary into the program a lane is isolated in, when it is started with
// the `lane` command: the isolation tests run the real thing end to end.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "lane" {
		if os.Getenv(hangEnvironment) != "" {
			time.Sleep(time.Minute)
		}
		os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

const cleanWorkflow = `name: ci
on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`

// toolsDirectory is where the development ShellCheck is, as an absolute path, which stays valid
// after a test changes the working directory.
func toolsDirectory(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", "bin"))
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

func realEnv(t *testing.T, isolation *cli.Isolation) cli.Env {
	t.Helper()

	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}
	known := func(id string) bool {
		_, ok := reg.Lookup(id)

		return ok
	}

	return cli.Env{Registry: reg, Pages: actionsqaci.Pages, Lanes: catalogue.Lanes(known), DefaultFormat: "agent", Isolation: isolation}
}

func thisProgram(t *testing.T, limits check.Limits, environment ...string) *cli.Isolation {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	return &cli.Isolation{Executable: executable, Environment: environment, Limits: limits}
}

var roomy = check.Limits{Timeout: time.Minute, MaxOutput: 16 << 20, MaxStderr: 4096}

func runReal(env cli.Env, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.RunWith(env, args, &stdout, &stderr)

	return code, stdout.String(), stderr.String()
}

func TestTheProgramRunsItsActionlintLaneInAChildProcessAndAnswersCleanWhenItIs(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow})

	code, stdout, stderr := run(t, "check", "--format", "agent")

	if code != cli.ExitOK {
		t.Errorf("exit = %d, want %d; stdout %q, stderr %q", code, cli.ExitOK, stdout, stderr)
	}
}

func TestAFindingOfALaneInAChildProcessIsReportedAsIfItWereFoundInProcess(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow + "unknown-key: 1\n"})

	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, roomy)), "check", "--format", "agent")

	if code != cli.ExitFindings || !strings.Contains(stdout, workflowPath) {
		t.Errorf("exit = %d, want %d; stdout %q, stderr %q", code, cli.ExitFindings, stdout, stderr)
	}
}

func TestAWorkflowThatMakesTheLaneProcessCrashHasNoVerdictAndIsNotClean(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	depth := 3_000_000
	repository(t, map[string]string{
		workflowPath: cleanWorkflow + "      - if: ${{ " + strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth) + " }}\n        run: echo again\n",
	})

	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, roomy)), "check", "--format", "agent")

	if code != cli.ExitNoVerdict {
		t.Fatalf("exit = %d, want %d; stdout %.200q, stderr %.400q", code, cli.ExitNoVerdict, stdout, stderr)
	}
	if !strings.Contains(stderr, "no verdict") || !strings.Contains(stderr, workflowPath) || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr should say in one line that there is no verdict for %s: %.600q", workflowPath, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %.200q; a run with no verdict reports nothing as found", stdout)
	}
}

func TestALaneProcessThatRunsPastItsTimeLimitHasNoVerdict(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow})
	limits := check.Limits{Timeout: 700 * time.Millisecond, MaxOutput: 1 << 20, MaxStderr: 4096}

	started := time.Now()
	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, limits, hangEnvironment+"=1")), "check")

	if code != cli.ExitNoVerdict || !strings.Contains(stderr, "time limit") || stdout != "" {
		t.Errorf("exit = %d, want %d; stdout %q, stderr %.400q", code, cli.ExitNoVerdict, stdout, stderr)
	}
	if time.Since(started) > 30*time.Second {
		t.Errorf("took %s: the time limit did not end the lane process", time.Since(started))
	}
}

func TestALaneThatFailsInItsChildProcessIsAFaultOfTheToolchainNotNoVerdict(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", filepath.Join(t.TempDir(), "absent"))
	repository(t, map[string]string{workflowPath: cleanWorkflow})

	code, stdout, stderr := runReal(realEnv(t, thisProgram(t, roomy)), "check")

	if code != cli.ExitToolchainFault || stdout != "" || strings.Contains(stderr, "no verdict") {
		t.Errorf("exit = %d, want %d; stdout %q, stderr %.400q", code, cli.ExitToolchainFault, stdout, stderr)
	}
}

func TestOnlyTheLaneThatRunsAThirdPartyBackendIsIsolated(t *testing.T) {
	isolated := map[string]bool{}
	for _, lane := range realEnv(t, nil).Lanes {
		isolated[lane.Name] = lane.Isolated
	}

	if len(isolated) != 6 || !isolated["actionlint"] {
		t.Errorf("isolated lanes = %v; actionlint runs a third-party parser and is isolated", isolated)
	}
	for _, name := range []string{"firstParty", "templateDrift", "requiredChecks", "inlineSuppression", "nativeSuppressionConfig"} {
		if isolated[name] {
			t.Errorf("lane %s is this program's own code and is not isolated", name)
		}
	}
}

func TestTheLaneCommandRunsOneLaneOnOneFileAndPrintsItsFindingsAsJSON(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow + "unknown-key: 1\n"})

	code, stdout, stderr := runReal(realEnv(t, nil), "lane", "actionlint", workflowPath)

	if code != cli.ExitOK || !strings.HasPrefix(stdout, "[") || !strings.Contains(stdout, `"path": "`+workflowPath+`"`) {
		t.Errorf("exit = %d; stdout %.400q, stderr %.400q", code, stdout, stderr)
	}
}

func TestTheLaneCommandRefusesWhatItCannotRun(t *testing.T) {
	repository(t, map[string]string{workflowPath: cleanWorkflow, "README.md": "text\n"})
	cases := map[string][]string{
		"no arguments":                     {"lane"},
		"no path":                          {"lane", "actionlint"},
		"too many arguments":               {"lane", "actionlint", workflowPath, workflowPath},
		"an unknown lane":                  {"lane", "nonesuch", workflowPath},
		"a path outside":                   {"lane", "actionlint", "../outside.yml"},
		"a file the lane does not analyse": {"lane", "actionlint", "README.md"},
	}

	for name, args := range cases {
		code, stdout, stderr := runReal(realEnv(t, nil), args...)

		if code != cli.ExitUsage || stdout != "" || stderr == "" {
			t.Errorf("%s: exit = %d, stdout %q, stderr %.200q; want exit %d and a message", name, code, stdout, stderr, cli.ExitUsage)
		}
	}
}

func TestTheLaneCommandIsNotPartOfTheUsageText(t *testing.T) {
	code, stdout, stderr := run(t, "help")

	if code != cli.ExitOK {
		t.Fatalf("exit = %d; stderr %q", code, stderr)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "lane ") {
			t.Errorf("help lists the internal lane command: %q", line)
		}
	}
}
