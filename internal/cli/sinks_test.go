package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestADiagnosticThatCannotBeWrittenIsAToolchainFaultWhateverKindItIs(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	for name, args := range map[string][]string{
		"a usage error":  {"check", "-x"},
		"a refusal":      {"check", "-t", "nothing-like-it"},
		"a missing file": {"check", "-t", "fake", "missing.yml"},
	} {
		var stdout strings.Builder

		code := cli.RunWith(checkEnv(t, run.lane("fake", yml, findsNothing)), args, &stdout, brokenWriter{})

		if code != cli.ExitToolchainFault || stdout.Len() != 0 {
			t.Errorf("%s: exit %d, stdout %q; a message nobody can read is not a usage error", name, code, stdout.String())
		}
	}

	repository(t, map[string]string{record.Path: "version: 1\nsurprise: 1\n"})
	var stdout strings.Builder
	code := cli.RunWith(checkEnv(t, run.lane("fake", yml, findsNothing)), []string{"check"}, &stdout, brokenWriter{})
	if code != cli.ExitToolchainFault || stdout.Len() != 0 {
		t.Errorf("an invalid record: exit %d, stdout %q", code, stdout.String())
	}
}

func TestEveryKindOfDiagnosticEscapesWhatItEchoes(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	strayID := func(path string) ([]finding.Finding, error) {
		return []finding.Finding{finds("actionsqaci.stray\x1b[2J\nactions-qa: FORGED", path)}, nil
	}

	for name, got := range map[string]result{
		"a usage error": runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", "--x\x1b[2J\nactions-qa: FORGED"),
		"a fault":       runWith(t, checkEnv(t, run.lane("fake", yml, strayID)), "check"),
		"a refusal":     runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", "-t", "x\x1b[2J\nactions-qa: FORGED"),
	} {
		if got.code != cli.ExitUsage && got.code != cli.ExitToolchainFault {
			t.Errorf("%s: exit %d", name, got.code)
		}
		if !strings.HasPrefix(got.stderr, "actions-qa: ") || strings.Contains(got.stderr, "\nactions-qa: FORGED") || strings.ContainsAny(got.stderr, "\x1b") {
			t.Errorf("%s: stderr %q", name, got.stderr)
		}
	}
}

func TestANamedRecordThatIsALinkIsNoVerdictToo(t *testing.T) {
	repository(t, map[string]string{"real.yml": "version: 1\n"})
	if err := os.MkdirAll(".github", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real.yml", record.Path); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", func(string) bool { return true }, findsNothing)), "check", record.Path)

	if got.code != cli.ExitNoVerdict || len(run.ran) != 0 {
		t.Errorf("exit %d, ran %v, stderr %s", got.code, run.ran, got.stderr)
	}
}

func TestARecordWrittenWithTheWrongExtensionIsRefusedNotIgnored(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", ".github/actions-qa-ci.yaml": "version: 1\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 || !strings.Contains(got.stderr, record.Path) {
		t.Errorf("exit %d, stdout %q, ran %v, stderr %q; a record nobody reads must not look like a project without one", got.code, got.stdout, run.ran, got.stderr)
	}
}
