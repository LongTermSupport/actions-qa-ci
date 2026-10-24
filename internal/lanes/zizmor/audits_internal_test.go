package zizmor

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func auditsOf(findings []finding.Finding) []string {
	var names []string
	for _, f := range findings {
		if !slices.Contains(names, f.UpstreamID) {
			names = append(names, f.UpstreamID)
		}
	}
	slices.Sort(names)

	return names
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func TestEveryAuditThisBuildRunsHasAFixturePairAndEveryPairIsForOne(t *testing.T) {
	var running []string
	for _, a := range Audits() {
		if a.Held == "" {
			running = append(running, a.Name)
		}
	}
	slices.Sort(running)

	if got, want := strings.Join(sortedKeys(fixtures), ","), strings.Join(running, ","); got != want {
		t.Errorf("fixtures cover\n%s\nthe audits this build runs are\n%s", got, want)
	}
}

func TestEachRedFixtureFiresItsAuditAndNothingElseItDidNotSay(t *testing.T) {
	for _, name := range sortedKeys(fixtures) {
		p := fixtures[name]
		target := inProject(t, p.red)

		found, err := lintAs(target, personaAuditor)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}
		want := append([]string{name}, p.alsoFires...)
		slices.Sort(want)
		if got := auditsOf(found); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: the red fixture reported %v, want %v", name, got, want)
		}
		for _, f := range found {
			if f.ID != ID(f.UpstreamID) {
				t.Errorf("%s: finding has identifier %s, want %s", name, f.ID, ID(f.UpstreamID))
			}
		}
	}
}

func TestEachGreenFixtureIsCleanAtTheWidestPersona(t *testing.T) {
	for _, name := range sortedKeys(fixtures) {
		target := inProject(t, fixtures[name].green)

		found, err := lintAs(target, personaAuditor)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}
		if len(found) != 0 {
			t.Errorf("%s: the green fixture reported %+v", name, found)
		}
	}
}

func TestTheLanesOwnPersonaReportsAnAuditExactlyWhenItsPairSaysSo(t *testing.T) {
	for _, name := range sortedKeys(fixtures) {
		p := fixtures[name]
		target := inProject(t, p.red)

		found, err := Lint(target)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}
		if reported := slices.Contains(auditsOf(found), name); reported != p.atDefault {
			t.Errorf("%s: the lane's persona reported it = %v, the pair says %v", name, reported, p.atDefault)
		}
	}
}

func TestARuleShowsWhatItsAuditFindsWhateverThePersona(t *testing.T) {
	for _, name := range sortedKeys(fixtures) {
		var check func(string) ([]finding.Finding, error)
		for _, rule := range Rules() {
			if rule.ID == ID(name) {
				check = rule.Check
			}
		}
		if check == nil {
			t.Errorf("%s has no rule", name)

			continue
		}
		target := inProject(t, fixtures[name].red)

		found, err := check(target)

		if err != nil || len(found) == 0 {
			t.Errorf("%s: the rule showed %+v, %v", name, found, err)
		}
		for _, f := range found {
			if f.UpstreamID != name {
				t.Errorf("%s: the rule showed a finding of %s", name, f.UpstreamID)
			}
		}
	}
}

func TestAnAuditThisBuildDoesNotRunSaysWhyInsteadOfShowingNothing(t *testing.T) {
	held := 0
	for _, a := range Audits() {
		if a.Held == "" {
			continue
		}
		held++
		for _, rule := range Rules() {
			if rule.ID != ID(a.Name) {
				continue
			}
			target := inProject(t, clean())

			found, err := rule.Check(target)

			if err == nil || !strings.Contains(err.Error(), a.Held) || found != nil {
				t.Errorf("%s: got %+v, %v; want the reason it is held", a.Name, found, err)
			}
		}
	}
	if held == 0 {
		t.Error("no audit is held, so this test checks nothing")
	}
}

func TestEveryAuditIsOneThePinnedZizmorKnowsByThatName(t *testing.T) {
	binary, err := pinnedZizmor()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range Audits() {
		if !bytes.Contains(content, []byte(a.Name)) {
			t.Errorf("the pinned zizmor does not contain the audit name %q; it may have been renamed or removed", a.Name)
		}
	}
}

func TestACleanWorkflowIsCleanAtTheWidestPersona(t *testing.T) {
	target := inProject(t, clean())

	found, err := lintAs(target, personaAuditor)

	if err != nil || len(found) != 0 {
		t.Errorf("got %+v, %v; the base of every fixture must have nothing for any audit", found, err)
	}
}

func TestAFileZizmorCannotParseIsAFaultAndNotACleanRun(t *testing.T) {
	target := inProject(t, single("name: x\non: push\njobs: [this is not a mapping\n"))

	found, err := Lint(target)

	if err == nil || found != nil {
		t.Errorf("got %+v, %v; want a fault", found, err)
	}
}

func TestAProjectConfigurationAndAnIgnoreCommentChangeNothing(t *testing.T) {
	source := header + jobWith("", "      - run: echo ${{ github.event.pull_request.title }} # zizmor: ignore[template-injection]\n")
	fx := fixture{target: workflowPath, files: map[string]string{
		workflowPath:                source,
		".github/zizmor.yml":        "rules:\n  template-injection:\n    disable: true\n",
		"zizmor.yml":                "rules:\n  template-injection:\n    disable: true\n",
		".github/workflows/zz.yaml": source,
	}}
	target := inProject(t, fx)

	found, err := Lint(target)

	if err != nil || !slices.Contains(auditsOf(found), "template-injection") {
		t.Errorf("got %+v, %v; the finding must stand whatever the file or the comment says", found, err)
	}
}

func TestTheCallersTokenAndConfigurationNeverReachZizmor(t *testing.T) {
	t.Setenv("GH_TOKEN", "a-token")
	t.Setenv("ZIZMOR_CONFIG", "/nowhere/at/all.yml")
	t.Setenv("ZIZMOR_OFFLINE", "0")
	target := inProject(t, clean())

	found, err := Lint(target)

	if err != nil || len(found) != 0 {
		t.Errorf("got %+v, %v", found, err)
	}
}
