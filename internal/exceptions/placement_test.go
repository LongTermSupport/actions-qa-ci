package exceptions_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/exceptions"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestStaleFindingsAreErrorsAtColumnOneFromThisTool(t *testing.T) {
	rec := &record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "release", "", 4)}}

	got := exceptions.Stale(rec, nil)

	if len(got) != 1 || got[0].Severity != finding.SeverityError || got[0].Col != 1 || got[0].Tool != "actions-qa-ci" ||
		!strings.Contains(got[0].Message, "matched no finding") || !strings.Contains(got[0].Message, "job release") {
		t.Errorf("stale = %+v", got)
	}
}

func TestStaleMessagesNameEachPartOfWhereTheExceptionApplied(t *testing.T) {
	rec := &record.Record{Version: "1", Exceptions: []record.Exception{
		exception(glob, "", "", "", 3),
		exception(glob, release, "", "", 4),
		exception(glob, release, "release", "", 5),
		exception(glob, release, "release", "tag", 6),
	}}

	got := exceptions.Stale(rec, nil)

	want := []string{
		"exception 1 (" + glob + ", everywhere) ",
		"exception 2 (" + glob + ", in " + release + ") ",
		"exception 3 (" + glob + ", in " + release + ", job release) ",
		"exception 4 (" + glob + ", in " + release + ", job release, step tag) ",
	}
	if len(got) != len(want) {
		t.Fatalf("stale = %+v", got)
	}
	for i := range want {
		if !strings.HasPrefix(got[i].Message, want[i]) {
			t.Errorf("message %d = %q, want it to begin %q", i+1, got[i].Message, want[i])
		}
	}
}

func TestAnExceptionByAnIDIsNotAlsoAnExceptionForAJobOnlyNamedLikeIt(t *testing.T) {
	data := "jobs:\n  test:\n    name: build\n    steps:\n      - run: echo\n  build:\n    name: Other\n    steps:\n      - run: echo\n"
	findings := []finding.Finding{at(glob, release, 5), at(glob, release, 9)}

	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "build", "", 4)}},
		findings, reader(map[string]string{release: data}))

	if err != nil || result.Findings[0].Excepted || !result.Findings[1].Excepted {
		t.Errorf("result = %+v, err = %v; only the job whose id is build is excepted", result.Findings, err)
	}
}

func TestAKeyAfterTheLastStepOrJobIsNotCoveredByAnExceptionForThem(t *testing.T) {
	data := "jobs:\n  a:\n    steps:\n      - name: only\n        run: echo\n    env:\n      X: 1\nenv:\n  Y: 2\n"
	findings := []finding.Finding{at(glob, release, 5), at(glob, release, 7), at(glob, release, 9)}

	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "a", "only", 4)}},
		findings, reader(map[string]string{release: data}))

	got := []bool{result.Findings[0].Excepted, result.Findings[1].Excepted, result.Findings[2].Excepted}
	if err != nil || !got[0] || got[1] || got[2] {
		t.Errorf("excepted = %v, err = %v; only the finding in the step is covered", got, err)
	}
}

func TestAFileWithNoTrailingNewlineStillPlacesItsLastLine(t *testing.T) {
	data := strings.TrimSuffix(workflow, "\n")

	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "verify", "", 4)}},
		[]finding.Finding{at(glob, release, 16)}, reader(map[string]string{release: data}))

	if err != nil || !result.Findings[0].Excepted {
		t.Errorf("result = %+v, err = %v", result.Findings, err)
	}
}
