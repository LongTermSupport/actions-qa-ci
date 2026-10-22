package cli

import (
	"errors"
	"io"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/scope"
)

// includedFiles is the files the project record asks to have analysed beyond the default set
// (analyse.include), found among the files the repository tracks. A project with no record, or a
// record that includes nothing, reads no index. The code is ExitOK when the files could be found.
func includedFiles(rec *record.Record, stderr io.Writer) (check.Included, int) {
	if rec == nil || len(rec.Analyse.Include) == 0 {
		return check.Included{}, ExitOK
	}

	tracked, err := gitindex.Read(".")
	if err != nil {
		return check.Included{}, fault(stderr, err)
	}
	resolution, err := scope.Resolve(".", tracked, rec.Analyse.Include)
	if errors.Is(err, check.ErrNotAnalysable) {
		return check.Included{}, noVerdict(stderr, err)
	}
	if err != nil {
		return check.Included{}, fault(stderr, err)
	}

	return resolution.Included, ExitOK
}
