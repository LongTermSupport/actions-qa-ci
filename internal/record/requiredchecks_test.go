package record_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestParseReadsTheRequiredChecksWithTheirLines(t *testing.T) {
	data := "version: 1\nrequiredChecks:\n  - context: QA Pipeline\n    branch: default\n  - context: Lint\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if len(rec.RequiredChecks) != 2 {
		t.Fatalf("required checks = %+v", rec.RequiredChecks)
	}
	first, second := rec.RequiredChecks[0], rec.RequiredChecks[1]
	if first.Context != "QA Pipeline" || first.Branch != "default" || first.Line != 3 {
		t.Errorf("first = %+v; want its context, branch and line 3", first)
	}
	if second.Context != "Lint" || second.Branch != "" || second.Line != 5 {
		t.Errorf("second = %+v; want no branch (the default one) and line 5", second)
	}
}

func TestARecordKeepsRequiredChecksTemplatesAndExceptionsAtOnce(t *testing.T) {
	data := "version: 1\nrequiredChecks:\n  - context: QA Pipeline\ntemplates:\n  - source: a.yml\n    copies: [b.yml]\nexceptions:\n" + goodException

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil || len(rec.RequiredChecks) != 1 || len(rec.Templates) != 1 || len(rec.Exceptions) != 1 {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
	if err == nil && (rec.Templates[0].Line != 5 || rec.Exceptions[0].Line != 8) {
		t.Errorf("lines = %d and %d; want 5 and 8", rec.Templates[0].Line, rec.Exceptions[0].Line)
	}
}

func TestParseRefusesARequiredCheckThatNamesNothingUsable(t *testing.T) {
	cases := map[string]struct{ data, mention string }{
		"no context":          {"version: 1\nrequiredChecks:\n  - branch: default\n", "context"},
		"a blank context":     {"version: 1\nrequiredChecks:\n  - context: '   '\n", "context"},
		"an empty entry":      {"version: 1\nrequiredChecks:\n  -\n", "empty"},
		"leading space":       {"version: 1\nrequiredChecks:\n  - context: ' QA Pipeline'\n", "space"},
		"trailing space":      {"version: 1\nrequiredChecks:\n  - context: 'QA Pipeline '\n", "space"},
		"a control character": {"version: 1\nrequiredChecks:\n  - context: \"QA\\tPipeline\"\n", "control"},
		"a line break":        {"version: 1\nrequiredChecks:\n  - context: \"QA\\nPipeline\"\n", "control"},
		"the same twice":      {"version: 1\nrequiredChecks:\n  - context: QA Pipeline\n  - context: QA Pipeline\n", "twice"},
		"a branch with space": {"version: 1\nrequiredChecks:\n  - context: QA\n    branch: my branch\n", "branch"},
		"a glob branch":       {"version: 1\nrequiredChecks:\n  - context: QA\n    branch: 'release/*'\n", "branch"},
		"an unknown field":    {"version: 1\nrequiredChecks:\n  - context: QA\n    strict: true\n", "strict"},
		"as a map":            {"version: 1\nrequiredChecks:\n  context: QA\n", "unmarshal"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), everythingIsKnown)

		var invalid *record.InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v; want the record refused as invalid", name, err)

			continue
		}
		if !strings.Contains(strings.Join(invalid.Reasons, "\n"), c.mention) {
			t.Errorf("%s: reasons %q should mention %q", name, invalid.Reasons, c.mention)
		}
	}
}

func TestTheSameContextOnTwoBranchesIsTwoRequirements(t *testing.T) {
	data := "version: 1\nrequiredChecks:\n  - context: QA\n    branch: default\n  - context: QA\n    branch: release\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil || len(rec.RequiredChecks) != 2 {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
}

func TestRequiredChecksRefuseEveryIndirectionToo(t *testing.T) {
	cases := map[string]string{
		"an alias":  "version: 1\nrequiredChecks:\n  - context: &c QA\n  - context: *c\n    branch: x\n",
		"an anchor": "version: 1\nrequiredChecks:\n  - &r\n    context: QA\n",
		"a tag":     "version: 1\nrequiredChecks:\n  - context: !!str QA\n",
	}

	for name, data := range cases {
		if _, err := record.Parse([]byte(data), everythingIsKnown); err == nil {
			t.Errorf("%s: the record was accepted", name)
		}
	}
}
