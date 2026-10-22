package actionlint

import (
	"strconv"
	"strings"

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

// isShellcheckCode accepts the identifiers ShellCheck's codes can take: its parser's and
// analyses' codes lie between SC1000 and SC3999. The exact set of codes is not enumerated yet,
// so a code in that range that ShellCheck never emits is accepted and simply never fires.
func isShellcheckCode(id string) bool {
	kind, err := finding.ClassifyIdentifier(id)
	if err != nil || kind != finding.KindShellcheck {
		return false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, finding.ToolPrefix+".shellcheck.SC"))

	return err == nil && number >= 1000 && number <= 3999
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
