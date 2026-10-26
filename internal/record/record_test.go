package record_test

import (
	"errors"
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
		"empty":                     {"", "version"},
		"no version":                {"exceptions: []\n", "version"},
		"blank version":             {"version: ''\n", "version"},
		"not yaml":                  {"version: [\n", "not valid"},
		"a list at the top":         {"- version\n", "not valid"},
		"unknown top field":         {"version: 1\nlabels: {}\n", "labels"},
		"unknown entry field":       {exception("    reason: because\n"), "reason"},
		"duplicate key":             {"version: 1\nversion: 2\n", "already defined"},
		"second document":           {"version: 1\n---\nversion: 2\n", "more than one document"},
		"no id":                     {"version: 1\nexceptions:\n  - path: a.yml\n", "id is missing"},
		"unknown id":                {"version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n    hazard: a\n    scope: b\n    decision: c\n", "unknown"},
		"id not in the scheme":      {"version: 1\nexceptions:\n  - id: Not An Identifier\n", "scheme"},
		"absolute path":             {exception("    path: /etc/passwd\n"), "path"},
		"parent path":               {exception("    path: ../x.yml\n"), "path"},
		"backslash path":            {exception("    path: a\\b.yml\n"), "path"},
		"glob path":                 {exception("    path: .github/workflows/*.yml\n"), "path"},
		"job without a path":        {exception("    job: release\n"), "path"},
		"step without a path":       {exception("    step: build\n"), "path"},
		"scalar exceptions":         {"version: 1\nexceptions: yes\n", "not valid"},
		"a number for an id":        {"version: 1\nexceptions:\n  - id: 7\n", "scheme"},
		"an empty mapping":          {"version: 1\nexceptions:\n  - {}\n", "id is missing"},
		"an empty id":               {"version: 1\nexceptions:\n  - id: \"\"\n", "id is missing"},
		"a version by alias":        {"exceptions:\n  - id: actionsqaci.actionlint.glob\n    hazard: &h true\nversion: *h\n", "anchor"},
		"an alias as a key":         {"exceptions:\n  - id: actionsqaci.actionlint.glob\n    job: &k version\n    path: .github/workflows/a.yml\n*k : true\n", "alias"},
		"an anchor":                 {"version: &v 1\n", "anchor"},
		"an explicit tag":           {"version: !!str 1\n", "tag"},
		"a custom tag":              {"version: !custom 1\n", "tag"},
		"a tag on an entry":         {"version: 1\nexceptions:\n  - !!map {id: actionsqaci.actionlint.glob}\n", "tag"},
		"a null key":                {"~: a\nversion: 1\n", "key"},
		"a number key":              {"1: a\nversion: 1\n", "key"},
		"a key in an entry":         {"version: 1\nexceptions:\n  - 5: x\n    id: actionsqaci.actionlint.glob\n", "key"},
		"a hexadecimal version":     {"version: 0x1\n", "version"},
		"an octal version":          {"version: 0o7\n", "version"},
		"an underscored version":    {"version: 1_0\n", "version"},
		"a merge key at the top":    {"<<: {version: true}\nexceptions: []\n", "merge"},
		"a merge key supplying all": {"<<: {version: 1}\n", "merge"},
		"a bare dash":               {"version: 1\nexceptions:\n  -\n", "empty"},
		"a null entry":              {"version: 1\nexceptions:\n  - null\n  - ~\n", "empty"},
		"a boolean version":         {"version: true\n", "version"},
		"a float version":           {"version: 1.0\n", "version"},
		"a list version":            {"version: [1]\n", "not valid"},
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

func TestAnInvalidRecordIsMarkedInvalidAndAnUnreadableFileIsNot(t *testing.T) {
	_, err := record.Parse([]byte("version: 1\nsurprise: 1\n"), everythingIsKnown)
	if !errors.Is(err, record.ErrInvalid) {
		t.Errorf("a record with an unknown field must be marked invalid, got %v", err)
	}
	_, err = record.Parse([]byte("version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n"), func(string) bool { return false })
	if !errors.Is(err, record.ErrInvalid) {
		t.Errorf("a record naming an unknown identifier must be marked invalid, got %v", err)
	}

	_, err = record.Load(filepath.Join(t.TempDir(), "absent.yml"), everythingIsKnown)
	if err == nil || errors.Is(err, record.ErrInvalid) {
		t.Errorf("a file that cannot be read is not an invalid record, got %v", err)
	}
}

func TestEachEntryKeepsItsOwnLineWhateverFormItIsWrittenIn(t *testing.T) {
	data := "version: 1\n" + // 1
		"exceptions:\n" + // 2
		"  # a comment\n" + // 3
		"  - id: actionsqaci.actionlint.glob\n" + // 4
		"    hazard: a\n" + // 5
		"\n" + // 6
		"  - {id: actionsqaci.actionlint.glob, hazard: b}\n" + // 7
		"  - id: actionsqaci.actionlint.glob\n" // 8

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	var lines []int
	for _, e := range rec.Exceptions {
		lines = append(lines, e.Line)
	}
	if len(lines) != 3 || lines[0] != 4 || lines[1] != 7 || lines[2] != 8 {
		t.Errorf("lines = %v, want [4 7 8]", lines)
	}
}

func TestAnEmptyEntryIsReportedAtItsOwnLineAndShiftsNothingAfterIt(t *testing.T) {
	data := "version: 1\nexceptions:\n  -\n  - id: actionsqaci.nonsense\n"

	_, err := record.Parse([]byte(data), func(string) bool { return false })
	if err == nil {
		t.Fatal("accepted")
	}

	for _, want := range []string{"exception 1 (line 3) is empty", "exception 2 (line 4): id \"actionsqaci.nonsense\" is unknown"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error lacks %q: %q", want, err.Error())
		}
	}
}
