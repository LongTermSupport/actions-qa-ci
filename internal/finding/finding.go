package finding

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
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
	case f.Tool == "":
		return errors.New("finding names no tool")
	case f.Path == "":
		return fmt.Errorf("finding %s names no path", f.ID)
	case f.Message == "":
		return fmt.Errorf("finding %s has no message", f.ID)
	case f.Line < 1:
		return fmt.Errorf("finding %s at %s has line %d; lines start at 1", f.ID, f.Path, f.Line)
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

func (s Severity) valid() bool {
	return s == SeverityError || s == SeverityWarning || s == SeverityNote
}

// Sort orders findings by path, then position, then identifier, so output is the same on
// every run.
func Sort(findings []Finding) {
	slices.SortStableFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.ID, b.ID),
		)
	})
}
