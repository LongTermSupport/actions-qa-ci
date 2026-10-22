package record

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
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

// pointsElsewhere matches a reason that refers to another entry instead of stating itself: one
// of the phrases opening a sentence or closing the text. The same words in the middle of a
// sentence ("reads it as before GitHub changed it") say something.
var pointsElsewhere = regexp.MustCompile(`(?i)((^|[.;:!?]\s*)(see (above|below)|as (above|before)|(the )?same as (above|before)|ditto)\b|\b(see (above|below)|as (above|before)|same as (above|before)|ditto)\W*$)`)

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
	if reason == "" && words(scope) == words(hazard) {
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
	case onlyIdentifier(text, id):
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

// words is the text reduced to its lower-case letters and digits, one space between runs, so
// that punctuation and case cannot make one sentence another.
func words(text string) string {
	return strings.Join(wordsOf(text), " ")
}

func wordsOf(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// onlyIdentifier says whether every word of the text is a word of the identifier, which says
// nothing about the exception.
func onlyIdentifier(text, id string) bool {
	known := map[string]bool{}
	for _, word := range wordsOf(id) {
		known[word] = true
	}
	for _, word := range wordsOf(text) {
		if !known[word] {
			return false
		}
	}

	return true
}

func decisionFault(decision string) string {
	if decision == "" {
		return "is missing"
	}
	if reference, isOwner := strings.CutPrefix(decision, "Owner:"); isOwner {
		if !strings.ContainsFunc(reference, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			return "names no decision after Owner:"
		}

		return ""
	}

	const want = "must be an https URL to where the decision was made, or Owner: and what was decided"
	if strings.ContainsFunc(decision, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || !unicode.IsPrint(r) }) {
		return want + "; a URL holds no space or control character"
	}
	address, err := url.Parse(decision)
	if err != nil || address.Scheme != "https" || address.User != nil || !plausibleHost(address.Hostname()) {
		return want
	}

	return ""
}

// plausibleHost accepts a dotted name made of letters, digits and hyphens, or an IP address.
func plausibleHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return false
			}
		}
	}

	return true
}
