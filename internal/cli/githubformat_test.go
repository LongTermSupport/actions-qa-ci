package cli_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

func TestCheckPrintsWorkflowAnnotationsInTheGitHubFormat(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--format", "github")

	if got.code != cli.ExitFindings || !strings.HasPrefix(got.stdout, "::error file="+workflowPath+",") || !strings.Contains(got.stdout, "title="+templateDrift+"::") || !strings.HasSuffix(got.stdout, "1 finding\n") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheEnvironmentCanNameTheGitHubFormat(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))
	env.DefaultFormat = "github"

	got := runWith(t, env, "check")

	if !strings.HasPrefix(got.stdout, "::error ") {
		t.Errorf("stdout %q", got.stdout)
	}
}
