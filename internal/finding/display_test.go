package finding_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestEscapeMakesControlCharactersAndLineBreaksVisibleAndTouchesNothingElse(t *testing.T) {
	cases := map[string]string{
		"plain text, spaces and ünïcode": "plain text, spaces and ünïcode",
		"a\nb":                           `a\nb`,
		"a\r\nb":                         `a\r\nb`,
		"clear\x1b[2Jscreen":             `clear\x1b[2Jscreen`,
		"bell\x07":                       `bell\a`,
		"tab\there":                      `tab\there`,
		"c1\u009bcontrol":                `c1\u009bcontrol`,
		"":                               "",
	}

	for in, want := range cases {
		if got := finding.Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDisplayPathQuotesWhatCouldPassForAnotherLine(t *testing.T) {
	cases := map[string]string{
		".github/workflows/ci.yml": ".github/workflows/ci.yml",
		"has space.yml":            `"has space.yml"`,
		"new\nline.yml":            `"new\nline.yml"`,
		"esc\x1b.yml":              `"esc\x1b.yml"`,
	}

	for in, want := range cases {
		if got := finding.DisplayPath(in); got != want {
			t.Errorf("DisplayPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeMakesUnicodeLineAndParagraphSeparatorsVisible(t *testing.T) {
	in := "line" + string(rune(0x2028)) + "break" + string(rune(0x2029))

	if got, want := finding.Escape(in), `line\u2028break\u2029`; got != want {
		t.Errorf("Escape = %q, want %q", got, want)
	}
}
