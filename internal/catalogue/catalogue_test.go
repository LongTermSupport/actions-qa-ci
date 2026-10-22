package catalogue_test

import (
	"os"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
)

func TestTheShippedRulesFormAValidRegistry(t *testing.T) {
	if _, err := catalogue.Registry(); err != nil {
		t.Fatalf("the rules this build ships do not form a registry: %v", err)
	}
}

// The self-audit: every shipped rule has a complete page, and every page is for a shipped
// rule. A rule added without its page, or a page left behind, fails here.
func TestEveryShippedRuleHasACompletePageAndEveryPageHasARule(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	for _, problem := range pages.Audit(reg, os.DirFS("../..")) {
		t.Error(problem)
	}
}
