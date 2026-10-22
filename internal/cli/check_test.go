package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const workflowPath = ".github/workflows/ci.yml"

// repository makes a directory that is a repository root holding these files the working
// directory.
func repository(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
}

// laneRun notes what a fake lane was asked to analyse.
type laneRun struct{ ran []string }

func (r *laneRun) lane(name string, selects func(string) bool, found func(path string) ([]finding.Finding, error)) check.Lane {
	return check.Lane{
		Name:    name,
		Selects: selects,
		Run: func(path string) ([]finding.Finding, error) {
			r.ran = append(r.ran, name+":"+path)

			return found(path)
		},
	}
}

func yml(path string) bool { return strings.HasSuffix(path, ".yml") }

func findsAlways(path string) ([]finding.Finding, error) {
	return []finding.Finding{finds(templateDrift, path)}, nil
}

func findsNothing(string) ([]finding.Finding, error) { return nil, nil }

func checkEnv(t *testing.T, lanes ...check.Lane) cli.Env {
	t.Helper()

	env := envWith(t, twoRules()...)
	env.Lanes = lanes

	return env
}

func TestCheckRunsTheLanesOverTheDefaultSetAndExitsWithFindings(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", "README.md": "# x\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check")

	if got.code != cli.ExitFindings {
		t.Fatalf("exit %d; stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, workflowPath+":3:5 "+templateDrift+" workflow drifted\n") || !strings.HasSuffix(got.stdout, "1 finding\n") {
		t.Errorf("stdout:\n%s", got.stdout)
	}
	if !slices.Equal(run.ran, []string{"fake:" + workflowPath}) {
		t.Errorf("ran %v; only the discovered workflow is analysed", run.ran)
	}
}

func TestCheckOfACleanRepositoryExitsZero(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestCheckExitsZeroWhenEveryFindingIsExcepted(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	excepted := func(path string) ([]finding.Finding, error) {
		f := finds(templateDrift, path)
		f.Excepted, f.ExceptionKey = true, "key"

		return []finding.Finding{f}, nil
	}

	got := runWith(t, checkEnv(t, run.lane("fake", yml, excepted)), "check")

	if got.code != cli.ExitOK {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestCheckRefusesToRunAnywhereButTheRepositoryRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, workflowPath), []byte("on: push\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, ".git") || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, ran %v; a directory that is not the root must not read as clean", got.code, got.stdout, got.stderr, run.ran)
	}
}

func TestCheckValidatesTheRecordFirstAndAnalysesNothingWhenItIsInvalid(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", record.Path: "version: 1\nsurprise: true\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsAlways)), "check")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "not valid") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if len(run.ran) != 0 {
		t.Errorf("ran %v; an invalid record means nothing is analysed", run.ran)
	}
}

func TestCheckAcceptsARecordNamingAnIdentifierTheRegistryHas(t *testing.T) {
	repository(t, map[string]string{
		workflowPath: "on: push\n",
		record.Path:  "version: 1\nexceptions:\n  - id: " + templateDrift + "\n    hazard: a\n    scope: b\n    decision: c\n",
	})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", "-t", "fake")

	if got.code != cli.ExitOK {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestCheckLimitsItselfToTheLanesNamedWithT(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	cases := map[string][]string{
		"one":              {"-t", "alpha"},
		"a list":           {"-t", "alpha,beta"},
		"repeated":         {"-t", "alpha", "-t", "beta"},
		"named twice":      {"-t", "alpha,alpha"},
		"after the others": {"-t", "beta"},
	}
	want := map[string][]string{
		"one":              {"alpha:" + workflowPath},
		"a list":           {"alpha:" + workflowPath, "beta:" + workflowPath},
		"repeated":         {"alpha:" + workflowPath, "beta:" + workflowPath},
		"named twice":      {"alpha:" + workflowPath},
		"after the others": {"beta:" + workflowPath},
	}

	for name, flags := range cases {
		var run laneRun
		env := checkEnv(t, run.lane("alpha", yml, findsNothing), run.lane("beta", yml, findsNothing), run.lane("gamma", yml, findsNothing))

		got := runWith(t, env, append([]string{"check"}, flags...)...)

		if got.code != cli.ExitOK || !slices.Equal(run.ran, want[name]) {
			t.Errorf("%s: exit %d, ran %v, want %v; stderr %s", name, got.code, run.ran, want[name], got.stderr)
		}
	}
}

func TestCheckRefusesALaneItDoesNotHaveAndNamesTheOnesItDoes(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("alpha", yml, findsNothing), run.lane("beta", yml, findsNothing)), "check", "-t", "alpah")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "alpah") ||
		!strings.Contains(got.stderr, "alpha, beta") || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, ran %v", got.code, got.stdout, got.stderr, run.ran)
	}
}

func TestCheckOfNamedFilesAnalysesOnlyThose(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", ".github/workflows/other.yml": "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsAlways)), "check", "./"+workflowPath)

	if got.code != cli.ExitFindings || !slices.Equal(run.ran, []string{"fake:" + workflowPath}) {
		t.Errorf("exit %d, ran %v; stderr %s", got.code, run.ran, got.stderr)
	}
}

func TestCheckOfAFileNoLaneAnalysesIsAUsageErrorNotAQuietPass(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n", "README.md": "# x\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", "README.md")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "README.md") || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, ran %v", got.code, got.stdout, got.stderr, run.ran)
	}
}

func TestCheckOfAFileTheChosenLanesDoNotAnalyseIsRefusedToo(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("alpha", yml, findsNothing), run.lane("txt", func(p string) bool { return strings.HasSuffix(p, ".txt") }, findsNothing))

	got := runWith(t, env, "check", "-t", "txt", workflowPath)

	if got.code != cli.ExitUsage || len(run.ran) != 0 {
		t.Errorf("exit %d, ran %v, stderr %s", got.code, run.ran, got.stderr)
	}
}

func TestCheckRefusesPathsItCannotUse(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	outside := filepath.Join(t.TempDir(), "x.yml")
	if err := os.WriteFile(outside, []byte("on: push\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"absent": ".github/workflows/absent.yml", "a directory": ".github", "outside": outside, "parent": "../x.yml"} {
		var run laneRun

		got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", path)

		if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 {
			t.Errorf("%s: exit %d, stdout %q, ran %v", name, got.code, got.stdout, run.ran)
		}
	}
}

func TestCheckStopsWithAToolchainFaultWhenALaneFailsAndPrintsNoFindings(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	failing := func(string) ([]finding.Finding, error) { return nil, errors.New("tool exploded") }

	got := runWith(t, checkEnv(t, run.lane("alpha", yml, findsAlways), run.lane("beta", yml, failing)), "check")

	if got.code != cli.ExitToolchainFault || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q; a failed run has no verdict and prints none", got.code, got.stdout)
	}
	for _, want := range []string{"beta", workflowPath, "tool exploded"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q lacks %q", got.stderr, want)
		}
	}
}

func TestCheckTakesAnInvalidRecordFoundByALaneAsAUsageError(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	invalid := func(string) ([]finding.Finding, error) { return nil, record.ErrInvalid }

	got := runWith(t, checkEnv(t, run.lane("fake", yml, invalid)), "check")

	if got.code != cli.ExitUsage || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestCheckReportsNoVerdictForAFileItCannotReadAsAFile(t *testing.T) {
	repository(t, map[string]string{"target.yml": "on: push\n"})
	if err := os.MkdirAll(".github/workflows", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../target.yml", workflowPath); err != nil {
		t.Fatal(err)
	}
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.code != cli.ExitNoVerdict || got.stdout != "" || !strings.Contains(got.stderr, workflowPath) || len(run.ran) != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, ran %v; a file that was not analysed must never read as clean", got.code, got.stdout, got.stderr, run.ran)
	}
}

func TestCheckPrintsInTheFormatAskedForAndByDefaultInTheOneTheEnvironmentNames(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsAlways))

	got := runWith(t, env, "check", "--format", "json")
	var parsed []finding.Finding
	if err := json.Unmarshal([]byte(got.stdout), &parsed); err != nil || len(parsed) != 1 || got.code != cli.ExitFindings {
		t.Errorf("json: exit %d, err %v, stdout %q", got.code, err, got.stdout)
	}

	env.DefaultFormat = "json"
	got = runWith(t, env, "check")
	if err := json.Unmarshal([]byte(got.stdout), &parsed); err != nil {
		t.Errorf("the environment's format was not used: %v, stdout %q", err, got.stdout)
	}

	got = runWith(t, env, "check", "--format", "agent")
	if strings.HasPrefix(got.stdout, "[") {
		t.Errorf("a format on the command line must win over the environment's: %q", got.stdout)
	}
}

func TestCheckRefusesAFormatItDoesNotHave(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	env := checkEnv(t, run.lane("fake", yml, findsNothing))

	got := runWith(t, env, "check", "--format", "sarif")
	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "sarif") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	env.DefaultFormat = "sarif"
	got = runWith(t, env, "check")
	if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 {
		t.Errorf("a format the environment names must be refused the same way: exit %d, stdout %q, ran %v", got.code, got.stdout, run.ran)
	}
}

func TestCheckRefusesWhatItDoesNotUnderstand(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	cases := map[string][]string{
		"an unknown flag":         {"check", "--sarif", "out.sarif"},
		"-t without a value":      {"check", "-t"},
		"an empty lane name":      {"check", "-t", ""},
		"an empty name in a list": {"check", "-t", "fake,"},
		"--format without value":  {"check", "--format"},
	}

	for name, args := range cases {
		var run laneRun

		got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), args...)

		if got.code != cli.ExitUsage || got.stdout != "" || len(run.ran) != 0 {
			t.Errorf("%s: exit %d, stdout %q, ran %v", name, got.code, got.stdout, run.ran)
		}
	}
}

func TestCheckTakesAFileNamedLikeAFlagAfterTheDoubleDash(t *testing.T) {
	repository(t, map[string]string{"-odd.yml": "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check", "--", "-odd.yml")

	if got.code != cli.ExitOK || !slices.Equal(run.ran, []string{"fake:-odd.yml"}) {
		t.Errorf("exit %d, ran %v, stderr %s", got.code, run.ran, got.stderr)
	}
}

func TestCheckIsAToolchainFaultWhenItsOutputCannotBeWritten(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	var stderr strings.Builder

	code := cli.RunWith(checkEnv(t, run.lane("fake", yml, findsAlways)), []string{"check"}, brokenWriter{}, &stderr)

	if code != cli.ExitToolchainFault {
		t.Errorf("exit %d; output nobody could read is not a verdict", code)
	}
}

func TestCheckIsListedInTheUsage(t *testing.T) {
	got := runWith(t, checkEnv(t), "help")

	if !strings.Contains(got.stdout, "check [-t") {
		t.Errorf("usage lacks check:\n%s", got.stdout)
	}
}

func TestCheckIsAToolchainFaultWhenALaneReportsAnIdentifierNoRuleHas(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun
	stray := func(path string) ([]finding.Finding, error) {
		return []finding.Finding{finds("actionsqaci.zizmor.not-a-rule", path)}, nil
	}

	got := runWith(t, checkEnv(t, run.lane("fake", yml, stray)), "check")

	if got.code != cli.ExitToolchainFault || got.stdout != "" || !strings.Contains(got.stderr, "actionsqaci.zizmor.not-a-rule") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheProgramsOwnCheckRunsTheRecordLaneOnTheRecordOfARepository(t *testing.T) {
	repository(t, map[string]string{
		record.Path: "version: 1\nexceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: legacy\n    scope: legacy\n    decision: later\n",
	})

	got := runProgram(t, "check", "-t", "firstParty")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, "actionsqaci.unjustifiedException") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheProgramsOwnCheckOfARepositoryWithNothingToAnalyseIsClean(t *testing.T) {
	repository(t, map[string]string{"README.md": "# x\n"})

	got := runProgram(t, "check")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheProgramsOwnCheckNamesItsLanesWhenOneIsMistyped(t *testing.T) {
	repository(t, map[string]string{"README.md": "# x\n"})

	got := runProgram(t, "check", "-t", "zizmor")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "actionlint, firstParty") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestTheProgramsOwnCheckReadsTheFormatFromTheEnvironment(t *testing.T) {
	repository(t, map[string]string{"README.md": "# x\n"})

	t.Setenv("ACTIONS_QA_FORMAT", "json")
	got := runProgram(t, "check")
	if got.code != cli.ExitOK || got.stdout != "[]\n" {
		t.Errorf("json: exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	t.Setenv("ACTIONS_QA_FORMAT", "bogus")
	got = runProgram(t, "check")
	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "bogus") {
		t.Errorf("bogus: exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	got = runProgram(t, "check", "--format", "agent")
	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("a format on the command line must win over a wrong one in the environment: exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestCheckIsNoVerdictWhenDotGithubIsAFileWhateverElseIsAskedFor(t *testing.T) {
	for name, args := range map[string][]string{"by default": {"check"}, "for a named file": {"check", "README.md"}} {
		repository(t, map[string]string{".github": "not a directory\n", "README.md": "# x\n"})
		var run laneRun
		env := checkEnv(t, run.lane("fake", func(string) bool { return true }, findsNothing))

		got := runWith(t, env, args...)

		if name == "by default" && (got.code != cli.ExitNoVerdict || got.stdout != "") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; the record cannot be looked for, which is no verdict", name, got.code, got.stdout, got.stderr)
		}
		if name == "for a named file" && got.code == cli.ExitOK && len(run.ran) == 0 {
			t.Errorf("%s: exit %d without analysing anything", name, got.code)
		}
	}
}

func TestCheckNamesTheDiscoveredFilesNoLaneAnalysesInsteadOfPassingThemQuietly(t *testing.T) {
	repository(t, map[string]string{
		workflowPath:                   "on: push\n",
		"action.yml":                   "runs: {}\n",
		".github/dependabot.yaml":      "version: 2\n",
		".github/actions/x/action.yml": "runs: {}\n",
	})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", func(p string) bool { return p == workflowPath }, findsNothing)), "check")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Fatalf("exit %d, stdout %q; the verdict itself does not change", got.code, got.stdout)
	}
	for _, want := range []string{"action.yml", ".github/dependabot.yaml", ".github/actions/x/action.yml", "no lane"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q does not name %q", got.stderr, want)
		}
	}
	if strings.Contains(got.stderr, workflowPath) {
		t.Errorf("stderr %q names a file that was analysed", got.stderr)
	}
}

func TestCheckSaysNothingAboutSkippedFilesWhenEveryFileWasAnalysed(t *testing.T) {
	repository(t, map[string]string{workflowPath: "on: push\n"})
	var run laneRun

	got := runWith(t, checkEnv(t, run.lane("fake", yml, findsNothing)), "check")

	if got.stderr != "" {
		t.Errorf("stderr = %q", got.stderr)
	}
}
