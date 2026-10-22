package actionlint

import (
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Kind is one kind of finding actionlint reports, with the standing instruction its rule states.
type Kind struct {
	Name    string
	Summary string
}

// kinds is every kind actionlint reports as itself, in no order that matters. Findings it
// reports from the shellcheck and pyflakes integrations are other tools' and are not here.
var kinds = []Kind{
	{"syntax-check", "A workflow or action file is valid GitHub Actions syntax"},
	{"matrix", "A matrix has no duplicate values and no combination that matches nothing"},
	{"credentials", "Container and service credentials come from secrets, never as literals"},
	{"shell-name", "A step's shell is one GitHub provides"},
	{"runner-label", "runs-on names a runner label that exists"},
	{"events", "on: names real webhook events with valid event configuration"},
	{"job-needs", "needs: names jobs that exist and form no cycle"},
	{"action", "uses: names a well-formed action reference with a version and valid inputs"},
	{"env-var", "Environment variable names contain no space, = or &"},
	{"id", "Job and step IDs are unique and well formed"},
	{"glob", "Branch, tag and path filters are valid glob patterns"},
	{"permissions", "permissions: uses real scopes with read, write or none"},
	{"workflow-call", "A reusable-workflow call passes the inputs and secrets the callee declares"},
	{"expression", "Every ${{ }} expression parses and type-checks"},
	{"deprecated-commands", "No set-output, save-state, set-env or add-path workflow command"},
	{"if-cond", "An if: condition is not always true or always false"},
}

// Kinds returns every kind of finding the lane can report.
func Kinds() []Kind {
	return append([]Kind(nil), kinds...)
}

func known(kind string) bool {
	for _, k := range kinds {
		if k.Name == kind {
			return true
		}
	}

	return false
}

// ID is the stable identifier of an actionlint kind.
func ID(kind string) string {
	return finding.ToolPrefix + ".actionlint." + kind
}

// Rules returns one rule per kind. Each runs the whole lane on a file and keeps only its own
// kind, so the harness shows exactly what that rule prints.
func Rules() []registry.Rule {
	rules := make([]registry.Rule, 0, len(kinds))
	for _, kind := range kinds {
		rules = append(rules, registry.Rule{
			ID:       ID(kind.Name),
			Summary:  kind.Summary,
			Upstream: Tool,
			Check:    checkKind(kind.Name),
		})
	}

	return rules
}

func checkKind(kind string) func(string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		all, err := Lint(path)
		if err != nil {
			return nil, err
		}

		var own []finding.Finding
		for _, f := range all {
			if f.UpstreamID == kind {
				own = append(own, f)
			}
		}

		return own, nil
	}
}
