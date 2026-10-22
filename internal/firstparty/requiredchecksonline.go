package firstparty

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/branchprotection"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// ReadProtection reads the contexts a branch of the repository requires; the branch
// record.DefaultBranch is the default one.
type ReadProtection func(branch string) (branchprotection.Result, error)

// CheckRequiredChecksOnline compares the record's requiredChecks with the protection the
// repository really has, for each branch the record names: a finding at the entry for a context
// the protection does not require, and one at the first entry for the branch for each context the
// protection requires that the record does not list. A protection that cannot be read is an error
// wrapping check.ErrNotAnalysable, never a clean result. It reports under the identifier of the
// offline check, which it completes. A record with no requiredChecks has nothing to compare, and
// nothing is read.
func CheckRequiredChecksOnline(known func(id string) bool, read ReadProtection) func(path string) ([]finding.Finding, error) {
	return func(recordFile string) ([]finding.Finding, error) {
		rec, err := record.Load(recordFile, known)
		if err != nil {
			return nil, err
		}

		// The protection is read once for each way the record spells a branch, and entries are
		// grouped by the branch it turns out to be: "default" and the default branch's own name
		// are one branch, and one protection.
		bySpelling := map[string]branchprotection.Result{}
		byBranch := map[string]branchprotection.Result{}
		var order []string
		groups := map[string][]record.RequiredCheck{}
		for _, required := range rec.RequiredChecks {
			spelling := required.EffectiveBranch()
			protection, done := bySpelling[spelling]
			if !done {
				var err error
				protection, err = read(spelling)
				if err != nil {
					return nil, fmt.Errorf("%w: the required checks on branch %s cannot be compared with its protection: %w", check.ErrNotAnalysable, spelling, err)
				}
				bySpelling[spelling] = protection
				byBranch[protection.Branch] = protection
			}
			if _, seen := groups[protection.Branch]; !seen {
				order = append(order, protection.Branch)
			}
			groups[protection.Branch] = append(groups[protection.Branch], required)
		}

		var found []finding.Finding
		for _, branch := range order {
			protection := byBranch[branch]
			entries := groups[branch]

			listed := map[string]bool{}
			for _, required := range entries {
				listed[required.Context] = true
				if slices.Contains(protection.Contexts, required.Context) {
					continue
				}
				message := fmt.Sprintf("the record requires %q on branch %s, but the protection of %s does not require it (it requires %s)",
					required.Context, required.EffectiveBranch(), branch, quoted(protection.Contexts))
				f, err := newFinding(RequiredChecks, recordFile, required.Line, 1, message)
				if err != nil {
					return nil, err
				}
				found = append(found, f)
			}

			for _, context := range protection.Contexts {
				if listed[context] {
					continue
				}
				message := fmt.Sprintf("the protection of %s requires %q, which the record does not list for that branch: list it, or remove it from the protection",
					branch, context)
				f, err := newFinding(RequiredChecks, recordFile, entries[0].Line, 1, message)
				if err != nil {
					return nil, err
				}
				found = append(found, f)
			}
		}
		finding.Sort(found)

		return found, nil
	}
}

// quoted lists the contexts in quotes, or says there are none.
func quoted(contexts []string) string {
	if len(contexts) == 0 {
		return "nothing"
	}
	items := make([]string, len(contexts))
	for i, c := range contexts {
		items[i] = fmt.Sprintf("%q", c)
	}

	return strings.Join(items, ", ")
}
