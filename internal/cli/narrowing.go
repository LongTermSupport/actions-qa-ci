package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/LongTermSupport/actions-qa-ci/internal/exceptions"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// narrowByRecord marks, on the entries `rules` lists, each defence the project record turns off
// everywhere. A narrowing of the method's standard is the Owner's, so wherever the defences are
// listed it is shown beside the defence it narrows. A record that is not valid is refused here
// as it is by `check`: listing as if it were not there would hide the narrowing.
func narrowByRecord(env Env, entries []listing.Entry, stderr io.Writer) int {
	rec, code := validateRecord(env, stderr)
	if code != ExitOK {
		return code
	}
	if rec == nil {
		noteRecordAbove(stderr)

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

// noteRecordAbove says, when the working directory is below a repository root whose project
// record exists, that the record was not looked for here. Staying silent would read as nothing
// being narrowed.
func noteRecordAbove(stderr io.Writer) {
	if _, err := os.Stat(".git"); err == nil {
		return
	}

	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for parent := filepath.Dir(dir); parent != dir; dir, parent = parent, filepath.Dir(parent) {
		if _, err := os.Stat(filepath.Join(parent, ".git")); err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(parent, filepath.FromSlash(record.Path))); err == nil {
			fmt.Fprintf(stderr, "actions-qa: %s\n", finding.Escape("the project record is not looked for below the repository root; run from "+parent+" to see what it narrows"))
		}

		return
	}
}
