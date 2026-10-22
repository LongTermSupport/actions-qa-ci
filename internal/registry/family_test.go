package registry_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

var shellcheckMember = regexp.MustCompile(`^actionsqaci\.shellcheck\.SC[0-9]{4}$`)

// codesFamily is a family whose check reports, for the identifier it was asked about, one
// finding on the path, so a test can see which identifier reached the check.
func codesFamily() registry.Family {
	return registry.Family{
		Name:     "actionsqaci.shellcheck.SC<nnnn>",
		Page:     "actionsqaci.shellcheck",
		Summary:  "Shell in run: steps is clean of ShellCheck warnings and errors",
		Upstream: "shellcheck v0.11.0",
		Member:   shellcheckMember.MatchString,
		Check: func(id string) func(string) ([]finding.Finding, error) {
			return func(path string) ([]finding.Finding, error) {
				return []finding.Finding{{ID: id, Path: path}}, nil
			}
		},
	}
}

func TestLookupResolvesAFamilyMemberToARuleCarryingTheFamilysStatement(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()}, rule(templateDrift))
	if err != nil {
		t.Fatal(err)
	}

	got, known := reg.Lookup("actionsqaci.shellcheck.SC2086")
	if !known {
		t.Fatal("a member of a registered family must be found")
	}
	if got.ID != "actionsqaci.shellcheck.SC2086" || got.Summary != codesFamily().Summary || got.Upstream != "shellcheck v0.11.0" {
		t.Errorf("the synthesised rule is wrong: %+v", got)
	}
	if got.Check == nil {
		t.Fatal("a family member must be runnable")
	}
	found, err := got.Check("a.yml")
	if err != nil || len(found) != 1 || found[0].ID != "actionsqaci.shellcheck.SC2086" {
		t.Errorf("the check was not handed the member's own identifier: %+v, %v", found, err)
	}
}

func TestLookupDoesNotResolveWhatIsNotAMember(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()})
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"actionsqaci.shellcheck.SC208", "actionsqaci.shellcheck.sc2086", "actionsqaci.shellcheck", "actionsqaci.zizmor.template-injection", ""} {
		if got, known := reg.Lookup(id); known {
			t.Errorf("%q must not resolve, got %+v", id, got)
		}
	}
}

func TestALooseMemberPatternCannotMakeAnUnclassifiableIdentifierResolve(t *testing.T) {
	loose := codesFamily()
	loose.Member = func(string) bool { return true }
	reg, err := registry.NewWithFamilies([]registry.Family{loose})
	if err != nil {
		t.Fatal(err)
	}

	if got, known := reg.Lookup("../etc/passwd"); known {
		t.Errorf("an identifier outside the scheme resolved: %+v", got)
	}
}

func TestFamiliesAreListedAndTheListIsACopy(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()})
	if err != nil {
		t.Fatal(err)
	}

	listed := reg.Families()
	if len(listed) != 1 || listed[0].Name != "actionsqaci.shellcheck.SC<nnnn>" {
		t.Fatalf("Families = %+v", listed)
	}
	listed[0].Name = "changed"
	if reg.Families()[0].Name != "actionsqaci.shellcheck.SC<nnnn>" {
		t.Error("changing the returned list changed the registry")
	}
}

func TestAFamilyIsRefusedWhenItCannotBeListedOrRun(t *testing.T) {
	cases := map[string]func(*registry.Family){
		"no name":        func(f *registry.Family) { f.Name = "" },
		"no page":        func(f *registry.Family) { f.Page = "" },
		"page is a path": func(f *registry.Family) { f.Page = "../x" },
		"no summary":     func(f *registry.Family) { f.Summary = " " },
		"summary break":  func(f *registry.Family) { f.Summary = "a\nb" },
		"no member":      func(f *registry.Family) { f.Member = nil },
		"no check":       func(f *registry.Family) { f.Check = nil },
	}

	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			family := codesFamily()
			damage(&family)

			if reg, err := registry.NewWithFamilies([]registry.Family{family}); err == nil {
				t.Errorf("a family with %s was accepted: %+v", name, reg.Families())
			}
		})
	}
}

func TestTwoFamiliesMayNotShareANameOrAPage(t *testing.T) {
	second := codesFamily()
	second.Page = "actionsqaci.other"
	if reg, err := registry.NewWithFamilies([]registry.Family{codesFamily(), second}); err == nil {
		t.Errorf("a repeated name was accepted: %+v", reg.Families())
	}

	third := codesFamily()
	third.Name = "actionsqaci.shellcheck.SC<other>"
	if reg, err := registry.NewWithFamilies([]registry.Family{codesFamily(), third}); err == nil {
		t.Errorf("a repeated page was accepted: %+v", reg.Families())
	}
}

func TestAShellcheckCodeCannotBeRegisteredAsAnIndividualRule(t *testing.T) {
	reg, err := registry.New(rule("actionsqaci.shellcheck.SC2086"))
	if err == nil {
		t.Fatalf("an individual shellcheck rule was accepted: %+v", reg.All())
	}
	if !strings.Contains(err.Error(), "family") {
		t.Errorf("the error must point at the family, got %q", err.Error())
	}
}

func TestEveryShellcheckCodeSharesTheFamilyPage(t *testing.T) {
	for _, id := range []string{"actionsqaci.shellcheck.SC2086", "actionsqaci.shellcheck.SC1000"} {
		if got := registry.DocPath(id); got != "docs/rules/actionsqaci.shellcheck.md" {
			t.Errorf("DocPath(%s) = %q", id, got)
		}
	}
	if got := registry.DocPath("actionsqaci.actionlint.glob"); got != "docs/rules/actionsqaci.actionlint.glob.md" {
		t.Errorf("another identifier's page moved: %q", got)
	}
}

func TestNearestStillSuggestsAnIndividualRuleWhenAFamilyIsRegisteredToo(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()}, rule(templateDrift))
	if err != nil {
		t.Fatal(err)
	}

	got := reg.Nearest("actionsqaci.templateDrif", 3)
	if !slices.Equal(got, []string{templateDrift}) {
		t.Errorf("Nearest = %v", got)
	}
}

func TestAFamilyIsFoundByTheNameItIsListedUnder(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()})
	if err != nil {
		t.Fatal(err)
	}

	family, known := reg.FamilyNamed("actionsqaci.shellcheck.SC<nnnn>")
	if !known || family.Page != "actionsqaci.shellcheck" {
		t.Errorf("family = %+v, known = %v", family, known)
	}
	if other, known := reg.FamilyNamed("actionsqaci.shellcheck.SC2086"); known {
		t.Errorf("a member is not a family: %+v", other)
	}
}

func TestNearestSuggestsAFamilyBySomethingCloseToItsListedName(t *testing.T) {
	reg, err := registry.NewWithFamilies([]registry.Family{codesFamily()}, rule(templateDrift))
	if err != nil {
		t.Fatal(err)
	}

	got := reg.Nearest("actionsqaci.shellcheck.SC<nnn>", 3)
	if !slices.Equal(got, []string{"actionsqaci.shellcheck.SC<nnnn>"}) {
		t.Errorf("Nearest = %v", got)
	}
}
