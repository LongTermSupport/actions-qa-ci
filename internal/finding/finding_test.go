package finding_test

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const templateDriftID = "actionsqaci.templateDrift"

func sample() finding.Finding {
	return finding.Finding{
		ID:         templateDriftID,
		Tool:       "actions-qa-ci",
		UpstreamID: "",
		Path:       ".github/workflows/ci.yml",
		Line:       12,
		Col:        7,
		EndLine:    13,
		Severity:   finding.SeverityError,
		Message:    "workflow drifted from its template",
		DocPath:    "docs/rules/actionsqaci.templateDrift.md",
	}
}

func TestAFindingSerialisesToTheDocumentedSchema(t *testing.T) {
	encoded, err := json.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(fields))
	for key := range fields {
		got = append(got, key)
	}
	sort.Strings(got)

	want := []string{"col", "docPath", "endLine", "excepted", "exceptionKey", "id", "line", "message", "path", "severity", "tool", "upstreamId"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("schema keys = %v, want %v", got, want)
	}
}

func TestAValidFindingValidates(t *testing.T) {
	if err := sample().Validate(); err != nil {
		t.Errorf("a well-formed finding was refused: %v", err)
	}
}

func TestAFindingThatCannotBeActedOnIsRefused(t *testing.T) {
	cases := map[string]func(*finding.Finding){
		"malformed identifier": func(f *finding.Finding) { f.ID = "template-drift" },
		"no path":              func(f *finding.Finding) { f.Path = "" },
		"no message":           func(f *finding.Finding) { f.Message = "" },
		"unknown severity":     func(f *finding.Finding) { f.Severity = "fatal" },
		"line before the file": func(f *finding.Finding) { f.Line = 0 },
		"end before start":     func(f *finding.Finding) { f.EndLine = 3 },
		"no tool":              func(f *finding.Finding) { f.Tool = "" },
		"excepted without key": func(f *finding.Finding) { f.Excepted = true },
		"key without exception": func(f *finding.Finding) {
			f.ExceptionKey = "ci.yml#templateDrift"
		},
	}

	for name, mutate := range cases {
		f := sample()
		mutate(&f)

		if err := f.Validate(); err == nil {
			t.Errorf("%s: the finding was accepted", name)
		}
	}
}

func TestAnExceptedFindingNamesItsException(t *testing.T) {
	f := sample()
	f.Excepted = true
	f.ExceptionKey = "ci.yml#templateDrift"

	if err := f.Validate(); err != nil {
		t.Errorf("an excepted finding with its key was refused: %v", err)
	}
}

func TestFindingsAreOrderedByPathThenPositionThenIdentifier(t *testing.T) {
	at := func(path string, line int, id string) finding.Finding {
		f := sample()
		f.Path, f.Line, f.EndLine, f.ID = path, line, line, id

		return f
	}

	findings := []finding.Finding{
		at("b.yml", 1, templateDriftID),
		at("a.yml", 9, templateDriftID),
		at("a.yml", 2, "actionsqaci.unjustifiedException"),
		at("a.yml", 2, templateDriftID),
	}
	finding.Sort(findings)

	got := make([]string, 0, len(findings))
	for _, f := range findings {
		got = append(got, f.Path+":"+strconv.Itoa(f.Line)+":"+f.ID)
	}
	want := []string{
		"a.yml:2:" + templateDriftID,
		"a.yml:2:actionsqaci.unjustifiedException",
		"a.yml:9:" + templateDriftID,
		"b.yml:1:" + templateDriftID,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("order = %v, want %v", got, want)
	}
}
