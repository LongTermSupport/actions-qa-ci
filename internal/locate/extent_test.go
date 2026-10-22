package locate_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/locate"
)

func TestEveryLineBreakTheYAMLReaderCountsIsCountedHere(t *testing.T) {
	body := []string{"jobs:", "  a:", "    runs-on: x", "  b:", "    runs-on: y"}
	for name, breaks := range map[string]string{
		"next line":           string(rune(0x85)),
		"line separator":      string(rune(0x2028)),
		"paragraph separator": string(rune(0x2029)),
	} {
		inside := "jobs:\n  a:\n    runs-on: x # " + breaks + "\n  b:\n    runs-on: y\n"
		ending := strings.Join(body, "\n") + breaks

		// The break inside the comment adds a line of its own, so the last job's last line is the
		// sixth, but only if the file is counted the way the YAML reader counts it.
		for what, c := range map[string]struct {
			data string
			last int
		}{"inside a comment": {inside, 6}, "ending the file": {ending, 5}} {
			if !locate.Parse([]byte(c.data)).Within(c.last, "b", "") {
				t.Errorf("%s, %s: the last line is not in the last job", name, what)
			}
		}
	}
}

func TestAJobOrStepNamedAloneCoversOnlyThatOneAndNeverTheWholeFile(t *testing.T) {
	data := `jobs:
  a:
    name: Alpha
    steps:
      - id: one
        run: echo
      - id: two
        run: echo
  b:
    name: Beta
    steps:
      - id: one
        run: echo
`
	w := locate.Parse([]byte(data))

	for _, c := range []struct {
		name      string
		line      int
		job, step string
		want      bool
	}{
		{"a job by name, its own line", 6, "Alpha", "", true},
		{"a job by name, the other job's line", 13, "Alpha", "", false},
		{"a step alone, in a step of that name", 6, "", "one", true},
		{"a step alone, in a step of another name", 8, "", "one", false},
		{"a step alone, outside every step", 3, "", "one", false},
		{"a step alone, in the other job's step of that id", 13, "", "one", true},
		{"nothing wanted, inside a job", 3, "", "", true},
		{"nothing wanted, outside every job", 1, "", "", false},
	} {
		if got := w.Within(c.line, c.job, c.step); got != c.want {
			t.Errorf("%s: Within(%d, %q, %q) = %v, want %v", c.name, c.line, c.job, c.step, got, c.want)
		}
	}
}

func TestAnIDTwoJobsOrStepsShareNamesNoneOfThem(t *testing.T) {
	data := "jobs:\n  a:\n    steps:\n      - id: s\n        run: x\n      - id: s\n        run: y\n"
	duplicated := data + "  a:\n    steps:\n      - run: z\n"

	w := locate.Parse([]byte(data))
	if w.Within(5, "a", "s") || w.Within(7, "a", "s") || !w.Within(5, "a", "") {
		t.Error("a step id two steps share names neither, and the job is still the job")
	}
	d := locate.Parse([]byte(duplicated))
	if d.Within(5, "a", "") || d.Within(9, "a", "") {
		t.Error("a job id two jobs share names neither")
	}
}
