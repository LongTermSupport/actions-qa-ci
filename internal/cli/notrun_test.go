package cli_test

import (
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

func TestACleanRunStillNamesWhatWasNotRunSoCleanIsNotReadAsEverything(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, laneThatDoesNotRun(&run)), "check", "--format", "json")

	if got.code != cli.ExitOK || !onlyOurLines(t, got.stderr) {
		t.Errorf("exit %d, stderr %q; every diagnostic line starts with the program's name", got.code, got.stderr)
	}
}
