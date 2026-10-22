package record_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func justified() record.Exception {
	return record.Exception{
		ID:       "actionsqaci.actionlint.glob",
		Path:     ".github/workflows/release.yml",
		Hazard:   "A branch filter with a character class the glob matcher reads differently from GitHub's",
		Scope:    "Only the release job, whose filter is generated and checked against GitHub's behaviour by hand",
		Decision: "https://github.com/LongTermSupport/php-qa-ci/issues/123",
	}
}

func TestAnExceptionWithARealJustificationHasNoFault(t *testing.T) {
	if faults := justified().Faults(); len(faults) != 0 {
		t.Errorf("faults = %+v", faults)
	}
}

func TestEachJustificationFieldIsRequired(t *testing.T) {
	for _, field := range []string{"hazard", "scope", "decision"} {
		e := justified()
		switch field {
		case "hazard":
			e.Hazard = ""
		case "scope":
			e.Scope = " \n\t "
		case "decision":
			e.Decision = ""
		}

		faults := e.Faults()
		if len(faults) != 1 || faults[0].Field != field || !strings.Contains(faults[0].Reason, "missing") {
			t.Errorf("%s: faults = %+v", field, faults)
		}
	}
}

func TestAGenericReasonIsNotAJustification(t *testing.T) {
	generic := []string{
		"needed for now", "for now", "legacy", "Legacy code", "TODO", "fixme", "WIP", "temporary", "temp",
		"will fix later", "fix later", "too noisy", "noisy", "known issue", "ignore", "false positive",
		"False positive.", "legacy!",
	}

	for _, text := range generic {
		e := justified()
		e.Hazard = text

		faults := e.Faults()
		if len(faults) == 0 || faults[0].Field != "hazard" {
			t.Errorf("%q was accepted as a hazard: %+v", text, faults)
		}
	}
}

func TestAJustificationHasToBeLongEnoughAndSaySomething(t *testing.T) {
	short := "A glob the matcher misreads"                              // under 40 characters
	fewWords := "Supercalifragilisticexpialidocious-glob-thing-here-xx" // long, one word
	cases := map[string]string{
		"under 40 characters":     short,
		"too few words":           fewWords,
		"see above":               "see above, the same as the one before it in this file please",
		"as above":                "as above for every other entry in this record of exceptions",
		"the identifier repeated": "actionsqaci.actionlint.glob actionsqaci.actionlint.glob actionsqaci.actionlint.glob",
	}

	for name, text := range cases {
		e := justified()
		e.Scope = text

		faults := e.Faults()
		if len(faults) != 1 || faults[0].Field != "scope" {
			t.Errorf("%s: faults = %+v", name, faults)
		}
	}
}

func TestHazardAndScopeMayNotBeTheSameSentence(t *testing.T) {
	e := justified()
	e.Scope = e.Hazard

	faults := e.Faults()
	if len(faults) != 1 || faults[0].Field != "scope" || !strings.Contains(faults[0].Reason, "same") {
		t.Errorf("faults = %+v", faults)
	}
}

func TestWhitespaceDoesNotMakeAShortJustificationLongEnough(t *testing.T) {
	e := justified()
	e.Hazard = "a     b     c     d     e     f     g     h     i     j"

	if faults := e.Faults(); len(faults) != 1 || faults[0].Field != "hazard" {
		t.Errorf("faults = %+v", faults)
	}
}

func TestADecisionIsAnHTTPSURLOrAnOwnerReference(t *testing.T) {
	accepted := []string{
		"https://github.com/LongTermSupport/php-qa-ci/issues/310",
		"https://example.org/decision",
		"Owner: the 2026 branch-protection ruling",
	}
	refused := []string{
		"http://example.org/decision", "https://", "https:///x", "example.org/decision", "owner: someone",
		"Owner:", "Owner:   ", "later", "the Owner said so", "ftp://example.org/x",
	}

	for _, decision := range accepted {
		e := justified()
		e.Decision = decision
		if faults := e.Faults(); len(faults) != 0 {
			t.Errorf("%q was refused: %+v", decision, faults)
		}
	}
	for _, decision := range refused {
		e := justified()
		e.Decision = decision
		if faults := e.Faults(); len(faults) != 1 || faults[0].Field != "decision" {
			t.Errorf("%q was accepted: %+v", decision, faults)
		}
	}
}

func TestEveryFaultOfOneExceptionIsReported(t *testing.T) {
	faults := record.Exception{ID: "actionsqaci.actionlint.glob"}.Faults()

	if len(faults) != 3 {
		t.Errorf("faults = %+v, want one for each of hazard, scope and decision", faults)
	}
}
