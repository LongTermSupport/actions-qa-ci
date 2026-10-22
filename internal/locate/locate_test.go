package locate_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/locate"
)

const workflow = `name: CI
on: push
jobs:
  build:
    name: Build it
    runs-on: ubuntu-latest
    steps:
      - id: checkout
        name: Check out
        uses: actions/checkout@v1
      - name: Test
        run: |
          echo one
          echo two
      - run: echo unnamed
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: echo deploy
`

func TestFindSaysWhichJobAndStepALineIsIn(t *testing.T) {
	cases := []struct {
		name string
		line int
		want locate.Location
	}{
		{"on the job key", 4, locate.Location{Job: "build", JobName: "Build it"}},
		{"in the job outside any step", 6, locate.Location{Job: "build", JobName: "Build it"}},
		{"on the steps key", 7, locate.Location{Job: "build", JobName: "Build it"}},
		{"on the first step's first key", 8, locate.Location{Job: "build", JobName: "Build it", Step: "checkout", StepName: "Check out"}},
		{"inside the first step", 10, locate.Location{Job: "build", JobName: "Build it", Step: "checkout", StepName: "Check out"}},
		{"on the second step", 11, locate.Location{Job: "build", JobName: "Build it", StepName: "Test"}},
		{"in the second step's script", 14, locate.Location{Job: "build", JobName: "Build it", StepName: "Test"}},
		{"in the third step", 15, locate.Location{Job: "build", JobName: "Build it"}},
		{"on the second job", 16, locate.Location{Job: "deploy"}},
		{"in the second job's step", 19, locate.Location{Job: "deploy"}},
	}

	for _, c := range cases {
		got := locate.Find([]byte(workflow), c.line)

		if got != c.want {
			t.Errorf("%s (line %d): %+v, want %+v", c.name, c.line, got, c.want)
		}
	}
}

func TestFindSaysNothingAboutALineOutsideEveryJob(t *testing.T) {
	for _, line := range []int{0, 1, 2, 3, -1, 999} {
		if got := locate.Find([]byte(workflow), line); got != (locate.Location{}) {
			t.Errorf("line %d: %+v; a line outside every job has no location", line, got)
		}
	}
}

func TestFindSaysNothingWhenTheFileIsNotAWorkflowItCanRead(t *testing.T) {
	for name, data := range map[string]string{
		"not yaml":           "jobs: [\n",
		"no jobs":            "name: x\non: push\n",
		"jobs is a list":     "jobs:\n  - a\n",
		"a job is a scalar":  "jobs:\n  a: b\n",
		"empty":              "",
		"steps is a mapping": "jobs:\n  a:\n    steps:\n      run: x\n",
	} {
		got := locate.Find([]byte(data), 3)

		if got.Step != "" {
			t.Errorf("%s: %+v; no step can be named", name, got)
		}
	}
}

func TestFindDoesNotNameAJobOrStepWhoseNameIsNotText(t *testing.T) {
	data := "jobs:\n  a:\n    name: [1, 2]\n    steps:\n      - id: {x: y}\n        name: 7\n        run: echo\n"

	got := locate.Find([]byte(data), 7)

	if got.Job != "a" || got.JobName != "" || got.Step != "" || got.StepName != "7" {
		t.Errorf("%+v; a name that is a collection names nothing, and a number is its text", got)
	}
}

func TestFindReadsAJobAfterAnAliasWithoutFollowingIt(t *testing.T) {
	data := "x-steps: &steps\n  - run: echo\njobs:\n  a:\n    steps: *steps\n  b:\n    steps:\n      - run: echo b\n"

	got := locate.Find([]byte(data), 8)

	if got.Job != "b" {
		t.Errorf("%+v", got)
	}
}
