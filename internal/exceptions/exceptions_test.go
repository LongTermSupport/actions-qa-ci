package exceptions_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/exceptions"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const (
	glob     = "actionsqaci.actionlint.glob"
	syntax   = "actionsqaci.actionlint.syntax-check"
	release  = ".github/workflows/release.yml"
	other    = ".github/workflows/other.yml"
	workflow = `name: Release
on: push
jobs:
  release:
    name: Publish
    runs-on: ubuntu-latest
    steps:
      - id: tag
        name: Tag it
        run: echo tag
      - name: Push it
        run: echo push
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: echo verify
`
)

func at(id, path string, line int) finding.Finding {
	return finding.Finding{
		ID: id, Tool: "t", Path: path, Line: line, Col: 1,
		Severity: finding.SeverityError, Message: "m",
	}
}

func reader(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}

		return []byte(content), nil
	}
}

func exception(id, path, job, step string, line int) record.Exception {
	return record.Exception{ID: id, Path: path, Job: job, Step: step, Hazard: "h", Scope: "s", Decision: "d", Line: line}
}

func apply(t *testing.T, exceptionList []record.Exception, findings []finding.Finding) exceptions.Result {
	t.Helper()

	result, err := exceptions.Apply(&record.Record{Version: "1", Exceptions: exceptionList}, findings, reader(map[string]string{release: workflow, other: workflow}))
	if err != nil {
		t.Fatal(err)
	}

	return result
}

func TestAnExceptionExceptsTheFindingsItNamesAndNoOthers(t *testing.T) {
	findings := []finding.Finding{
		at(glob, release, 5),
		at(glob, other, 5),
		at(syntax, release, 5),
	}

	result := apply(t, []record.Exception{exception(glob, release, "", "", 12)}, findings)

	got := []bool{result.Findings[0].Excepted, result.Findings[1].Excepted, result.Findings[2].Excepted}
	if len(result.Findings) != 3 || got[0] != true || got[1] != false || got[2] != false {
		t.Errorf("excepted = %v; only the finding with the same identifier and path is", got)
	}
	if result.Findings[0].ExceptionKey != record.Path+":12" {
		t.Errorf("key = %q, want the record and the line of the exception", result.Findings[0].ExceptionKey)
	}
	if err := result.Findings[0].Validate(); err != nil {
		t.Error(err)
	}
}

func TestAnExceptionWithNoPathDisablesTheDefenceEverywhere(t *testing.T) {
	findings := []finding.Finding{at(glob, release, 5), at(glob, other, 9), at(syntax, other, 9)}

	result := apply(t, []record.Exception{exception(glob, "", "", "", 3)}, findings)

	if !result.Findings[0].Excepted || !result.Findings[1].Excepted || result.Findings[2].Excepted {
		t.Errorf("findings = %+v", result.Findings)
	}
}

func TestAnExceptionNarrowedToAJobOrAStepCoversOnlyThatPlace(t *testing.T) {
	cases := []struct {
		name      string
		job, step string
		line      int
		want      bool
	}{
		{"the job by id", "release", "", 5, true},
		{"the job by name", "Publish", "", 5, true},
		{"the job, from a line in a step", "release", "", 11, true},
		{"another job", "verify", "", 5, false},
		{"the job, a line in the other job", "release", "", 16, false},
		{"the step by id", "release", "tag", 9, true},
		{"the step by name", "release", "Push it", 12, true},
		{"the step, a line in another step", "release", "tag", 12, false},
		{"the step, a line outside every step", "release", "tag", 5, false},
		{"a step with no job named", "", "tag", 9, true},
		{"the step of another job", "verify", "tag", 9, false},
		{"a name that is nobody's", "nobody", "", 5, false},
		{"a line outside every job", "release", "", 2, false},
	}

	for _, c := range cases {
		result := apply(t, []record.Exception{exception(glob, release, c.job, c.step, 4)}, []finding.Finding{at(glob, release, c.line)})

		if got := result.Findings[0].Excepted; got != c.want {
			t.Errorf("%s: excepted = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAFindingInAFileThatCannotBePlacedIsNeverExceptedByAJobOrStep(t *testing.T) {
	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "release", "", 4)}},
		[]finding.Finding{at(glob, release, 5)},
		reader(map[string]string{release: "jobs: [\n"}),
	)

	if err != nil || result.Findings[0].Excepted {
		t.Errorf("result = %+v, err = %v; what cannot be placed stays reported", result.Findings, err)
	}
}

func TestApplyDoesNotReadAFileAnExceptionWithNoJobOrStepDoesNotNeed(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errors.New("must not be read") }

	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "", "", 4)}},
		[]finding.Finding{at(glob, release, 5)}, read)

	if err != nil || !result.Findings[0].Excepted {
		t.Errorf("result = %+v, err = %v", result.Findings, err)
	}
}

func TestApplyFailsWhenAFileItMustReadCannotBeRead(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errors.New("disk on fire") }

	result, err := exceptions.Apply(
		&record.Record{Version: "1", Exceptions: []record.Exception{exception(glob, release, "release", "", 4)}},
		[]finding.Finding{at(glob, release, 5)}, read)

	if err == nil || !strings.Contains(err.Error(), "disk on fire") || len(result.Findings) != 0 {
		t.Errorf("result = %+v, err = %v; a finding that could not be judged must not be passed on as if it had been", result.Findings, err)
	}
}

func TestEachExceptionSaysWhetherItMatchedAnything(t *testing.T) {
	findings := []finding.Finding{at(glob, release, 5)}
	list := []record.Exception{
		exception(glob, release, "", "", 4),
		exception(glob, release, "", "", 5),
		exception(syntax, release, "", "", 6),
		exception(glob, other, "", "", 7),
		exception(glob, release, "verify", "", 8),
	}

	result := apply(t, list, findings)

	want := []bool{true, true, false, false, false}
	for i := range want {
		if result.Matched[i] != want[i] {
			t.Errorf("exception %d matched = %v, want %v", i+1, result.Matched[i], want[i])
		}
	}
	if result.Findings[0].ExceptionKey != record.Path+":4" {
		t.Errorf("key = %q; when two exceptions match, the first is the one named", result.Findings[0].ExceptionKey)
	}
}

func TestStaleFindsTheExceptionsThatMatchedNothing(t *testing.T) {
	rec := &record.Record{Version: "1", Exceptions: []record.Exception{
		exception(glob, release, "", "", 4),
		exception(syntax, other, "job", "", 9),
	}}

	got := exceptions.Stale(rec, []bool{true, false})

	if len(got) != 1 || got[0].ID != "actionsqaci.staleException" || got[0].Path != record.Path || got[0].Line != 9 {
		t.Fatalf("stale = %+v", got)
	}
	if !strings.Contains(got[0].Message, "exception 2") || !strings.Contains(got[0].Message, syntax) || !strings.Contains(got[0].Message, other) {
		t.Errorf("message = %q; it should name the exception, its identifier and its path", got[0].Message)
	}
	if err := got[0].Validate(); err != nil {
		t.Error(err)
	}
	if exceptions.Stale(rec, []bool{true, true}) != nil {
		t.Error("an exception that matched is not stale")
	}
}

func TestDisabledListsTheExceptionsThatTurnADefenceOffEverywhere(t *testing.T) {
	rec := &record.Record{Version: "1", Exceptions: []record.Exception{
		exception(glob, release, "", "", 4),
		exception(syntax, "", "", "", 9),
	}}

	got := exceptions.Disabled(rec)

	if len(got) != 1 || got[0].ID != syntax || got[0].Line != 9 {
		t.Errorf("disabled = %+v", got)
	}
}
