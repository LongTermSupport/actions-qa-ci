package record

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The thresholds follow the convention of php-qa-ci's phpstanIgnoreJustification check: a
// reason has to be long enough to name a hazard and a scope.
const (
	minLength = 40
	minWords  = 6
)

// generic is the documented list of reasons that fit any entry unchanged.
var generic = regexp.MustCompile(`(?i)^(needed for now|for now|legacy( code)?|todo|fixme|wip|temporary|temp|will fix later|fix later|too noisy|noisy|known issue|ignore|false positive)\W*$`)

// pointsElsewhere matches a reason that refers to another entry instead of stating itself.
var pointsElsewhere = regexp.MustCompile(`(?i)\b(see (above|below)|as (above|before)|same as (above|before)|ditto)\b`)

// Fault is one way an exception's justification is not one.
type Fault struct {
	// Field is hazard, scope or decision.
	Field string
	// Reason says what is wrong with it.
	Reason string
}

// Faults lists what is wrong with the exception's justification, at most one fault for each of
// hazard, scope and decision. The check cannot tell whether a sentence is true; it refuses the
// ones that could be written without knowing anything.
func (e Exception) Faults() []Fault {
	var faults []Fault

	hazard := normalise(e.Hazard)
	if reason := reasonFault(hazard, e.ID); reason != "" {
		faults = append(faults, Fault{"hazard", reason})
	}

	scope := normalise(e.Scope)
	reason := reasonFault(scope, e.ID)
	if reason == "" && strings.EqualFold(scope, hazard) {
		reason = "is the same as the hazard; say where the exception applies and why it is accepted only there"
	}
	if reason != "" {
		faults = append(faults, Fault{"scope", reason})
	}

	if reason := decisionFault(strings.TrimSpace(e.Decision)); reason != "" {
		faults = append(faults, Fault{"decision", reason})
	}

	return faults
}

// normalise collapses every run of white space to one space.
func normalise(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func reasonFault(text, id string) string {
	switch {
	case text == "":
		return "is missing"
	case generic.MatchString(text):
		return fmt.Sprintf("is %q, a reason that fits any entry; say what the defence would report and why that is accepted", text)
	case withoutIdentifier(text, id) == "":
		return "only repeats the identifier"
	case pointsElsewhere.MatchString(text):
		return "refers to another entry instead of stating the hazard or scope itself"
	case utf8.RuneCountInString(text) < minLength:
		return fmt.Sprintf("is shorter than %d characters", minLength)
	case len(strings.Fields(text)) < minWords:
		return fmt.Sprintf("has fewer than %d words", minWords)
	}

	return ""
}

// withoutIdentifier is the text with the identifier, and the words made of its segments, taken
// out; what is left says nothing about the exception.
func withoutIdentifier(text, id string) string {
	rest := text
	if id != "" {
		rest = strings.ReplaceAll(strings.ToLower(rest), strings.ToLower(id), "")
	}

	return strings.TrimFunc(rest, func(r rune) bool { return r == ' ' || r == '.' || r == ',' || r == ':' || r == ';' })
}

func decisionFault(decision string) string {
	if decision == "" {
		return "is missing"
	}
	if reference, isOwner := strings.CutPrefix(decision, "Owner:"); isOwner {
		if strings.TrimSpace(reference) == "" {
			return "names no decision after Owner:"
		}

		return ""
	}

	address, err := url.Parse(decision)
	if err != nil || address.Scheme != "https" || address.Host == "" {
		return "must be an https URL to where the decision was made, or Owner: and what was decided"
	}

	return ""
}
