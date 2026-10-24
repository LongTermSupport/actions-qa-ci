package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

func laneThatDoesNotRun(run *laneRun) check.Lane {
	lane := run.lane("alpha", yml, findsNothing)
	lane.NotRun = []check.NotRun{
		{ID: "actionsqaci.zizmor.impostor-commit", Reason: "it needs the network"},
		{ID: "actionsqaci.zizmor.forbidden-uses", Reason: "it needs a policy"},
	}

	return lane
}

func TestCheckNamesEachAuditAChosenLaneDoesNotRunAndWhy(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run)), "check")

	want := "actions-qa: the alpha lane does not run actionsqaci.zizmor.impostor-commit: it needs the network\n" +
		"actions-qa: the alpha lane does not run actionsqaci.zizmor.forbidden-uses: it needs a policy\n"
	if got.code != cli.ExitOK || got.stdout != "0 findings\n" || got.stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q; want the skipped audits named on stderr and the verdict unchanged", got.code, got.stdout, got.stderr)
	}
}

func TestCheckSaysNothingOfALaneItWasNotAskedToRun(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	other := run.lane("beta", yml, findsNothing)

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run), other), "check", "-t", "beta")

	if got.code != cli.ExitOK || strings.Contains(got.stderr, "does not run") {
		t.Errorf("exit %d, stderr %q; a lane that did not run has nothing to say", got.code, got.stderr)
	}
}

func TestCheckNamesTheSkippedAuditsWhenItIsGivenFiles(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run)), "check", workflowPath)

	if got.code != cli.ExitOK || strings.Count(got.stderr, "does not run") != 2 {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestTheNoteIsPrintedWhicheverFileOfTheRunTheLaneAnalysed(t *testing.T) {
	first, last := ".github/workflows/a.yml", ".github/workflows/z.yml"
	for name, only := range map[string]string{"first": first, "last": last} {
		repository(t, map[string]string{first: "on: push\n", last: "on: push\n"})
		var run laneRun
		lane := laneThatDoesNotRun(&run)
		lane.Selects = func(path string) bool { return path == only }

		got := runWith(t, checkEnv(t, lane), "check")

		if strings.Count(got.stderr, "does not run") != 2 {
			t.Errorf("%s file only: stderr %q; the lane looked at a file and has audits to skip", name, got.stderr)
		}
	}
}

func TestEveryChosenLaneThatAnalysedAFileNamesItsOwnSkippedAudits(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	second := run.lane("beta", yml, findsNothing)
	second.NotRun = []check.NotRun{{ID: "actionsqaci.zizmor.stale-action-refs", Reason: "it needs the network"}}

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run), second), "check")

	if !strings.Contains(got.stderr, "the alpha lane does not run actionsqaci.zizmor.forbidden-uses") || !strings.Contains(got.stderr, "the beta lane does not run actionsqaci.zizmor.stale-action-refs") {
		t.Errorf("stderr %q; each lane speaks for itself", got.stderr)
	}
}

func TestTheNoteDoesNotChangeWhatTheSarifLogHolds(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	log := filepath.Join(t.TempDir(), "out.sarif")

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run)), "check", "--sarif", log)

	if got.code != cli.ExitOK || strings.Count(got.stderr, "does not run") != 2 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	written, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "does not run") {
		t.Errorf("the SARIF log carries the diagnostic note: %s", written)
	}
}

func TestCheckSaysNothingOfALaneThatAnalysedNoFile(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", "README.md": "# x\n"})
	var run laneRun
	lane := laneThatDoesNotRun(&run)
	lane.Selects = func(path string) bool { return path == "never.yml" }

	got := runWith(t, checkEnv(t, lane), "check")

	if strings.Contains(got.stderr, "does not run") {
		t.Errorf("stderr %q; a lane that looked at nothing has no audits to skip", got.stderr)
	}
}

func TestTheNoteIsPrintedWhenThereAreFindingsToo(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	lane := laneThatDoesNotRun(&run)
	lane.Run = findsAlways

	got := runWith(t, checkEnv(t, lane), "check")

	if got.code != cli.ExitFindings || strings.Count(got.stderr, "does not run") != 2 {
		t.Errorf("exit %d, stderr %q; findings are when a reader most needs to know what was not run", got.code, got.stderr)
	}
}

func TestTheNoteCannotCarryAnotherLineOrAnEscapeSequence(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	hostile := "\nactions-qa: FORGED\x1b]0;pwned\x07"
	for _, field := range []string{"name", "identifier", "reason"} {
		lane := run.lane("alpha", yml, findsNothing)
		lane.NotRun = []check.NotRun{{ID: "actionsqaci.zizmor.impostor-commit", Reason: "it needs the network"}}
		switch field {
		case "name":
			lane.Name = "alpha" + hostile
		case "identifier":
			lane.NotRun[0].ID += hostile
		default:
			lane.NotRun[0].Reason += hostile
		}

		got := runWith(t, checkEnv(t, lane), "check")

		if got.code != cli.ExitOK || !onlyOurLines(t, got.stderr) || strings.Contains(got.stderr, "FORGED\n") || strings.Count(got.stderr, "\n") != 1 {
			t.Errorf("%s: exit %d, stderr %q; the note is one line whatever the lane's words hold", field, got.code, got.stderr)
		}
	}
}

func TestCheckIsAToolchainFaultWhenTheNoteCannotBeWritten(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	var stdout strings.Builder

	code := cli.RunWith(checkEnv(t, laneThatDoesNotRun(&run)), []string{"check"}, &stdout, brokenWriter{})

	if code != cli.ExitToolchainFault {
		t.Errorf("exit %d; a note nobody could read was not delivered", code)
	}
}

func TestACleanRunStillNamesWhatWasNotRunSoCleanIsNotReadAsEverything(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run)), "check", "--format", "json")

	if got.code != cli.ExitOK || !onlyOurLines(t, got.stderr) {
		t.Errorf("exit %d, stderr %q; every diagnostic line starts with the program's name", got.code, got.stderr)
	}
}
