package actionlint

import (
	"regexp"

	upstream "github.com/rhysd/actionlint"
)

// Duplicate is a report of actionlint's that another rule owns, which the lane therefore drops: a
// hazard reported twice would carry two identifiers and need two exceptions.
type Duplicate struct {
	// Report says which of actionlint's reports is dropped.
	Report string
	// Kind is the kind of actionlint finding it is.
	Kind string
	// WholeKind is true when every finding of the kind is dropped, and the kind has no rule.
	WholeKind bool
	// Owner is the identifier of the rule that reports the hazard instead.
	Owner string

	// messages is what a finding of the kind says when it is this duplicate, for a kind that also
	// carries findings that stay (a type error is actionlint's alone).
	messages []*regexp.Regexp
}

var duplicates = []Duplicate{
	{
		Report:    "actionlint's credentials kind",
		Kind:      "credentials",
		WholeKind: true,
		Owner:     "actionsqaci.zizmor.hardcoded-container-credentials",
	},
	{
		Report: "actionlint's expression findings that call an input potentially untrusted",
		Kind:   "expression",
		Owner:  "actionsqaci.zizmor.template-injection",
		messages: []*regexp.Regexp{
			regexp.MustCompile(`(?s)^".*" is potentially untrusted\. avoid using it directly in inline scripts`),
			regexp.MustCompile(`(?s)^object filter extracts potentially untrusted properties `),
		},
	},
}

// Duplicates returns every report of actionlint's the lane drops, with the rule that owns it.
func Duplicates() []Duplicate {
	return append([]Duplicate(nil), duplicates...)
}

// dropped says whether actionlint's error is a duplicate of a hazard another rule owns.
func dropped(e *upstream.Error) bool {
	for _, d := range duplicates {
		if d.Kind != e.Kind {
			continue
		}
		if d.WholeKind {
			return true
		}
		for _, message := range d.messages {
			if message.MatchString(e.Message) {
				return true
			}
		}
	}

	return false
}
