package finding_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestIsControlOrSeparatorCoversEverythingThatReordersOrHidesText(t *testing.T) {
	for name, r := range map[string]rune{
		"newline": '\n', "escape": 0x1b, "delete": 0x7f, "next line": 0x85,
		"line separator": 0x2028, "paragraph separator": 0x2029,
		"right-to-left override": 0x202e, "left-to-right isolate": 0x2066,
		"zero-width space": 0x200b, "byte order mark": 0xfeff, "soft hyphen": 0xad,
	} {
		if !finding.IsControlOrSeparator(r) {
			t.Errorf("%s (U+%04X) is not reported", name, r)
		}
	}
	for name, r := range map[string]rune{"letter": 'a', "space": ' ', "accented": 'é', "ideograph": 0x65e5, "no-break space": 0xa0} {
		if finding.IsControlOrSeparator(r) {
			t.Errorf("%s (U+%04X) is reported; ordinary text must print as it is", name, r)
		}
	}
}

func TestAFindingWhosePathCouldReorderTheLineItIsPrintedOnIsInvalid(t *testing.T) {
	f := finding.Finding{
		ID: "actionsqaci.actionlint.glob", Tool: "t", Path: ".github/workflows/rlo" + string(rune(0x202e)) + "yml.exe",
		Line: 1, Col: 1, Severity: finding.SeverityError, Message: "m",
	}

	err := f.Validate()

	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Errorf("Validate = %v", err)
	}
}
