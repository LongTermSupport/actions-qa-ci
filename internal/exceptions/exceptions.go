// Package exceptions applies the project record's exceptions to findings. A finding is excepted
// when an exception names its identifier and its path (or no path, which disables the defence
// everywhere) and, if it names a job or step, the place the finding is in. An excepted finding
// is kept and marked, never dropped, so the output can count it and the record can be audited.
package exceptions

import (
	"fmt"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/locate"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Result is the findings with the excepted ones marked, and, for each exception of the
// record in order, whether it matched any finding.
type Result struct {
	Findings []finding.Finding
	Matched  []bool
}

// Apply marks the findings an exception covers. read returns the content of a repository file;
// it is asked for a file only when an exception that names a job or a step has to place a
// finding in it, and a failure to read it is an error: a finding that could not be judged is not
// passed on as if it had been. When several exceptions cover a finding, the first is the one it
// names, and all of them count as matched.
func Apply(rec *record.Record, findings []finding.Finding, read func(path string) ([]byte, error)) (Result, error) {
	result := Result{Findings: make([]finding.Finding, 0, len(findings)), Matched: make([]bool, len(rec.Exceptions))}
	files := map[string][]byte{}

	placed := func(path string, line int) (locate.Location, error) {
		content, cached := files[path]
		if !cached {
			var err error
			content, err = read(path)
			if err != nil {
				return locate.Location{}, fmt.Errorf("cannot read %s to place a finding in it: %w", path, err)
			}
			files[path] = content
		}

		return locate.Find(content, line), nil
	}

	for _, f := range findings {
		for i, e := range rec.Exceptions {
			if e.ID != f.ID || (e.Path != "" && e.Path != f.Path) {
				continue
			}
			if e.Job != "" || e.Step != "" {
				place, err := placed(f.Path, f.Line)
				if err != nil {
					return Result{}, err
				}
				if !named(e.Job, place.Job, place.JobName) || !named(e.Step, place.Step, place.StepName) {
					continue
				}
			}

			if !f.Excepted {
				f.Excepted = true
				f.ExceptionKey = fmt.Sprintf("%s:%d", record.Path, e.Line)
			}
			result.Matched[i] = true
		}
		result.Findings = append(result.Findings, f)
	}

	return result, nil
}

// named says whether an exception's job or step, when it has one, is the id or the name found.
func named(wanted, id, name string) bool {
	return wanted == "" || (id != "" && wanted == id) || (name != "" && wanted == name)
}

// Stale is a finding for each exception that matched nothing, so that the record cannot
// outlive the code it was written for. matched has one entry per exception of the record.
func Stale(rec *record.Record, matched []bool) []finding.Finding {
	var stale []finding.Finding

	for i, e := range rec.Exceptions {
		if i < len(matched) && matched[i] {
			continue
		}
		stale = append(stale, finding.Finding{
			ID:       firstparty.StaleException,
			Tool:     firstparty.Tool,
			Path:     record.Path,
			Line:     max(e.Line, 1),
			Col:      1,
			Severity: finding.SeverityError,
			Message:  fmt.Sprintf("exception %d (%s) matched no finding; remove it", i+1, describe(e)),
			DocPath:  registry.DocPath(firstparty.StaleException),
		})
	}

	return stale
}

// describe says where an exception applies, for a message about it.
func describe(e record.Exception) string {
	if e.Path == "" {
		return e.ID + ", everywhere"
	}

	parts := []string{e.ID, "in " + e.Path}
	if e.Job != "" {
		parts = append(parts, "job "+e.Job)
	}
	if e.Step != "" {
		parts = append(parts, "step "+e.Step)
	}

	return strings.Join(parts, ", ")
}

// Disabled is the exceptions that turn a defence off everywhere, in the order of the record.
func Disabled(rec *record.Record) []record.Exception {
	var disabled []record.Exception
	for _, e := range rec.Exceptions {
		if e.DisablesDefence() {
			disabled = append(disabled, e)
		}
	}

	return disabled
}
