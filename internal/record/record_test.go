package record_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func everythingIsKnown(string) bool { return true }

const goodException = `  - id: actionsqaci.actionlint.glob
    path: .github/workflows/release.yml
    job: release
    hazard: "A branch filter with a character class the glob matcher reads differently from GitHub's"
    scope: "Only the release job, whose filter is generated and checked against GitHub's behaviour by hand"
    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123
`

func TestParseReadsAVersionAndItsExceptionsWithTheirLines(t *testing.T) {
	rec, err := record.Parse([]byte("version: 1\nexceptions:\n"+goodException), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Version != "1" || len(rec.Exceptions) != 1 {
		t.Fatalf("record = %+v", rec)
	}
	e := rec.Exceptions[0]
	if e.ID != "actionsqaci.actionlint.glob" || e.Path != ".github/workflows/release.yml" || e.Job != "release" || e.Step != "" {
		t.Errorf("exception = %+v", e)
	}
	if e.Line != 3 {
		t.Errorf("line = %d, want the line of the entry's first key (3)", e.Line)
	}
	if !strings.HasPrefix(e.Decision, "https://") || e.Hazard == "" || e.Scope == "" {
		t.Errorf("justification fields lost: %+v", e)
	}
}

func TestAnExceptionNeedsNoPathToDisableADefenceEverywhere(t *testing.T) {
	data := "version: 1\nexceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: a\n    scope: b\n    decision: c\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if len(rec.Exceptions) != 1 || rec.Exceptions[0].Path != "" {
		t.Errorf("record = %+v", rec)
	}
	if !rec.Exceptions[0].DisablesDefence() {
		t.Error("an exception with no path disables the defence everywhere, and says so")
	}
}

func TestAVersionOnlyRecordIsValid(t *testing.T) {
	rec, err := record.Parse([]byte("version: 1\n"), everythingIsKnown)
	if err != nil || rec.Version != "1" || len(rec.Exceptions) != 0 {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
}

func TestParseRefusesWhatIsNotAWellFormedRecord(t *testing.T) {
	type refusal struct{ data, mention string }
	exception := func(fields string) string {
		return "version: 1\nexceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: a\n    scope: b\n    decision: c\n" + fields
	}
	cases := map[string]refusal{
		"empty":                {"", "version"},
		"no version":           {"exceptions: []\n", "version"},
		"blank version":        {"version: ''\n", "version"},
		"not yaml":             {"version: [\n", "not valid"},
		"a list at the top":    {"- version\n", "not valid"},
		"unknown top field":    {"version: 1\ncalibrations: {}\n", "calibrations"},
		"unknown entry field":  {exception("    reason: because\n"), "reason"},
		"duplicate key":        {"version: 1\nversion: 2\n", "already defined"},
		"second document":      {"version: 1\n---\nversion: 2\n", "more than one document"},
		"no id":                {"version: 1\nexceptions:\n  - path: a.yml\n", "id"},
		"unknown id":           {"version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n    hazard: a\n    scope: b\n    decision: c\n", "unknown"},
		"id not in the scheme": {"version: 1\nexceptions:\n  - id: Not An Identifier\n", "scheme"},
		"absolute path":        {exception("    path: /etc/passwd\n"), "path"},
		"parent path":          {exception("    path: ../x.yml\n"), "path"},
		"backslash path":       {exception("    path: a\\b.yml\n"), "path"},
		"glob path":            {exception("    path: .github/workflows/*.yml\n"), "path"},
		"job without a path":   {exception("    job: release\n"), "path"},
		"step without a path":  {exception("    step: build\n"), "path"},
		"scalar exceptions":    {"version: 1\nexceptions: yes\n", "not valid"},
		"a number for an id":   {"version: 1\nexceptions:\n  - id: 7\n", "scheme"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			known := func(id string) bool { return id == "actionsqaci.actionlint.glob" }

			rec, err := record.Parse([]byte(c.data), known)
			if err == nil {
				t.Fatalf("Parse accepted %s: %+v", name, rec)
			}
			if !strings.Contains(err.Error(), c.mention) {
				t.Errorf("the error for %s must mention %q, got %q", name, c.mention, err.Error())
			}
		})
	}
}

func TestParseReportsEveryFaultOfARecordAtOnce(t *testing.T) {
	data := "version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n    path: /abs\n  - id: actionsqaci.alsoNonsense\n"

	_, err := record.Parse([]byte(data), func(string) bool { return false })
	if err == nil {
		t.Fatal("accepted")
	}

	for _, want := range []string{"actionsqaci.nonsense", "/abs", "actionsqaci.alsoNonsense"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error leaves out %q: %q", want, err.Error())
		}
	}
}

func TestParseRefusesARecordTooLargeToBeOneAHumanWrote(t *testing.T) {
	data := "version: 1\n# " + strings.Repeat("x", 2<<20) + "\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
}

func TestLoadReadsTheRecordFileAndNamesItInAnError(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yml")
	if err := os.WriteFile(good, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(bad, []byte("version: 1\nextra: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rec, err := record.Load(good, everythingIsKnown)
	if err != nil || rec.Version != "1" {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
	rec, err = record.Load(bad, everythingIsKnown)
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("a refused record must be named by its file, got %+v and %v", rec, err)
	}
	rec, err = record.Load(filepath.Join(dir, "absent.yml"), everythingIsKnown)
	if err == nil {
		t.Errorf("a missing record was loaded: %+v", rec)
	}
	rec, err = record.Load(dir, everythingIsKnown)
	if err == nil {
		t.Errorf("a directory was loaded as a record: %+v", rec)
	}
}

func TestTheRecordLivesAtOneFixedPath(t *testing.T) {
	if record.Path != ".github/actions-qa-ci.yml" {
		t.Errorf("Path = %q", record.Path)
	}
}
