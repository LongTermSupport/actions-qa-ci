// Package firstparty holds the checks this toolchain makes itself, as opposed to the ones it
// takes from a bundled backend: the checks on a project's own record.
package firstparty

import (
	"fmt"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Tool names the checks on every finding they produce.
const Tool = "actions-qa-ci"

// The identifiers of the first-party checks.
const (
	UnjustifiedException = finding.ToolPrefix + ".unjustifiedException"
)

// Rules returns the first-party checks. known says whether an identifier belongs to a defence
// of this build, which the record's exceptions have to name.
func Rules(known func(id string) bool) []registry.Rule {
	return []registry.Rule{{
		ID:      UnjustifiedException,
		Summary: "Every exception in the project record states a hazard, a scope and a decision, none of them generic",
		Check:   CheckRecord(known),
	}}
}

// CheckRecord is the check on the project record at a path: a finding for each justification
// that is missing or generic. A record that is not valid is an error wrapping record.ErrInvalid.
func CheckRecord(known func(id string) bool) func(path string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		rec, err := record.Load(path, known)
		if err != nil {
			return nil, err
		}

		var found []finding.Finding
		for i, e := range rec.Exceptions {
			for _, fault := range e.Faults() {
				f := finding.Finding{
					ID:       UnjustifiedException,
					Tool:     Tool,
					Path:     path,
					Line:     max(e.Line, 1),
					Col:      1,
					Severity: finding.SeverityError,
					Message:  fmt.Sprintf("%s of exception %d (%s) %s", fault.Field, i+1, e.ID, fault.Reason),
					DocPath:  registry.DocPath(UnjustifiedException),
				}
				if err := f.Validate(); err != nil {
					return nil, fmt.Errorf("the justification check produced a finding that cannot be reported: %w", err)
				}
				found = append(found, f)
			}
		}
		finding.Sort(found)

		return found, nil
	}
}
