package pages_test

import (
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const (
	familyName = "actionsqaci.shellcheck.SC<nnnn>"
	familyPage = "actionsqaci.shellcheck"
)

var familyMember = regexp.MustCompile(`^actionsqaci\.shellcheck\.SC[0-9]{4}$`)

func familyRegistry(t *testing.T, ids ...string) *registry.Registry {
	t.Helper()

	rules := make([]registry.Rule, 0, len(ids))
	for _, id := range ids {
		rules = append(rules, registry.Rule{ID: id, Summary: "stands for " + id})
	}

	reg, err := registry.NewWithFamilies([]registry.Family{{
		Name:    familyName,
		Page:    familyPage,
		Summary: "Shell in run: steps is free of ShellCheck warnings",
		Member:  familyMember.MatchString,
		Check: func(string) func(string) ([]finding.Finding, error) {
			return func(string) ([]finding.Finding, error) { return nil, nil }
		},
	}}, rules...)
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

func TestTheIndexListsAFamilyOnceWithTheSharedPage(t *testing.T) {
	index := pages.Index(familyRegistry(t, lintCoverage, templateDrift))

	want := "| `" + familyName + "` | Shell in run: steps is free of ShellCheck warnings | [page](" + familyPage + ".md) |\n"
	if !strings.Contains(index, want) {
		t.Errorf("the index lacks the family row:\n%s", index)
	}
	if strings.Count(index, "ShellCheck") != 1 {
		t.Errorf("the family must be listed once, not once per code:\n%s", index)
	}
	if !(strings.Index(index, lintCoverage) < strings.Index(index, familyName) && strings.Index(index, familyName) < strings.Index(index, templateDrift)) {
		t.Errorf("rows are not in identifier order:\n%s", index)
	}
}

func TestTheIndexOfARegistryWithOnlyAFamilyIsNotSaidToBeEmpty(t *testing.T) {
	index := pages.Index(familyRegistry(t))

	if strings.Contains(index, "No rules are registered yet") || !strings.Contains(index, familyName) {
		t.Errorf("a family is a rule that is registered:\n%s", index)
	}
}

func familyPages(withFamilyPage bool, pageBody string) fstest.MapFS {
	fsys := fstest.MapFS{"docs/rules/README.md": {Data: []byte("# index\n")}}
	if withFamilyPage {
		fsys["docs/rules/"+familyPage+".md"] = &fstest.MapFile{Data: []byte(pageBody)}
	}

	return fsys
}

func TestAuditOfACompleteFamilyPageFindsNothing(t *testing.T) {
	problems := pages.Audit(familyRegistry(t), familyPages(true, completePage))

	if len(problems) != 0 {
		t.Errorf("problems = %v", problems)
	}
}

func TestAuditReportsAFamilyWithNoPage(t *testing.T) {
	problems := pages.Audit(familyRegistry(t), familyPages(false, ""))

	if len(problems) != 1 || !strings.Contains(problems[0], familyPage+".md") || !strings.Contains(problems[0], "no page") {
		t.Errorf("problems = %v", problems)
	}
}

func TestAuditHoldsAFamilyPageToTheRequiredSections(t *testing.T) {
	page := strings.Replace(completePage, "## Hazard", "## Danger", 1)

	problems := pages.Audit(familyRegistry(t), familyPages(true, page))

	if len(problems) != 1 || !strings.Contains(problems[0], `"## Hazard"`) {
		t.Errorf("problems = %v", problems)
	}
}

func TestAFamilyPageIsNotAPageForNoRegisteredRule(t *testing.T) {
	fsys := familyPages(true, completePage)
	fsys["docs/rules/actionsqaci.other.md"] = &fstest.MapFile{Data: []byte(completePage)}

	problems := pages.Audit(familyRegistry(t), fsys)

	if len(problems) != 1 || !strings.Contains(problems[0], "actionsqaci.other.md") || !strings.Contains(problems[0], "no registered rule") {
		t.Errorf("only the stray page may be reported, got %v", problems)
	}
}

func TestPageForAFamilyMemberIsTheSharedPage(t *testing.T) {
	got, err := pages.Page(familyPages(true, completePage), "actionsqaci.shellcheck.SC2086")
	if err != nil {
		t.Fatal(err)
	}

	if got != completePage {
		t.Errorf("the member's page is not the family page: %q", got)
	}
}
