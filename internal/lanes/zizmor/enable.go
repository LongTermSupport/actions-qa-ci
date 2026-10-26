package zizmor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// OptIn says whether the lane leaves the audit out unless the project record asks for it: one
// the regular persona does not report and this build is able to run.
func (a Audit) OptIn() bool {
	return a.WiderPersonaOnly && a.Held == ""
}

// Enabling returns Lint's run with each of the named audits (by identifier) switched on. An
// identifier that is not an audit of the opt-in kind is an error, so the project record cannot
// ask for something the lane would then not do.
func Enabling(ids []string) (func(path string) ([]finding.Finding, error), error) {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name, found := strings.CutPrefix(id, ID(""))
		wanted, known := audit(name)
		switch {
		case !found || !known:
			return nil, fmt.Errorf("%s is not an audit of zizmor's", finding.Escape(id))
		case !wanted.OptIn():
			return nil, fmt.Errorf("%s cannot be enabled: %s", finding.Escape(id), notOptIn(wanted))
		}
		names = append(names, name)
	}

	return func(path string) ([]finding.Finding, error) {
		binary, err := pinnedZizmor()
		if err != nil {
			return nil, err
		}

		return lintEnabling(binary, path, names, defaultLimits)
	}, nil
}

// notOptIn says why an audit is not one the record can enable.
func notOptIn(a Audit) string {
	if a.Held != "" {
		return "this build does not run it: " + a.Held
	}

	return "the lane already runs it"
}

// lintEnabling is the regular persona's findings with, for the audits named, the findings of the
// auditor persona. Nothing else of the wider persona is kept, so enabling one audit changes what
// no other audit reports.
func lintEnabling(binary, path string, names []string, bound limits) ([]finding.Finding, error) {
	// Both runs go through one call, so that they are held to the same limits.
	at := func(persona string) ([]finding.Finding, error) { return lint(binary, path, persona, bound) }

	regular, err := at(personaRegular)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return regular, nil
	}

	wider, err := at(personaAuditor)
	if err != nil {
		return nil, err
	}
	for _, f := range wider {
		if slices.Contains(names, f.UpstreamID) {
			regular = append(regular, f)
		}
	}
	finding.Sort(regular)

	return regular, nil
}
