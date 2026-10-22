package output_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
)

// hostile holds one of each kind of character that hides or reorders text on a terminal.
func hostile() string {
	return "a" + string(rune(0x202e)) + "b" + string(rune(0x85)) + "c" + string(rune(0x200b)) +
		"d\x7fe" + string(rune(0x2028)) + "f\x1b[2Jg" + string(rune(0xfeff)) + "h"
}

func TestNoFormatPrintsACharacterThatHidesOrReordersText(t *testing.T) {
	f := at(ciPath, 3, 5, templateDrift, hostile())
	f.UpstreamID = hostile()
	f.ExceptionKey, f.Excepted = "", false

	for _, format := range []output.Format{output.Text, output.Agent, output.JSON} {
		got := render(t, format, []finding.Finding{f})

		for _, r := range got {
			if r == '\n' || r == ' ' {
				continue
			}
			if finding.IsControlOrSeparator(r) {
				t.Errorf("%s printed U+%04X: %q", format, r, got)
			}
		}
	}
}

func TestTheJSONFormatKeepsWhatItWasGivenWhileEscapingIt(t *testing.T) {
	f := at(ciPath, 3, 5, templateDrift, hostile())

	got := render(t, output.JSON, []finding.Finding{f})

	var parsed []finding.Finding
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	if len(parsed) != 1 || parsed[0].Message != hostile() {
		t.Errorf("the message did not survive the round trip: %+v", parsed)
	}
	for _, want := range []string{"u202e", "u0085", "u007f", "u200b", "ufeff"} {
		if !strings.Contains(got, strings.Repeat(string(rune(92)), 1)+want) {
			t.Errorf("%s is not shown as an escape:\n%s", want, got)
		}
	}
}
