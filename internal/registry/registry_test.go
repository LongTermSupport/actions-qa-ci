package registry_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const (
	templateDrift = "actionsqaci.templateDrift"
	lintCoverage  = "actionsqaci.lintCoverage"
	templateInj   = "actionsqaci.zizmor.template-injection"
)

func rule(id string) registry.Rule {
	return registry.Rule{ID: id, Summary: "stands for " + id}
}

func TestARegistryListsItsRulesInIdentifierOrder(t *testing.T) {
	reg, err := registry.New(rule(templateInj), rule(templateDrift), rule(lintCoverage))
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, r := range reg.All() {
		got = append(got, r.ID)
	}
	want := []string{lintCoverage, templateDrift, templateInj}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestARegistryRefusesAnIdentifierTwice(t *testing.T) {
	if _, err := registry.New(rule(templateDrift), rule(templateDrift)); err == nil {
		t.Fatal("a duplicate identifier was accepted; two rules would answer for one name")
	}
}

func TestARegistryRefusesARuleThatCannotBeReadAsAStandingInstruction(t *testing.T) {
	cases := map[string]registry.Rule{
		"malformed identifier": rule("template-drift"),
		"no summary":           {ID: templateDrift},
		"blank summary":        {ID: templateDrift, Summary: "  "},
		"multi-line summary":   {ID: templateDrift, Summary: "one\ntwo"},
	}

	for name, r := range cases {
		if _, err := registry.New(r); err == nil {
			t.Errorf("%s: the rule was accepted", name)
		}
	}
}

func TestLookupFindsARegisteredRuleAndOnlyThat(t *testing.T) {
	reg, err := registry.New(rule(templateDrift))
	if err != nil {
		t.Fatal(err)
	}

	found, ok := reg.Lookup(templateDrift)
	if !ok || found.ID != templateDrift {
		t.Errorf("Lookup(%s) = %+v, %v", templateDrift, found, ok)
	}
	if _, ok := reg.Lookup(lintCoverage); ok {
		t.Errorf("Lookup found %s, which was never registered", lintCoverage)
	}
}

func TestNearestSuggestsTheClosestIdentifiersFirst(t *testing.T) {
	reg, err := registry.New(rule(templateDrift), rule(lintCoverage), rule(templateInj))
	if err != nil {
		t.Fatal(err)
	}

	got := reg.Nearest("actionsqaci.templatedrift", 2)

	if len(got) == 0 || got[0] != templateDrift {
		t.Errorf("Nearest = %v, want %s first", got, templateDrift)
	}
	if len(got) > 2 {
		t.Errorf("Nearest returned %d identifiers, want at most 2", len(got))
	}
}

func TestNearestSuggestsNothingForAnIdentifierNothingResembles(t *testing.T) {
	reg, err := registry.New(rule(templateDrift))
	if err != nil {
		t.Fatal(err)
	}

	if got := reg.Nearest("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", 3); len(got) != 0 {
		t.Errorf("Nearest = %v, want nothing: a wild guess is worse than none", got)
	}
}

func TestEveryRuleHasAPageAtADerivedPath(t *testing.T) {
	if got, want := registry.DocPath(templateDrift), "docs/rules/actionsqaci.templateDrift.md"; got != want {
		t.Errorf("DocPath = %q, want %q", got, want)
	}
}
