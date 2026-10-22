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
	cases := []struct {
		name    string
		mutate  func(*finding.Finding)
		message string
	}{
		{"malformed identifier", func(f *finding.Finding) { f.ID = "template-drift" }, "not of a form"},
		{"no path", func(f *finding.Finding) { f.Path = "" }, "no path"},
		{"blank path", func(f *finding.Finding) { f.Path = "  " }, "no path"},
		{"absolute path", func(f *finding.Finding) { f.Path = "/etc/ci.yml" }, "repository-relative"},
		{"path leaving the repository", func(f *finding.Finding) { f.Path = "../ci.yml" }, "repository-relative"},
		{"no message", func(f *finding.Finding) { f.Message = "" }, "no message"},
		{"blank message", func(f *finding.Finding) { f.Message = "\n" }, "no message"},
		{"unknown severity", func(f *finding.Finding) { f.Severity = "fatal" }, "severity"},
		{"line before the file", func(f *finding.Finding) { f.Line = 0 }, "lines start at 1"},
		{"negative column", func(f *finding.Finding) { f.Col = -1 }, "column"},
		{"end before start", func(f *finding.Finding) { f.EndLine = 3 }, "before it starts"},
		{"no tool", func(f *finding.Finding) { f.Tool = "" }, "no tool"},
		{"blank tool", func(f *finding.Finding) { f.Tool = " " }, "no tool"},
		{"excepted without key", func(f *finding.Finding) { f.Excepted = true }, "names no exception"},
		{"key without exception", func(f *finding.Finding) { f.ExceptionKey = "ci.yml#templateDrift" }, "is not excepted"},
	}

	for _, c := range cases {
		f := sample()
		c.mutate(&f)

		err := f.Validate()
		if err == nil {
			t.Errorf("%s: the finding was accepted", c.name)

			continue
		}
		if !strings.Contains(err.Error(), c.message) {
			t.Errorf("%s: refused for the wrong reason: %v (want it to mention %q)", c.name, err, c.message)
		}
	}
}

func TestFindingsThatTieOnPositionAreOrderedByTheRestOfTheirContent(t *testing.T) {
	first, second := sample(), sample()
	first.Message = "alpha"
	second.Message = "beta"

	forward := []finding.Finding{first, second}
	backward := []finding.Finding{second, first}
	finding.Sort(forward)
	finding.Sort(backward)

	for i := range forward {
		if forward[i].Message != backward[i].Message {
			t.Fatalf("position %d is %q from one input order and %q from the other", i, forward[i].Message, backward[i].Message)
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
