package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestTheBelowRootNoteIsGivenFromAnyDepthAndEscapesTheRootsName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evil\x1b[2J\nactions-qa: FORGED")
	for _, dir := range []string{".git", ".github/workflows"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(record.Path)), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, ".github", "workflows"))

	got := runWith(t, familyEnv(t, twoRules()...), "rules")

	if got.code != cli.ExitOK || !strings.Contains(got.stderr, "repository root") || !onlyOurLines(t, got.stderr) {
		t.Errorf("exit %d, stderr %q; two levels down must be noticed, and the name must not forge a line", got.code, got.stderr)
	}
}

func TestAFindingThatCannotBeReportedIsJudgedBeforeAnExceptionTriesToReadItsFile(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry(templateDrift, "gone.yml", "    job: release\n")),
	})
	var run laneRun
	badLine := func(path string) ([]finding.Finding, error) {
		if path == record.Path {
			return nil, nil
		}
		f := finds(templateDrift, "gone.yml")
		f.Line = 0

		return []finding.Finding{f}, nil
	}

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, badLine)), "check")

	if got.code != cli.ExitToolchainFault || !strings.Contains(got.stderr, "a lane reported a finding that cannot be reported") ||
		strings.Contains(got.stderr, "cannot read") {
		t.Errorf("exit %d, stderr %q; the finding is invalid whatever its file holds", got.code, got.stderr)
	}
}

func TestAFindingThatCannotBeReportedIsAFaultNamedAsSuchBeforeAnyExceptionReadsAnything(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: releaseWorkflow,
		record.Path:  excepting(entry(templateDrift, workflowPath, "")),
	})
	var run laneRun
	outside := func(path string) ([]finding.Finding, error) {
		if path == record.Path {
			return nil, nil
		}
		f := finds(templateDrift, path)
		f.Path = "../outside.yml"

		return []finding.Finding{f}, nil
	}

	got := runWith(t, checkEnv(t, run.lane("fake", workflowAndRecord, outside)), "check")

	if got.code != cli.ExitToolchainFault || got.stdout != "" || !strings.Contains(got.stderr, "a lane reported a finding that cannot be reported") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}
