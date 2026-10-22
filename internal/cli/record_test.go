package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

// withRecord makes a repository holding the given project record the working directory.
func withRecord(t *testing.T, record string) {
	t.Helper()

	root := t.TempDir()
	full := filepath.Join(root, ".github", "actions-qa-ci.yml")
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
}

func runProgram(t *testing.T, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestARecordWithAMistakeInItIsAUsageErrorNotAToolchainFault(t *testing.T) {
	for name, record := range map[string]string{
		"an unknown field":      "version: 1\nsurprise: true\n",
		"an unknown identifier": "version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n",
		"no version":            "exceptions: []\n",
	} {
		withRecord(t, record)

		got := runProgram(t, "rule", "actionsqaci.unjustifiedException", ".github/actions-qa-ci.yml")

		if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "not valid") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; a mistake in the record is exit %d and analyses nothing", name, got.code, got.stdout, got.stderr, cli.ExitUsage)
		}
	}
}

func TestAnUnjustifiedExceptionIsAFindingAndAJustifiedRecordIsClean(t *testing.T) {
	withRecord(t, "version: 1\nexceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: legacy\n    scope: legacy\n    decision: later\n")
	got := runProgram(t, "rule", "--format", "agent", "actionsqaci.unjustifiedException", ".github/actions-qa-ci.yml")
	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "actionsqaci.unjustifiedException") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	withRecord(t, "version: 1\n")
	got = runProgram(t, "rule", "actionsqaci.unjustifiedException", ".github/actions-qa-ci.yml")
	if got.code != cli.ExitOK {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}
