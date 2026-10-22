// Package catalogue holds the rules this build ships. It is the one place a rule is added to
// the program: every command that names a rule reads the registry built here.
package catalogue

import (
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Rules returns the individual rules this build ships, in no particular order. The bundled
// backends and the first-party checks add theirs here as they arrive. known says whether an
// identifier belongs to a defence of this build, for the checks that read the project record.
func Rules(known func(id string) bool) []registry.Rule {
	return append(actionlint.Rules(), firstparty.Rules(known)...)
}

// Lanes returns the analyses `check` runs, in the order it runs them. The actionlint lane
// carries ShellCheck's findings on the shell in run: steps; the first-party lane checks the
// project record. known says whether an identifier belongs to a defence of this build.
func Lanes(known func(id string) bool) []check.Lane {
	return []check.Lane{
		{
			Name:     "actionlint",
			Selects:  check.IsWorkflow,
			Run:      actionlint.Lint,
			Isolated: true,
		},
		{
			Name:    "firstParty",
			Selects: func(path string) bool { return path == record.Path },
			Run:     firstparty.CheckRecord(known),
		},
		{
			Name:    "templateDrift",
			Selects: func(path string) bool { return path == record.Path },
			Run:     firstparty.CheckTemplateDrift(known),
		},
		{
			Name:    "requiredChecks",
			Selects: func(path string) bool { return path == record.Path },
			Run:     firstparty.CheckRequiredChecks(known),
		},
		{
			// A comment that tells a tool to leave a finding out: a suppression the project
			// record cannot see.
			Name:    "inlineSuppression",
			Selects: func(path string) bool { return check.IsWorkflow(path) || check.IsActionFile(path) },
			Run:     firstparty.CheckInlineSuppression,
			Guard:   true,
		},
		{
			Name:    "nativeSuppressionConfig",
			Selects: check.IsNativeSuppressionConfig,
			Run:     firstparty.CheckNativeSuppressionConfig,
			Guard:   true,
		},
	}
}

// Extensions returns the bundled tools this build runs, with the releases they are at, for a
// SARIF log to name.
func Extensions() []output.Extension {
	return []output.Extension{
		{Name: "actionlint", Version: actionlint.Version},
		{Name: "shellcheck", Version: actionlint.ShellcheckVersion},
	}
}

// Families returns the families of rules this build ships: sets too large to register one rule
// at a time, which share a page.
func Families() []registry.Family {
	return actionlint.Families()
}

// Registry builds the registry of the rules this build ships. The checks that read the project
// record ask it which identifiers exist, so it is handed to them after it is built.
func Registry() (*registry.Registry, error) {
	var reg *registry.Registry
	known := func(id string) bool {
		if reg == nil {
			return false
		}
		_, ok := reg.Lookup(id)

		return ok
	}

	built, err := registry.NewWithFamilies(Families(), Rules(known)...)
	if err != nil {
		return nil, err
	}
	reg = built

	return reg, nil
}
