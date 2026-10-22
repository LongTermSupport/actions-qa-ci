package pages_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const (
	templateDrift = "actionsqaci.templateDrift"
	lintCoverage  = "actionsqaci.lintCoverage"
)

const completePage = `# actionsqaci.x

## What it forbids or requires

Something.

## Hazard

Something bad.

## Correct construction

` + "```yaml\nrun: echo ok\n```" + `

## Exceptions

How to record one.

## Upstream

None: first-party.
`

func registryOf(t *testing.T, ids ...string) *registry.Registry {
	t.Helper()

	rules := make([]registry.Rule, 0, len(ids))
	for _, id := range ids {
		rules = append(rules, registry.Rule{ID: id, Summary: "stands for " + id})
	}

	reg, err := registry.New(rules...)
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

func pagesFor(ids ...string) fstest.MapFS {
	fsys := fstest.MapFS{"docs/rules/README.md": {Data: []byte("# index\n")}}
	for _, id := range ids {
		fsys["docs/rules/"+id+".md"] = &fstest.MapFile{Data: []byte(completePage)}
	}

	return fsys
}

func TestTheIndexListsEveryRuleInIdentifierOrderWithItsPage(t *testing.T) {
	got := pages.Index(registryOf(t, templateDrift, lintCoverage))

	first := strings.Index(got, "| `"+lintCoverage+"` |")
	second := strings.Index(got, "| `"+templateDrift+"` |")
	if first < 0 || second < 0 || first > second {
		t.Errorf("index rows are missing or out of order:\n%s", got)
	}
	if !strings.Contains(got, "(actionsqaci.templateDrift.md)") {
		t.Errorf("a row should link its page relative to the index:\n%s", got)
	}
	if !strings.Contains(got, "stands for "+lintCoverage) {
		t.Errorf("a row should carry the rule's summary:\n%s", got)
	}
}

func TestTheIndexSaysSoWhenNoRuleIsRegistered(t *testing.T) {
	got := pages.Index(registryOf(t))

	if !strings.Contains(got, "No rules are registered") {
		t.Errorf("an empty index should say so:\n%s", got)
	}
	if strings.Contains(got, "| Identifier |") {
		t.Errorf("an empty index should have no table:\n%s", got)
	}
}

func TestTheIndexIsMarkedGeneratedAndEndsWithANewline(t *testing.T) {
	got := pages.Index(registryOf(t, templateDrift))

	if !strings.Contains(got, "do not edit") {
		t.Errorf("the index should say it is generated:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("the index should end in exactly one newline: %q", got[max(0, len(got)-10):])
	}
}

func TestPageReturnsTheEmbeddedPageForARegisteredRule(t *testing.T) {
	got, err := pages.Page(pagesFor(templateDrift), templateDrift)
	if err != nil {
		t.Fatal(err)
	}
	if got != completePage {
		t.Errorf("page = %q", got)
	}
}

func TestPageSaysWhenARulesPageIsMissing(t *testing.T) {
	_, err := pages.Page(pagesFor(), templateDrift)

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one that wraps fs.ErrNotExist", err)
	}
	if err != nil && !strings.Contains(err.Error(), templateDrift) {
		t.Errorf("the error should name the identifier: %v", err)
	}
}

func TestPageRefusesAnIdentifierThatIsAPathNotAName(t *testing.T) {
	for _, id := range []string{"../README", "actionsqaci/x", "", "a b", "README"} {
		if _, err := pages.Page(pagesFor("README"), id); err == nil {
			t.Errorf("%q was treated as a page name", id)
		}
	}
}

func TestAuditOfACompleteSetOfPagesFindsNothing(t *testing.T) {
	problems := pages.Audit(registryOf(t, templateDrift, lintCoverage), pagesFor(templateDrift, lintCoverage))

	if len(problems) != 0 {
		t.Errorf("problems = %v", problems)
	}
}

func TestAuditReportsARegisteredRuleWithNoPage(t *testing.T) {
	problems := pages.Audit(registryOf(t, templateDrift, lintCoverage), pagesFor(templateDrift))

	if len(problems) != 1 || !strings.Contains(problems[0], lintCoverage) || !strings.Contains(problems[0], "no page") {
		t.Errorf("problems = %v", problems)
	}
}

func TestAuditReportsAPageMissingARequiredSection(t *testing.T) {
	for _, section := range pages.RequiredSections {
		fsys := pagesFor(templateDrift)
		fsys["docs/rules/"+templateDrift+".md"] = &fstest.MapFile{
			Data: []byte(strings.Replace(completePage, section, "## Something else", 1)),
		}

		problems := pages.Audit(registryOf(t, templateDrift), fsys)

		if len(problems) != 1 || !strings.Contains(problems[0], section) {
			t.Errorf("without %q: problems = %v", section, problems)
		}
	}
}

func TestAuditReportsAPageForNoRegisteredRule(t *testing.T) {
	problems := pages.Audit(registryOf(t, templateDrift), pagesFor(templateDrift, lintCoverage))

	if len(problems) != 1 || !strings.Contains(problems[0], lintCoverage) || !strings.Contains(problems[0], "no registered rule") {
		t.Errorf("problems = %v", problems)
	}
}

func TestAuditIgnoresTheIndexItself(t *testing.T) {
	if problems := pages.Audit(registryOf(t), pagesFor()); len(problems) != 0 {
		t.Errorf("the index was audited as a page: %v", problems)
	}
}

func TestAuditReportsAnUnreadableDirectoryRatherThanPassing(t *testing.T) {
	problems := pages.Audit(registryOf(t, templateDrift), fstest.MapFS{})

	if len(problems) == 0 {
		t.Error("an empty file system passed the audit")
	}
}
