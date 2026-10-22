package finding

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Severity is how serious a finding is.
type Severity string

// The severities a finding can carry.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNote    Severity = "note"
)

// Finding is one thing a lane found. The JSON form is the schema the json output format
// prints, so the field tags are part of the public contract.
type Finding struct {
	ID           string   `json:"id"`
	Tool         string   `json:"tool"`
	UpstreamID   string   `json:"upstreamId"`
	Path         string   `json:"path"`
	Line         int      `json:"line"`
	Col          int      `json:"col"`
	EndLine      int      `json:"endLine"`
	Severity     Severity `json:"severity"`
	Message      string   `json:"message"`
	DocPath      string   `json:"docPath"`
	Excepted     bool     `json:"excepted"`
	ExceptionKey string   `json:"exceptionKey"`
}

// Validate says why a finding could not be acted on, or returns nil. A lane that builds an
// invalid finding has a defect of its own, which the caller reports as a toolchain fault
// rather than printing something a reader cannot use.
func (f Finding) Validate() error {
	if _, err := ClassifyIdentifier(f.ID); err != nil {
		return err
	}

	switch {
	case blank(f.Tool):
		return errors.New("finding names no tool")
	case blank(f.Path):
		return fmt.Errorf("finding %s names no path", f.ID)
	case strings.ContainsFunc(f.Path, IsControlOrSeparator):
		return fmt.Errorf("finding %s names %q, which has a control character; a path must print on one line", f.ID, f.Path)
	case !repositoryRelative(f.Path):
		return fmt.Errorf("finding %s names %q; a path must be repository-relative and stay inside the repository", f.ID, f.Path)
	case blank(f.Message):
		return fmt.Errorf("finding %s has no message", f.ID)
	case f.Line < 1:
		return fmt.Errorf("finding %s at %s has line %d; lines start at 1", f.ID, f.Path, f.Line)
	case f.Col < 0:
		return fmt.Errorf("finding %s at %s has column %d; a column is not negative", f.ID, f.Path, f.Col)
	case f.EndLine != 0 && f.EndLine < f.Line:
		return fmt.Errorf("finding %s at %s ends on line %d before it starts on line %d", f.ID, f.Path, f.EndLine, f.Line)
	case !f.Severity.valid():
		return fmt.Errorf("finding %s has unknown severity %q", f.ID, f.Severity)
	case f.Excepted && f.ExceptionKey == "":
		return fmt.Errorf("finding %s is excepted but names no exception", f.ID)
	case !f.Excepted && f.ExceptionKey != "":
		return fmt.Errorf("finding %s names exception %q but is not excepted", f.ID, f.ExceptionKey)
	}

	return nil
}

// IsControlOrSeparator reports a character that can start another line or move a cursor
// when printed: a control character (including NUL, tab, escape and the C1 set) or a Unicode
// line or paragraph separator.
func IsControlOrSeparator(r rune) bool {
	return unicode.IsControl(r) || r == ' ' || r == ' '
}

// Escape makes every control character and line or paragraph separator in text visible as a Go
// escape (\n, \x1b,  ) and changes nothing else, so a name that came from a repository can
// be put in a diagnostic without starting a line or reaching the terminal as a command.
func Escape(text string) string {
	var escaped strings.Builder
	for _, r := range text {
		if !IsControlOrSeparator(r) {
			escaped.WriteRune(r)

			continue
		}
		quoted := strconv.QuoteToASCII(string(r))
		escaped.WriteString(quoted[1 : len(quoted)-1])
	}

	return escaped.String()
}

// DisplayPath prints a path as it is when that is unambiguous, and as a double-quoted Go
// string when it contains a space or anything that needs escaping (a quote, a backslash,
// invalid UTF-8, a non-printable character). A trailer line always holds a space, and a
// quoted path starts with a quote, so a file name cannot pass for a trailer.
func DisplayPath(path string) string {
	quoted := strconv.Quote(path)
	if strings.Contains(path, " ") || quoted != `"`+path+`"` {
		return quoted
	}

	return path
}

func blank(text string) bool {
	return strings.TrimSpace(text) == ""
}

func repositoryRelative(path string) bool {
	if strings.HasPrefix(path, "/") {
		return false
	}

	return !slices.Contains(strings.Split(path, "/"), "..")
}

func (s Severity) valid() bool {
	return s == SeverityError || s == SeverityWarning || s == SeverityNote
}

// Sort orders findings by path, then position, then identifier, then the rest of their
// content, so output is the same on every run whatever order the lanes reported in.
func Sort(findings []Finding) {
	slices.SortStableFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.ID, b.ID),
			cmp.Compare(a.EndLine, b.EndLine),
			cmp.Compare(a.Message, b.Message),
			cmp.Compare(a.Tool, b.Tool),
			cmp.Compare(a.UpstreamID, b.UpstreamID),
			cmp.Compare(a.Severity, b.Severity),
			cmp.Compare(a.DocPath, b.DocPath),
			cmp.Compare(a.ExceptionKey, b.ExceptionKey),
			compareBool(a.Excepted, b.Excepted),
		)
	})
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}
