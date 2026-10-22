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
	StaleException       = finding.ToolPrefix + ".staleException"
)

// Rules returns the first-party checks. known says whether an identifier belongs to a defence
// of this build, which the record's exceptions have to name.
func Rules(known func(id string) bool) []registry.Rule {
	return []registry.Rule{{
		ID:      UnjustifiedException,
		Summary: "Every exception in the project record states a hazard, a scope and a decision, and every unanalysed file a hazard and a scope, none of them generic",
		Check:   CheckRecord(known),
	}, {
		// Reported by `check` when it has run every lane over every file: an exception can be
		// judged stale only against a whole run, so this rule cannot run on one file.
		ID:      StaleException,
		Summary: "Every exception in the project record matches a finding in a full run",
	}, {
		ID:      TemplateDrift,
		Summary: "Each declared copy of a template is byte-identical to its source, and a missing source or copy is a finding",
		Check:   CheckTemplateDrift(known),
	}, {
		ID:      InlineSuppression,
		Summary: "No comment tells zizmor, ratchet or ShellCheck to leave a finding out",
		Check:   CheckInlineSuppression,
	}, {
		ID:      NativeSuppressionConfig,
		Summary: "No tool's own configuration file suppresses a finding: zizmor's file, an actionlint ignore, a pinact ignore_actions",
		Check:   CheckNativeSuppressionConfig,
	}, {
		ID:      RequiredChecks,
		Summary: "Every status check the project record requires is the name of a job in a workflow triggered on pull_request",
		Check:   CheckRequiredChecks(known),
	}, lintCoverageRule(known)}
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
		for i, u := range rec.Analyse.Unanalysed {
			for _, fault := range u.Faults() {
				f := finding.Finding{
					ID:       UnjustifiedException,
					Tool:     Tool,
					Path:     path,
					Line:     max(u.Line, 1),
					Col:      1,
					Severity: finding.SeverityError,
					Message:  fmt.Sprintf("%s of unanalysed entry %d (%s) %s", fault.Field, i+1, u.Path, fault.Reason),
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
