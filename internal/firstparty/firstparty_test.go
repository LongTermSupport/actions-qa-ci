package firstparty_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

func everythingIsKnown(string) bool { return true }

func inRepository(t *testing.T, record string) string {
	t.Helper()

	root := t.TempDir()
	full := filepath.Join(root, ".github", "actions-qa-ci.yml")
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	return ".github/actions-qa-ci.yml"
}

const unjustified = `version: 1
exceptions:
  - id: actionsqaci.actionlint.glob
    path: .github/workflows/release.yml
    hazard: legacy
    scope: A branch filter in the release workflow that is generated and checked by hand
    decision: later
`

const justifiedRecord = `version: 1
exceptions:
  - id: actionsqaci.actionlint.glob
    path: .github/workflows/release.yml
    hazard: A branch filter with a character class the glob matcher reads differently from GitHub's
    scope: Only the release job, whose filter is generated and checked against GitHub's behaviour by hand
    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123
`

func ruleFor(t *testing.T, id string) registry.Rule {
	t.Helper()

	for _, rule := range firstparty.Rules(everythingIsKnown) {
		if rule.ID == id {
			return rule
		}
	}
	t.Fatalf("no first-party rule %s", id)

	return registry.Rule{}
}

func TestUnjustifiedExceptionIsAFirstPartyRuleThatStatesItsInstruction(t *testing.T) {
	rule := ruleFor(t, firstparty.UnjustifiedException)

	if rule.ID != "actionsqaci.unjustifiedException" || rule.Summary == "" || rule.Check == nil {
		t.Errorf("rule = %+v", rule)
	}
	if kind, err := finding.ClassifyIdentifier(rule.ID); err != nil || kind != finding.KindFirstParty {
		t.Errorf("identifier kind = %v, %v", kind, err)
	}
}

func TestAnExceptionWithoutAJustificationIsAFindingPerFaultAtItsLine(t *testing.T) {
	path := inRepository(t, unjustified)

	found, err := ruleFor(t, firstparty.UnjustifiedException).Check(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 2 {
		t.Fatalf("found %+v, want one for the hazard and one for the decision", found)
	}
	for _, f := range found {
		if f.ID != firstparty.UnjustifiedException || f.Path != path || f.Line != 3 || f.Col != 1 || f.Severity != finding.SeverityError {
			t.Errorf("finding = %+v", f)
		}
		if f.DocPath != registry.DocPath(f.ID) || f.Tool != firstparty.Tool || f.Validate() != nil {
			t.Errorf("finding = %+v", f)
		}
	}
	var hazard, decision bool
	for _, f := range found {
		hazard = hazard || strings.HasPrefix(f.Message, "hazard of exception 1 (actionsqaci.actionlint.glob) is ")
		decision = decision || strings.HasPrefix(f.Message, "decision of exception 1 (actionsqaci.actionlint.glob) ")
	}
	if !hazard || !decision {
		t.Errorf("the messages must name the field, the exception and its identifier: %+v", found)
	}
}

func TestAJustifiedRecordHasNoFinding(t *testing.T) {
	path := inRepository(t, justifiedRecord)

	found, err := ruleFor(t, firstparty.UnjustifiedException).Check(path)
	if err != nil || len(found) != 0 {
		t.Errorf("found %+v, err %v", found, err)
	}
}

func TestAnInvalidRecordIsAnErrorNotAFinding(t *testing.T) {
	path := inRepository(t, "version: 1\nsurprise: true\n")

	found, err := ruleFor(t, firstparty.UnjustifiedException).Check(path)
	if err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Errorf("found %+v, err %v; a record that cannot be read has no verdict", found, err)
	}
}

func TestADefenceDisabledEverywhereIsHeldToTheSameStandard(t *testing.T) {
	path := inRepository(t, "version: 1\nexceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: legacy\n    scope: legacy\n    decision: x\n")

	found, err := ruleFor(t, firstparty.UnjustifiedException).Check(path)
	if err != nil || len(found) != 3 {
		t.Errorf("found %+v, err %v", found, err)
	}
}
