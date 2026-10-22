package catalogue_test

import (
	"os"
	"path/filepath"
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

func TestTheShippedRegistryResolvesEveryShellcheckCodeThroughTheFamily(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	rule, known := reg.Lookup("actionsqaci.shellcheck.SC2115")
	if !known || rule.Check == nil || rule.Summary == "" {
		t.Errorf("SC2115 does not resolve to a runnable rule: %+v, %v", rule, known)
	}
	if len(reg.Families()) != 1 {
		t.Errorf("families = %+v, want the ShellCheck family alone", reg.Families())
	}
}

func TestTheJustificationCheckIsShippedAndKnowsEveryShippedIdentifier(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	rule, known := reg.Lookup("actionsqaci.unjustifiedException")
	if !known || rule.Check == nil {
		t.Fatalf("rule = %+v, known = %v", rule, known)
	}

	root := t.TempDir()
	record := "version: 1\nexceptions:\n" +
		"  - id: actionsqaci.shellcheck.SC2115\n    hazard: legacy\n    scope: legacy\n    decision: x\n" +
		"  - id: actionsqaci.actionlint.glob\n    hazard: legacy\n    scope: legacy\n    decision: x\n" +
		"  - id: actionsqaci.unjustifiedException\n    hazard: legacy\n    scope: legacy\n    decision: x\n"
	if err := os.WriteFile(filepath.Join(root, "record.yml"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	found, err := rule.Check("record.yml")
	if err != nil {
		t.Fatalf("an exception naming a shipped identifier was refused as unknown: %v", err)
	}
	if len(found) != 9 {
		t.Errorf("found %d findings, want 3 for each of the 3 exceptions: %+v", len(found), found)
	}

	if err := os.WriteFile(filepath.Join(root, "record.yml"), []byte("version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if found, err := rule.Check("record.yml"); err == nil {
		t.Errorf("an exception naming no shipped identifier was accepted: %+v", found)
	}
}
