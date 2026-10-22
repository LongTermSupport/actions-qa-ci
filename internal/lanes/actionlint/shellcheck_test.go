package actionlint_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// shellStep is a workflow whose one step runs the script under bash.
func shellStep(script string) fixture {
	var body strings.Builder
	body.WriteString("    steps:\n      - run: |\n")
	for _, line := range strings.Split(script, "\n") {
		body.WriteString("          " + line + "\n")
	}

	return workflow(pushTrigger, "", body.String())
}

// The step runs under bash -e, as GitHub runs it, so the unguarded rm is a warning and an
// unguarded cd is not reported at all.
const (
	cdUnchecked = `rm -rf $DIR/*`
	cdChecked   = `rm -rf "${DIR:?}"/*`
)

func TestAShellWarningInARunStepIsReportedUnderItsShellcheckCode(t *testing.T) {
	target := inProject(t, shellStep(cdUnchecked))

	found, err := actionlint.Lint(target)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 {
		t.Fatalf("found %+v, want one finding", found)
	}
	f := found[0]
	if f.ID != "actionsqaci.shellcheck.SC2115" || f.UpstreamID != "SC2115" || f.Severity != finding.SeverityWarning {
		t.Errorf("identity or severity: %+v", f)
	}
	if !strings.Contains(f.Tool, "shellcheck v0.11.0") || !strings.Contains(f.Tool, "actionlint "+actionlint.Version) {
		t.Errorf("tool = %q, want both ShellCheck's pinned version and the lane that ran it", f.Tool)
	}
	if f.Path != workflowPath || f.Line < 1 || f.Col < 1 || f.DocPath != "docs/rules/actionsqaci.shellcheck.md" || f.Excepted {
		t.Errorf("position or page: %+v", f)
	}
	if !strings.Contains(f.Message, "${var:?}") || !strings.Contains(f.Message, "script line 1") {
		t.Errorf("message = %q, want the advice and where in the script", f.Message)
	}
	if err := f.Validate(); err != nil {
		t.Errorf("the finding is not valid: %v", err)
	}
}

func TestTheShellcheckVersionIsTheOneTheManifestPins(t *testing.T) {
	manifest, err := toolchain.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, platform := range []string{"linux_amd64", "linux_arm64"} {
		tool, asset, err := manifest.Lookup("shellcheck", platform)
		if err != nil {
			t.Fatal(err)
		}
		if tool.Version != actionlint.ShellcheckVersion || !strings.Contains(asset.URL, "/"+actionlint.ShellcheckVersion+"/") {
			t.Errorf("%s: the manifest pins %s (%s) but the lane says %s", platform, tool.Version, asset.URL, actionlint.ShellcheckVersion)
		}
	}
}

func TestTheCorrectedShellIsClean(t *testing.T) {
	target := inProject(t, shellStep(cdChecked))

	found, err := actionlint.Lint(target)
	if err != nil || len(found) != 0 {
		t.Errorf("found %+v, err %v", found, err)
	}
}

func TestInfoAndStyleFindingsAreBelowTheDefaultThreshold(t *testing.T) {
	// An unquoted variable is SC2086, which ShellCheck rates info.
	target := inProject(t, shellStep(`echo $HOME`))

	found, err := actionlint.Lint(target)
	if err != nil || len(found) != 0 {
		t.Errorf("found %+v, err %v; info is below the warning threshold", found, err)
	}
}

func TestAShellSyntaxErrorIsReportedAsAnError(t *testing.T) {
	target := inProject(t, shellStep("if true; then\necho hi"))

	found, err := actionlint.Lint(target)
	if err != nil {
		t.Fatal(err)
	}

	pattern := regexp.MustCompile(`^actionsqaci\.shellcheck\.SC[0-9]{4}$`)
	var errors int
	for _, f := range found {
		if pattern.MatchString(f.ID) && f.Severity == finding.SeverityError {
			errors++
		}
	}
	if errors == 0 {
		t.Errorf("no ShellCheck error reported for an unclosed if: %+v", found)
	}
}

func TestTheLaneFailsWhenThePinnedShellcheckIsAbsent(t *testing.T) {
	t.Setenv(toolchain.ToolsDirectoryVariable, t.TempDir())
	target := inProject(t, shellStep(cdChecked))

	found, err := actionlint.Lint(target)
	if !errors.Is(err, toolchain.ErrNotFound) {
		t.Errorf("a missing ShellCheck must fail the lane, never silently skip it: %+v, %v", found, err)
	}
}

func TestTheLaneFailsWhenShellcheckIsNotThePinnedBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shellcheck"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolchain.ToolsDirectoryVariable, dir)
	target := inProject(t, shellStep(cdUnchecked))

	found, err := actionlint.Lint(target)
	if !errors.Is(err, toolchain.ErrMismatch) {
		t.Errorf("a ShellCheck that is not the pinned one must fail the lane: %+v, %v", found, err)
	}
}

func TestTheLaneDoesNotUseAShellcheckFromPath(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "shellcheck"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	target := inProject(t, shellStep(cdUnchecked))

	found, err := actionlint.Lint(target)
	if err != nil || len(found) != 1 {
		t.Errorf("the pinned ShellCheck must run, not the one on PATH: %+v, %v", found, err)
	}
}

func TestTheShellcheckFamilyIsDeclaredWithItsMemberPattern(t *testing.T) {
	families := actionlint.Families()

	if len(families) != 1 {
		t.Fatalf("Families = %+v, want one", families)
	}
	family := families[0]
	if family.Name != "actionsqaci.shellcheck.SC<nnnn>" || family.Page != "actionsqaci.shellcheck" || family.Upstream == "" {
		t.Errorf("family = %+v", family)
	}
	for id, want := range map[string]bool{
		"actionsqaci.shellcheck.SC2115": true, "actionsqaci.shellcheck.SC208": false,
		"actionsqaci.actionlint.glob": false, "actionsqaci.shellcheck": false,
	} {
		if family.Member(id) != want {
			t.Errorf("Member(%q) = %v, want %v", id, !want, want)
		}
	}
}

func TestAFamilyMemberChecksOnlyItsOwnCode(t *testing.T) {
	family := actionlint.Families()[0]
	target := inProject(t, shellStep(cdUnchecked))

	own, err := family.Check("actionsqaci.shellcheck.SC2115")(target)
	if err != nil || len(own) != 1 || own[0].ID != "actionsqaci.shellcheck.SC2115" {
		t.Errorf("SC2115 found %+v, err %v", own, err)
	}
	other, err := family.Check("actionsqaci.shellcheck.SC2086")(target)
	if err != nil || len(other) != 0 {
		t.Errorf("SC2086 must not report SC2115's finding: %+v, err %v", other, err)
	}
}

func TestEveryShellcheckCodeResolvesThroughTheRegistryToTheFamilyPage(t *testing.T) {
	reg, err := registry.NewWithFamilies(actionlint.Families())
	if err != nil {
		t.Fatal(err)
	}

	rule, known := reg.Lookup("actionsqaci.shellcheck.SC2115")
	if !known || rule.Check == nil || rule.Upstream == "" {
		t.Errorf("rule = %+v, known = %v", rule, known)
	}
}

const shellcheckMessage = "shellcheck reported issue in this script: SC2164:warning:3:7: Use 'cd ... || exit' in case cd fails."

func TestConvertReadsTheCodeLevelAndScriptPositionOutOfShellchecksMessage(t *testing.T) {
	found, err := actionlint.Convert(workflowPath, []*upstream.Error{{Message: shellcheckMessage, Line: 12, Column: 9, Kind: "shellcheck"}})
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, err %v", found, err)
	}

	f := found[0]
	if f.ID != "actionsqaci.shellcheck.SC2164" || f.Line != 12 || f.Col != 9 || f.Severity != finding.SeverityWarning {
		t.Errorf("finding = %+v", f)
	}
	if f.Message != "Use 'cd ... || exit' in case cd fails. (script line 3, column 7)" {
		t.Errorf("message = %q", f.Message)
	}
}

func TestConvertMapsShellcheckErrorsToErrorsAndDropsWhatIsBelowWarning(t *testing.T) {
	message := func(level string) *upstream.Error {
		return &upstream.Error{Message: "shellcheck reported issue in this script: SC1073:" + level + ":1:1: text", Line: 2, Column: 3, Kind: "shellcheck"}
	}

	for level, want := range map[string]int{"error": 1, "warning": 1, "info": 0, "style": 0} {
		found, err := actionlint.Convert(workflowPath, []*upstream.Error{message(level)})
		if err != nil || len(found) != want {
			t.Errorf("level %s: found %+v, err %v, want %d findings", level, found, err, want)
		}
	}
	found, err := actionlint.Convert(workflowPath, []*upstream.Error{message("error")})
	if err != nil || found[0].Severity != finding.SeverityError {
		t.Errorf("an error level must be an error: %+v, %v", found, err)
	}
}

func TestConvertRefusesAShellcheckMessageItCannotRead(t *testing.T) {
	for _, text := range []string{
		"",
		"shellcheck reported issue in this script: SC2164 something",
		"shellcheck reported issue in this script: SC2164:fatal:1:1: text",
		"shellcheck reported issue in this script: SC21:warning:1:1: text",
		"something else entirely",
	} {
		found, err := actionlint.Convert(workflowPath, []*upstream.Error{{Message: text, Line: 1, Column: 1, Kind: "shellcheck"}})

		if err == nil || !strings.Contains(err.Error(), "shellcheck") {
			t.Errorf("message %q: found %+v, err %v; an unreadable message must be a fault, never a dropped finding", text, found, err)
		}
	}
}
