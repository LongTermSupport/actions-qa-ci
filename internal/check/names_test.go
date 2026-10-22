package check_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
)

// A file of the default set whose name cannot be printed on a line cannot be reported on, so
// the run says so at the start instead of failing later, and only if the file has a finding.
func TestDiscoverRefusesAFileWhoseNameCannotBePrintedOnALine(t *testing.T) {
	for name, file := range map[string]string{
		"a line break":    ".github/workflows/a\nb.yml",
		"an escape":       ".github/workflows/a\x1bb.yml",
		"a direction":     ".github/workflows/a" + string(rune(0x202e)) + "b.yml",
		"in an action":    ".github/actions/x\ny/action.yml",
		"a nested action": ".github/actions/ok/n\x07/action.yaml",
	} {
		got, err := check.Discover(fstest.MapFS{file: {Data: []byte("on: push\n")}})

		if got != nil || !errors.Is(err, check.ErrNotAnalysable) || !strings.Contains(err.Error(), "name") {
			t.Errorf("%s: Discover = %v, %v; want ErrNotAnalysable naming the problem", name, got, err)
		}
	}
}
