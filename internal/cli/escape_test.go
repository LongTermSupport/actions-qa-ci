package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// onlyOurLines says whether every line of a diagnostic is one this program wrote: it starts
// with the program's name, or is a bullet of a list the program printed. A name from the
// repository must not be able to add a line of its own, and nothing in it may move a terminal.
func onlyOurLines(t *testing.T, stderr string) bool {
	t.Helper()

	if strings.ContainsAny(stderr, "\x1b\x07‮\u0085 ") {
		return false
	}
	for _, line := range strings.Split(strings.TrimSuffix(stderr, "\n"), "\n") {
		if !strings.HasPrefix(line, "actions-qa:") && !strings.HasPrefix(line, "  - ") {
			return false
		}
	}

	return true
}

func TestAHostileKeyInTheRecordCannotForgeALineOfTheDiagnostic(t *testing.T) {
	for name, key := range map[string]string{
		"an escape and a line break": `"a\eb\nFORGED"`,
		"a next-line character":      `"k\u0085FORGED"`,
		"a line separator":           `"k FORGED"`,
		"a bell":                     `"k\aFORGED"`,
	} {
		repository(t, map[string]string{record.Path: "version: 1\n" + key + ": 1\n"})
		var run laneRun

		got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

		if got.code != cli.ExitUsage || got.stdout != "" || !onlyOurLines(t, got.stderr) || !strings.Contains(got.stderr, "not valid") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", name, got.code, got.stdout, got.stderr)
		}
	}
}

func TestAHostileKeyInTheRecordIsEscapedByTheRuleHarnessToo(t *testing.T) {
	repository(t, map[string]string{record.Path: "version: 1\n\"a\\eb\\nFORGED\": 1\n"})

	got := runProgram(t, "rule", "actionsqaci.unjustifiedException", record.Path)

	if got.code != cli.ExitUsage || !onlyOurLines(t, got.stderr) {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestAPathTypedOrGloboutOfTheRepositoryCannotForgeALineOfTheDiagnostic(t *testing.T) {
	hostile := ".github/actions/x\nactions-qa: FORGED\x1b[31m/action.yml"
	repository(t, map[string]string{hostile: "runs: {}\n", workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", onlyTheWorkflow, findsNothing))

	for name, args := range map[string][]string{
		"a file no lane analyses": {"check", hostile},
		"a file that is absent":   {"check", hostile + "absent"},
		"a file outside":          {"check", "../" + hostile},
		"a directory":             {"check", ".github/actions/x\nactions-qa: FORGED\x1b[31m"},
	} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitUsage || got.stdout != "" || !onlyOurLines(t, got.stderr) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", name, got.code, got.stdout, got.stderr)
		}
	}

	for name, path := range map[string]string{"absent": "a\nactions-qa: FORGED\x1b[31m.yml", "a hostile name that exists": hostile} {
		got := runWith(t, harnessEnv(t, findsAlways), "rule", templateDrift, path)

		if got.code == cli.ExitOK || !onlyOurLines(t, got.stderr) {
			t.Errorf("rule, %s: exit %d, stderr %q", name, got.code, got.stderr)
		}
	}
}

func TestADanglingGitLinkIsNotARepositoryRoot(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	if err := os.RemoveAll(".git"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nowhere", ".git"); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitUsage || len(run.ran) != 0 {
		t.Errorf("exit %d, ran %v, stderr %s", got.code, run.ran, got.stderr)
	}
}

func TestCheckSaysWhyItRefusesEachKindOfBadArgument(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	for want, args := range map[string][]string{
		"unknown flag": {"check", "-x"},
		"empty":        {"check", "-t", " "},
		"needs":        {"check", "-t"},
	} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitUsage || !strings.Contains(got.stderr, want) {
			t.Errorf("%v: exit %d, stderr %q; it should say %q", args, got.code, got.stderr, want)
		}
	}

	env.DefaultFormat = "bogus"
	got := runWith(t, env, "check")
	if !strings.Contains(got.stderr, "default output format") {
		t.Errorf("stderr %q; a wrong default must be told apart from a wrong --format", got.stderr)
	}
}

func TestTheNoteListsTheFilesItNamesSeparatedByCommas(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", "action.yml": "runs: {}\n", ".github/dependabot.yml": "version: 2\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", onlyTheWorkflow, findsNothing)), "check")

	if !strings.Contains(got.stderr, ".github/dependabot.yml, action.yml") {
		t.Errorf("stderr = %q", got.stderr)
	}
}
