package record

import (
	"fmt"
	"strings"
	"unicode"
)

// RequiredCheck is a status check context the project requires to pass before a merge, on a
// branch. A blank branch is the repository's default branch.
type RequiredCheck struct {
	Context string `yaml:"context"`
	Branch  string `yaml:"branch"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// DefaultBranch is how the record names the repository's default branch, which a blank branch
// also means.
const DefaultBranch = "default"

// EffectiveBranch is the branch the requirement is for, with a blank one made explicit.
func (r RequiredCheck) EffectiveBranch() string {
	if r.Branch == "" {
		return DefaultBranch
	}

	return r.Branch
}

// requiredCheckFaults judges the requiredChecks section: each entry has a context that is text
// a check run can carry, a branch that is a plain name, and no requirement is stated twice.
func requiredCheckFaults(checks []RequiredCheck) []string {
	var faults []string

	stated := map[[2]string]bool{}
	for i, c := range checks {
		if c.null {
			continue // reported once, by Parse
		}
		where := fmt.Sprintf("required check %d (line %d)", i+1, c.Line)

		switch {
		case strings.TrimSpace(c.Context) == "":
			faults = append(faults, where+": context is missing")
		case strings.ContainsFunc(c.Context, unicode.IsControl):
			faults = append(faults, where+": context contains a control character")
		case c.Context != strings.TrimSpace(c.Context):
			faults = append(faults, fmt.Sprintf("%s: context %q has a space at its start or end, which a check name never has", where, c.Context))
		}

		switch {
		case c.Branch == "":
		case strings.ContainsFunc(c.Branch, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }):
			faults = append(faults, fmt.Sprintf("%s: branch %q contains a space or control character", where, c.Branch))
		case strings.ContainsAny(c.Branch, "*?[]{}\\"):
			faults = append(faults, fmt.Sprintf("%s: branch %q contains a pattern character; name one branch, or %s for the default", where, c.Branch, DefaultBranch))
		}

		key := [2]string{c.Context, c.EffectiveBranch()}
		if stated[key] {
			faults = append(faults, fmt.Sprintf("%s: context %q is required twice on branch %s", where, c.Context, c.EffectiveBranch()))
		}
		stated[key] = true
	}

	return faults
}
