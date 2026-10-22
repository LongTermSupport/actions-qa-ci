package locate_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/locate"
)

func within(t *testing.T, data string, line int, job, step string) bool {
	t.Helper()

	return locate.Parse([]byte(data)).Within(line, job, step)
}

func TestAKeyAfterTheLastJobIsInNoJob(t *testing.T) {
	data := "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: echo\n  last:\n    runs-on: x\n    steps:\n      - run: echo\nenv:\n  X: 1\n"

	for line, want := range map[int]bool{8: true, 10: true, 11: false, 12: false} {
		if got := within(t, data, line, "last", ""); got != want {
			t.Errorf("line %d: Within(job last) = %v, want %v", line, got, want)
		}
	}
	if got := locate.Parse([]byte(data)).Find(11); got != (locate.Location{}) {
		t.Errorf("Find(11) = %+v; a top-level key after the jobs is in no job", got)
	}
}

func TestAKeyAfterTheStepsIsInTheJobAndInNoStep(t *testing.T) {
	data := "jobs:\n  a:\n    runs-on: x\n    steps:\n      - name: only\n        run: echo\n    env:\n      X: 1\n    timeout-minutes: 5\n  b:\n    runs-on: x\n"

	for line, c := range map[int]struct{ job, step bool }{
		5: {true, true}, 6: {true, true}, 7: {true, false}, 8: {true, false}, 9: {true, false},
	} {
		if got := within(t, data, line, "a", ""); got != c.job {
			t.Errorf("line %d: Within(job a) = %v, want %v", line, got, c.job)
		}
		if got := within(t, data, line, "a", "only"); got != c.step {
			t.Errorf("line %d: Within(job a, step only) = %v, want %v", line, got, c.step)
		}
	}
}

func TestAnIDWinsOverANameAndAnAmbiguousNameMatchesNothing(t *testing.T) {
	data := `jobs:
  test:
    name: build
    steps:
      - id: build
        run: echo
      - name: build
        run: echo
  build:
    name: Other
    steps:
      - name: dup
        run: echo
      - name: dup
        run: echo
  one:
    name: shared
    steps:
      - run: echo
  two:
    name: shared
    steps:
      - run: echo
`
	cases := []struct {
		name      string
		line      int
		job, step string
		want      bool
	}{
		{"the job whose id it is", 10, "build", "", true},
		{"a job only named so, when an id is", 3, "build", "", false},
		{"the step whose id it is", 5, "test", "build", true},
		{"a step only named so, when an id is", 7, "test", "build", false},
		{"a name that is unique", 10, "Other", "", true},
		{"a step name two steps share", 12, "build", "dup", false},
		{"a job name two jobs share", 17, "shared", "", false},
		{"the id of one of them", 17, "one", "", true},
	}
	for _, c := range cases {
		if got := within(t, data, c.line, c.job, c.step); got != c.want {
			t.Errorf("%s: Within(line %d, %q, %q) = %v, want %v", c.name, c.line, c.job, c.step, got, c.want)
		}
	}
}

func TestStepsOrJobsSharingALineCannotBeToldApart(t *testing.T) {
	steps := "jobs:\n  a:\n    steps: [{name: A, run: x}, {name: B, run: y}]\n"
	if got := locate.Parse([]byte(steps)).Find(3); got.Job != "a" || got.Step != "" || got.StepName != "" {
		t.Errorf("Find = %+v; the job is known, the step is not", got)
	}
	if within(t, steps, 3, "a", "A") || within(t, steps, 3, "a", "B") || !within(t, steps, 3, "a", "") {
		t.Error("a step on a line shared with another matches no step exception, and the job still matches")
	}

	jobs := "jobs: {a: {name: A}, b: {name: B}}\n"
	if got := locate.Parse([]byte(jobs)).Find(1); got != (locate.Location{}) {
		t.Errorf("Find = %+v; jobs sharing a line cannot be told apart", got)
	}
	if within(t, jobs, 1, "a", "") || within(t, jobs, 1, "b", "") {
		t.Error("a job on a line shared with another matches no job exception")
	}
}

func TestAnAliasMakesTheWorkflowUnplaceableRatherThanGuessed(t *testing.T) {
	data := "jobs:\n  b1:\n    steps: &s\n      - run: echo\n  b2:\n    steps: *s\n"

	if got := locate.Parse([]byte(data)).Find(4); got != (locate.Location{}) {
		t.Errorf("Find = %+v", got)
	}
	if within(t, data, 4, "b1", "") || within(t, data, 4, "b2", "") {
		t.Error("a finding that two jobs share through an alias belongs to neither by exception")
	}
}

func TestLinesAreCountedWhateverTheLineEndingsAndTheEndOfTheFile(t *testing.T) {
	body := []string{"jobs:", "  a:", "    runs-on: x", "  b:", "    runs-on: y"}
	cases := map[string]string{
		"lf":                strings.Join(body, "\n") + "\n",
		"no final newline":  strings.Join(body, "\n"),
		"crlf":              strings.Join(body, "\r\n") + "\r\n",
		"cr only":           strings.Join(body, "\r") + "\r",
		"cr only, no final": strings.Join(body, "\r"),
	}

	for name, data := range cases {
		if !within(t, data, 5, "b", "") || within(t, data, 5, "a", "") || !within(t, data, 3, "a", "") {
			t.Errorf("%s: the last line is not in the last job", name)
		}
	}
}

func TestFindIsTheSameAsParsingThenFinding(t *testing.T) {
	if locate.Find([]byte(workflow), 8) != locate.Parse([]byte(workflow)).Find(8) {
		t.Error("Find and Parse().Find disagree")
	}
}

func TestAnUnreadableWorkflowIsUnplaceable(t *testing.T) {
	for name, data := range map[string]string{"not yaml": "jobs: [\n", "empty": "", "a sequence": "- a\n", "jobs a list": "jobs:\n  - a\n"} {
		w := locate.Parse([]byte(data))

		if w.Find(1) != (locate.Location{}) || w.Within(1, "a", "") || w.Within(1, "", "") {
			t.Errorf("%s: placed something", name)
		}
	}
}
