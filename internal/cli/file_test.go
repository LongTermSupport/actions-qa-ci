package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// elsewhere makes the working directory a fresh directory that is in no repository, and returns
// the repository root the test was in before.
func elsewhere(t *testing.T) string {
	t.Helper()

	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	return root
}

// decoded reads a JSON report of findings.
func decoded(t *testing.T, stdout string) []finding.Finding {
	t.Helper()

	var found []finding.Finding
	if err := json.Unmarshal([]byte(stdout), &found); err != nil {
		t.Fatalf("the output is not a JSON list of findings: %v\n%s", err, stdout)
	}

	return found
}

func TestFileChecksOneFileOfTheRepositoryAboveItFromAnyWorkingDirectory(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, ".github/workflows/other.yml": releaseWorkflow})
	root := elsewhere(t)
	away, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", yml, findsAlways)), "check", "--format", "json", "--file", filepath.Join(root, ".github", "workflows", "ci.yml"))

	if got.code != cli.ExitFindings {
		t.Fatalf("exit %d, want %d; stdout %q, stderr %q", got.code, cli.ExitFindings, got.stdout, got.stderr)
	}
	found := decoded(t, got.stdout)
	if len(found) != 1 || found[0].ID != templateDrift || found[0].Path != workflowPath || found[0].Line != 3 || found[0].Excepted {
		t.Errorf("findings %+v, want one unexcepted %s at %s:3", found, templateDrift, workflowPath)
	}
	if strings.Join(runs.ran, " ") != "drift:"+workflowPath {
		t.Errorf("the lanes ran %v, want the named file alone, by its path in the repository", runs.ran)
	}
	if now, err := os.Getwd(); err != nil || now != away {
		t.Errorf("the working directory is %q (%v) after the run, want it left at %q", now, err, away)
	}
}

func TestFileTakesAPathRelativeToTheWorkingDirectory(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, "sub/README": "x"})
	t.Chdir("sub")
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", yml, findsAlways)), "check", "--format", "json", "--file", "../.github/workflows/ci.yml")

	if got.code != cli.ExitFindings || len(decoded(t, got.stdout)) != 1 || decoded(t, got.stdout)[0].Path != workflowPath {
		t.Errorf("exit %d, stdout %q, stderr %q; want one finding at %s", got.code, got.stdout, got.stderr, workflowPath)
	}
}

func TestFileFindsTheNearestRepositoryEvenWhenItsGitIsAWorktreeFile(t *testing.T) {
	inner := "vendor/inner/"
	repository(t, map[string]string{
		workflowPath:          releaseWorkflow,
		inner + ".git":        "gitdir: /nowhere/.git/worktrees/inner\n",
		inner + workflowPath:  releaseWorkflow,
		inner + record.Path:   excepting(entry(templateDrift, workflowPath, "")),
		".github/unused.yml":  "x: 1\n",
		inner + "unused.text": "x",
	})
	root := elsewhere(t)
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", workflowAndRecord, findsAlways)), "check", "--format", "json", "--file", filepath.Join(root, "vendor", "inner", ".github", "workflows", "ci.yml"))

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, want %d (the inner repository's record excepts the finding); stdout %q, stderr %q", got.code, cli.ExitOK, got.stdout, got.stderr)
	}
	found := decoded(t, got.stdout)
	if len(found) != 1 || found[0].Path != workflowPath || !found[0].Excepted {
		t.Errorf("findings %+v, want one excepted finding at %s, the path in the inner repository", found, workflowPath)
	}
}

func TestFileAppliesTheRecordsExceptions(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, record.Path: excepting(entry(templateDrift, workflowPath, ""))})
	root := elsewhere(t)
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", workflowAndRecord, findsAlways)), "check", "--format", "json", "--file", filepath.Join(root, workflowPath))

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, want %d; stdout %q, stderr %q", got.code, cli.ExitOK, got.stdout, got.stderr)
	}
	found := decoded(t, got.stdout)
	if len(found) != 1 || !found[0].Excepted || found[0].ExceptionKey == "" {
		t.Errorf("findings %+v, want the one finding excepted by the record", found)
	}
	if strings.Contains(got.stdout, "staleException") {
		t.Errorf("a one-file run judged an exception stale: %s", got.stdout)
	}
}

func TestFileRefusesAnInvalidRecordOfTheRepositoryItFinds(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow, record.Path: "version: 1\nunknown: 1\n"})
	root := elsewhere(t)
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", workflowAndRecord, findsAlways)), "check", "--file", filepath.Join(root, workflowPath))

	if got.code != cli.ExitUsage || len(runs.ran) != 0 {
		t.Errorf("exit %d, lanes ran %v; want %d with nothing analysed; stderr %q", got.code, runs.ran, cli.ExitUsage, got.stderr)
	}
}

const selfHostedWorkflow = `name: ci
on:
  push:
    paths: ['**.go']
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
jobs:
  build:
    runs-on: [self-hosted, pool-a]
    steps:
      - run: echo hello
`

func TestFileAppliesTheRepositorysDeclaredRunnerLabels(t *testing.T) {
	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))

	for name, c := range map[string]struct {
		files map[string]string
		code  int
	}{
		"labels declared":     {map[string]string{workflowPath: selfHostedWorkflow, ".github/actionlint.yaml": "self-hosted-runner:\n  labels:\n    - pool-a\n"}, cli.ExitOK},
		"no labels declared":  {map[string]string{workflowPath: selfHostedWorkflow}, cli.ExitFindings},
		"another label named": {map[string]string{workflowPath: selfHostedWorkflow, ".github/actionlint.yaml": "self-hosted-runner:\n  labels:\n    - pool-b\n"}, cli.ExitFindings},
	} {
		repository(t, c.files)
		root := elsewhere(t)

		got := runWith(t, realEnv(t, nil), "check", "--format", "json", "--file", filepath.Join(root, workflowPath))

		if got.code != c.code {
			t.Errorf("%s: exit %d, want %d; stdout %q, stderr %q", name, got.code, c.code, got.stdout, got.stderr)

			continue
		}
		for _, f := range decoded(t, got.stdout) {
			if c.code == cli.ExitFindings && f.ID != "actionsqaci.actionlint.runner-label" {
				t.Errorf("%s: finding %+v, want only runner-label", name, f)
			}
		}
	}
}

func TestFileOutsideAnyRepositoryIsRefused(t *testing.T) {
	outside := t.TempDir()
	t.Chdir(outside)
	file := filepath.Join(outside, "ci.yml")
	if err := os.WriteFile(file, []byte(releaseWorkflow), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", yml, findsAlways)), "check", "--format", "json", "--file", file)

	if got.code != cli.ExitUsage || got.stdout != "" || len(runs.ran) != 0 {
		t.Errorf("exit %d, stdout %q, lanes ran %v; want %d, no output and nothing analysed", got.code, got.stdout, runs.ran, cli.ExitUsage)
	}
	if !strings.Contains(got.stderr, "ci.yml") || !strings.Contains(got.stderr, "no .git") {
		t.Errorf("stderr %q does not name the file and say no repository holds it", got.stderr)
	}
}

func TestFileThatDoesNotExistIsRefused(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow})
	runs := &laneRun{}

	got := runWith(t, checkEnv(t, runs.lane("drift", yml, findsAlways)), "check", "--file", ".github/workflows/missing.yml")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "missing.yml") || len(runs.ran) != 0 {
		t.Errorf("exit %d, stderr %q, lanes ran %v; want %d naming the file", got.code, got.stderr, runs.ran, cli.ExitUsage)
	}
}

func TestFileIsGivenOnceWithAPathAndNoOtherPaths(t *testing.T) {
	repository(t, map[string]string{workflowPath: releaseWorkflow})
	runs := &laneRun{}

	for name, args := range map[string][]string{
		"no path":          {"check", "--file"},
		"empty path":       {"check", "--file", ""},
		"twice":            {"check", "--file", workflowPath, "--file", workflowPath},
		"and a path":       {"check", "--file", workflowPath, workflowPath},
		"and a path, --":   {"check", "--file", workflowPath, "--", workflowPath},
		"with --online":    {"check", "--online", "--file", workflowPath},
		"path then --file": {"check", workflowPath, "--file", workflowPath},
	} {
		got := runWith(t, checkEnv(t, runs.lane("drift", yml, findsAlways)), args...)
		if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "--file") {
			t.Errorf("%s: exit %d, stderr %q; want %d naming --file", name, got.code, got.stderr, cli.ExitUsage)
		}
	}
	if len(runs.ran) != 0 {
		t.Errorf("a refused command line analysed %v", runs.ran)
	}
}
