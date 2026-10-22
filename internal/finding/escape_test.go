package finding_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestEscapeShowsEverythingAReaderCouldNotSeeOrATerminalWouldActOn(t *testing.T) {
	cases := map[string]string{
		"rlo" + string(rune(0x202e)) + "x":   "rlo" + "\\" + "u202ex",
		"zero" + string(rune(0x200b)) + "w":  "zero" + "\\" + "u200bw",
		"bad\xffbyte":                        `bad\xffbyte`,
		"nul\x00":                            `nul\x00`,
		"del\x7f":                            `del\x7f`,
		"next" + string(rune(0x85)) + "line": `next\u0085line`,
		"accents: é ü 日本語":                   "accents: é ü 日本語",
		"quote \" and backslash \\":          "quote \" and backslash \\",
	}

	for in, want := range cases {
		if got := finding.Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}
