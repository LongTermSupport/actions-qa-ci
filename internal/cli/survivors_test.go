package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestADanglingLinkWhereTheWrongRecordNameIsIsStillRefused(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := os.Symlink("nowhere", ".github/actions-qa-ci.yaml"); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitUsage || len(run.ran) != 0 || !strings.Contains(got.stderr, record.Path) {
		t.Errorf("exit %d, ran %v, stderr %q", got.code, run.ran, got.stderr)
	}
}

func TestADotGithubThatLeadsOutsideTheRepositoryIsNoVerdictNotAFaultOrAPass(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "actions-qa-ci.yml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository(t, map[string]string{"README.md": "# x\n"})
	if err := os.Symlink(outside, ".github"); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitNoVerdict || got.stdout != "" || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, ran %v, stderr %q", got.code, got.stdout, run.ran, got.stderr)
	}
}

func TestTheRuleHarnessNamesAnInvalidRecordByItsFileQuotedWhenTheNameIsHostile(t *testing.T) {
	hostile := "rec\x1b[2J\nactions-qa: FORGED.yml"
	repository(t, map[string]string{hostile: "version: 1\nsurprise: 1\n"})

	got := runProgram(t, "rule", "actionsqaci.unjustifiedException", hostile)

	if got.code != cli.ExitUsage || !onlyOurLines(t, got.stderr) || !strings.Contains(got.stderr, `\x1b`) {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestAnErrorThatIsNotAnInvalidRecordIsRefusedWithItsWords(t *testing.T) {
	repository(t, map[string]string{record.Path: "version: 1\n"})

	got := runProgram(t, "rule", "actionsqaci.unjustifiedException", "absent-record.yml")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "absent-record.yml") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}
