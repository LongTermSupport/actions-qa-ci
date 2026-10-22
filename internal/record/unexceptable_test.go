package record_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestTheChecksOnTheRecordItselfCannotBeExcepted(t *testing.T) {
	for _, id := range []string{"actionsqaci.unjustifiedException", "actionsqaci.staleException"} {
		for name, scope := range map[string]string{"with a path": "    path: .github/workflows/ci.yml\n", "without a path": ""} {
			data := "version: 1\nexceptions:\n  - id: " + id + "\n" + scope + "    hazard: a\n    scope: b\n    decision: c\n"

			rec, err := record.Parse([]byte(data), everythingIsKnown)

			if err == nil || !strings.Contains(err.Error(), "cannot be excepted") {
				t.Errorf("%s %s: record = %+v, err = %v", id, name, rec, err)
			}
		}
	}

	if !record.Unexceptable("actionsqaci.staleException") || record.Unexceptable("actionsqaci.actionlint.glob") {
		t.Error("Unexceptable names exactly the checks on the record")
	}
}
