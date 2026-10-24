package actionlint

import (
	upstream "github.com/rhysd/actionlint"
)

// Duplicate is a report of actionlint's that another rule owns, which the lane therefore drops: a
// hazard reported twice would carry two identifiers and need two exceptions.
//
// A report is dropped only where the owner reports everything the dropped report would. zizmor's
// regular persona does not (it leaves a fromJSON of an untrusted input to its pedantic persona),
// so actionlint's "potentially untrusted" expression messages are not dropped.
type Duplicate struct {
	// Report says which of actionlint's reports is dropped.
	Report string
	// Kind is the kind of actionlint finding it is.
	Kind string
	// WholeKind is true when every finding of the kind is dropped, and the kind has no rule.
	WholeKind bool
	// Owner is the identifier of the rule that reports the hazard instead.
	Owner string
}

var duplicates = []Duplicate{
	{
		Report:    "actionlint's credentials kind",
		Kind:      "credentials",
		WholeKind: true,
		Owner:     "actionsqaci.zizmor.hardcoded-container-credentials",
	},
}

// Duplicates returns every report of actionlint's the lane drops, with the rule that owns it.
func Duplicates() []Duplicate {
	return append([]Duplicate(nil), duplicates...)
}

// dropped says whether actionlint's error is a duplicate of a hazard another rule owns.
func dropped(e *upstream.Error) bool {
	for _, d := range duplicates {
		if d.WholeKind && d.Kind == e.Kind {
			return true
		}
	}

	return false
}
