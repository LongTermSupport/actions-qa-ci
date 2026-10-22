package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// onlyTheWorkflow is the one file the fake lanes of these tests cover.
func onlyTheWorkflow(path string) bool { return path == workflowPath }

func TestTheNoteOfUnanalysedFilesCannotBeForgedByAFileName(t *testing.T) {
	evil := ".github/actions/evil\nactions-qa: no lane in this build analyses FORGED\x1b]0;pwned\x07"
	repository(t, map[string]string{evil + "/action.yml": "runs: {}\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", onlyTheWorkflow, findsNothing)), "check")

	if got.code != cli.ExitOK || strings.Contains(got.stderr, "\nactions-qa:") || !strings.HasPrefix(got.stderr, "actions-qa: ") || strings.ContainsAny(got.stderr, "\x1b\x07") ||
		strings.Count(got.stderr, "\n") != 1 {
		t.Errorf("exit %d, stderr %q; one line, naming the file quoted", got.code, got.stderr)
	}
}

func TestTheNoteNamesFilesNoLaneOfTheBuildAnalysesEvenWhenOnlySomeLanesAreChosen(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", "action.yml": "runs: {}\n", ".github/dependabot.yml": "version: 2\n"})
	var run laneRun
	covers := func(p string) bool { return p == "action.yml" }
	env := checkEnv(t, run.lane("alpha", onlyTheWorkflow, findsNothing), run.lane("beta", covers, findsNothing))

	got := runWith(t, env, "check", "-t", "alpha")

	if got.code != cli.ExitOK || strings.Contains(got.stderr, "action.yml") || !strings.Contains(got.stderr, "dependabot.yml") {
		t.Errorf("exit %d, stderr %q; a file another lane of the build covers is not unanalysed, one nobody covers is", got.code, got.stderr)
	}
}

func TestAProjectRecordThatIsNotAPlainFileInsideTheRepositoryIsARefusal(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func() error{
		"a directory":            func() error { return os.MkdirAll(record.Path, 0o700) },
		"a dangling symlink":     func() error { return os.Symlink("nowhere.yml", record.Path) },
		"a link leading outside": func() error { return os.Symlink(outside, record.Path) },
	}

	for name, arrange := range cases {
		repository(t, map[string]string{workflowPath: "on: push\n"})
		if err := arrange(); err != nil {
			t.Fatal(err)
		}
		var run laneRun

		got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

		if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 {
			t.Errorf("%s: exit %d, stdout %q, ran %v; a record that cannot be read as a file means nothing is analysed", name, got.code, got.stdout, run.ran)
		}
	}
}

func TestARecordPathThatCannotBeLookedAtIsAToolchainFault(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := os.RemoveAll(".github"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".github", ".github"); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitToolchainFault || got.stdout != "" || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, ran %v", got.code, got.stdout, got.stderr, run.ran)
	}
}

func TestADiagnosticNobodyCanReadIsAToolchainFaultNotAVerdict(t *testing.T) {
	var run laneRun
	var stdout strings.Builder

	repository(t, map[string]string{"action.yml": "runs: {}\n"})
	code := cli.RunWith(checkEnv(t, run.lane("fake", onlyTheWorkflow, findsNothing)), []string{"check"}, &stdout, brokenWriter{})
	if code != cli.ExitToolchainFault {
		t.Errorf("a note that could not be written: exit %d", code)
	}

	repository(t, map[string]string{".github": "not a directory\n"})
	code = cli.RunWith(checkEnv(t, run.lane("fake", yml, findsNothing)), []string{"check"}, &stdout, brokenWriter{})
	if code != cli.ExitToolchainFault {
		t.Errorf("a no-verdict message that could not be written: exit %d", code)
	}
}
