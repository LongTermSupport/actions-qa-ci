package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

type sarifFile struct {
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name  string `json:"name"`
				Rules []struct {
					ID      string `json:"id"`
					HelpURI string `json:"helpUri"`
				} `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID       string `json:"ruleId"`
			Suppressions []struct {
				Kind string `json:"kind"`
			} `json:"suppressions"`
		} `json:"results"`
	} `json:"runs"`
}

func readSARIF(t *testing.T, path string) sarifFile {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed sarifFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}

	return parsed
}

func TestCheckWritesTheFindingsToTheSARIFFileAndStillPrintsThem(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--format", "agent", "--sarif", "out.sarif")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, templateDrift) {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	log := readSARIF(t, "out.sarif")
	if log.Version != "2.1.0" || len(log.Runs) != 1 || len(log.Runs[0].Results) != 1 || log.Runs[0].Results[0].RuleID != templateDrift {
		t.Errorf("log %+v", log)
	}
	rules := log.Runs[0].Tool.Driver.Rules
	if len(rules) != 1 || !strings.HasPrefix(rules[0].HelpURI, "https://github.com/LongTermSupport/actions-qa-ci/") || !strings.HasSuffix(rules[0].HelpURI, "/docs/rules/"+templateDrift+".md") {
		t.Errorf("rules %+v; each carries the address of its page", rules)
	}
	if !strings.Contains(got.stderr, "out.sarif") {
		t.Errorf("stderr %q should say where the log went", got.stderr)
	}
}

func TestACleanRunStillWritesTheSARIFFile(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	got := runWith(t, env, "check", "--sarif", "out.sarif")

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if log := readSARIF(t, "out.sarif"); len(log.Runs) != 1 || len(log.Runs[0].Results) != 0 {
		t.Errorf("log %+v; a clean run must replace an old log with an empty one", log)
	}
}

func TestAnExceptedFindingIsInTheSARIFFileAsSuppressed(t *testing.T) {
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

	got := runWith(t, realEnv(t, nil), "check", "--sarif", "out.sarif")

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	log := readSARIF(t, "out.sarif")
	if len(log.Runs[0].Results) != 1 || len(log.Runs[0].Results[0].Suppressions) != 1 || log.Runs[0].Results[0].Suppressions[0].Kind != "external" {
		t.Errorf("results %+v", log.Runs[0].Results)
	}
}

func TestTheRealProgramNamesTheBundledToolsInTheSARIFFile(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	repository(t, map[string]string{workflowPath: cleanWorkflow})

	got := runWith(t, realEnv(t, nil), "check", "--sarif", "out.sarif")

	data, err := os.ReadFile("out.sarif")
	if got.code != cli.ExitOK || err != nil || !strings.Contains(string(data), `"name": "actionlint"`) || !strings.Contains(string(data), `"name": "shellcheck"`) {
		t.Errorf("exit %d, err %v, log %s", got.code, err, data)
	}
}

func TestSARIFNeedsAFileAndIsGivenOnce(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	for name, args := range map[string][]string{
		"no value":                      {"check", "--sarif"},
		"twice":                         {"check", "--sarif", "a.sarif", "--sarif", "b.sarif"},
		"empty":                         {"check", "--sarif", ""},
		"a flag where the file belongs": {"check", "--sarif", "--format", "json"},
	} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 {
			t.Errorf("%s: exit %d, stdout %q, ran %v", name, got.code, got.stdout, run.ran)
		}
	}
}

func TestNoSARIFFileIsWrittenWhenNothingWasAnalysed(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", ".github/actions-qa-ci.yml": "version: 1\nunknownSection: true\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--sarif", "out.sarif")

	if _, err := os.Lstat("out.sarif"); got.code != cli.ExitUsage || err == nil {
		t.Errorf("exit %d, lstat err %v; a log of an analysis that did not happen must not be written", got.code, err)
	}
}

func TestASymbolicLinkAtTheSARIFPathIsNeverWrittenThrough(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := os.Symlink(outside, "out.sarif"); err != nil {
		t.Fatal(err)
	}
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--sarif", "out.sarif")

	data, err := os.ReadFile(outside)
	if got.code != cli.ExitToolchainFault || got.stdout != "" || err != nil || string(data) != "precious" {
		t.Errorf("exit %d, stdout %q, victim %q, err %v", got.code, got.stdout, data, err)
	}
}

func TestADirectoryLinkThatLeadsOutsideTheRepositoryIsNeverWrittenThrough(t *testing.T) {
	outside := t.TempDir()
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := os.Symlink(outside, "reports"); err != nil {
		t.Fatal(err)
	}
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--sarif", "reports/out.sarif")

	entries, err := os.ReadDir(outside)
	if got.code != cli.ExitToolchainFault || err != nil || len(entries) != 0 {
		t.Errorf("exit %d, err %v, outside holds %d entries", got.code, err, len(entries))
	}
}

func TestADirectoryAtTheSARIFPathIsRefusedForWhatItIs(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	for _, typed := range []string{".", ".github"} {
		got := runWith(t, env, "check", "--sarif", typed)

		if got.code != cli.ExitToolchainFault || got.stdout != "" || !strings.Contains(got.stderr, "directory") || strings.Contains(got.stderr, "link") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want a refusal that says it is a directory", typed, got.code, got.stdout, got.stderr)
		}
	}
}

func TestAPipeAtTheSARIFPathIsRefusedAndDoesNotBlock(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := syscall.Mkfifo("out.sarif", 0o600); err != nil {
		t.Fatal(err)
	}
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	finished := make(chan result, 1)
	go func() { finished <- runWith(t, env, "check", "--sarif", "out.sarif") }()

	select {
	case got := <-finished:
		if got.code != cli.ExitToolchainFault || got.stdout != "" {
			t.Errorf("exit %d, stdout %q", got.code, got.stdout)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("check blocked opening a pipe")
	}
}

func TestAnAbsolutePathOutsideTheRepositoryIsTheOperatorsChoice(t *testing.T) {
	target := filepath.Join(t.TempDir(), "report.sarif")
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	got := runWith(t, env, "check", "--sarif", target)

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if log := readSARIF(t, target); len(log.Runs) != 1 {
		t.Errorf("log %+v", log)
	}
}

func TestAnUnwritableSARIFPathIsAToolchainFaultNotACleanRun(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	got := runWith(t, env, "check", "--sarif", "no-such-directory/out.sarif")

	if got.code != cli.ExitToolchainFault || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; a log that was asked for and not written must not read as success", got.code, got.stdout)
	}
}
