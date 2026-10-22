package actionlint

import (
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Families returns the one family of rules this lane reports besides its kinds: ShellCheck's
// codes. They share a page because a page per code cannot state each code's construction.
func Families() []registry.Family {
	return []registry.Family{{
		Name:     finding.ToolPrefix + ".shellcheck.SC<nnnn>",
		Page:     finding.ToolPrefix + ".shellcheck",
		Summary:  "Shell in a run: step has no ShellCheck error or warning",
		Upstream: shellcheckTool,
		Member:   isShellcheckCode,
		Check:    checkCode,
	}}
}

func isShellcheckCode(id string) bool {
	kind, err := finding.ClassifyIdentifier(id)

	return err == nil && kind == finding.KindShellcheck
}

func checkCode(id string) func(string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		all, err := Lint(path)
		if err != nil {
			return nil, err
		}

		var own []finding.Finding
		for _, f := range all {
			if f.ID == id {
				own = append(own, f)
			}
		}

		return own, nil
	}
}
