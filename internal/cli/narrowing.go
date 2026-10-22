package cli

import (
	"fmt"
	"io"

	"github.com/LongTermSupport/actions-qa-ci/internal/exceptions"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// narrowByRecord marks, on the entries `rules` lists, each defence the project record turns off
// everywhere. A narrowing of the method's standard is the Owner's, so wherever the defences are
// listed it is shown beside the defence it narrows. A record that is not valid is refused here
// as it is by `check`: listing as if it were not there would hide the narrowing.
func narrowByRecord(env Env, entries []ruleEntry, stderr io.Writer) int {
	rec, code := validateRecord(env, stderr)
	if code != ExitOK {
		return code
	}
	if rec == nil {
		return ExitOK
	}

	for _, e := range exceptions.Disabled(rec) {
		narrowing := fmt.Sprintf("%s is disabled everywhere by %s:%d", e.ID, record.Path, e.Line)
		for i := range entries {
			if entries[i].ID == e.ID {
				entries[i].NarrowedBy = append(entries[i].NarrowedBy, narrowing)

				break
			}
			if family, listed := env.Registry.FamilyNamed(entries[i].ID); listed && family.Member(e.ID) {
				entries[i].NarrowedBy = append(entries[i].NarrowedBy, narrowing)

				break
			}
		}
	}

	return ExitOK
}
