package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
)

const (
	templateDrift = "actionsqaci.templateDrift"
	lintCoverage  = "actionsqaci.lintCoverage"
	ciPath        = ".github/workflows/ci.yml"
)

func at(path string, line, col int, id, message string) finding.Finding {
	return finding.Finding{
		ID:       id,
		Tool:     "actions-qa-ci",
		Path:     path,
		Line:     line,
		Col:      col,
		EndLine:  line,
		Severity: finding.SeverityError,
		Message:  message,
		DocPath:  "docs/rules/" + id + ".md",
	}
}

func render(t *testing.T, format output.Format, findings []finding.Finding) string {
	t.Helper()

	var buffer bytes.Buffer
	if err := output.Write(&buffer, format, findings); err != nil {
		t.Fatalf("%s: %v", format, err)
	}

	return buffer.String()
}

func TestParseFormatNamesTheFormatsItKnows(t *testing.T) {
	for _, name := range []string{"text", "agent", "json"} {
		format, err := output.ParseFormat(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if string(format) != name {
			t.Errorf("%s parsed as %q", name, format)
		}
	}
}

func TestParseFormatRefusesAnUnknownFormatAndListsTheKnownOnes(t *testing.T) {
	_, err := output.ParseFormat("xml")
	if err == nil {
		t.Fatal("xml was accepted")
	}
	for _, want := range []string{"xml", "text", "agent", "json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q: %v", want, err)
		}
	}
}

func TestAgentFormatIsOneLinePerFindingThenTheDocCommandsThenACount(t *testing.T) {
	findings := []finding.Finding{
		at(ciPath, 12, 7, templateDrift, "workflow drifted from its template"),
		at(ciPath, 20, 3, templateDrift, "workflow drifted from its template"),
		at("action.yml", 4, 1, lintCoverage, "workflow is not linted"),
	}

	got := render(t, output.Agent, findings)

	want := strings.Join([]string{
		".github/workflows/ci.yml:12:7 actionsqaci.templateDrift workflow drifted from its template",
		".github/workflows/ci.yml:20:3 actionsqaci.templateDrift workflow drifted from its template",
		"action.yml:4:1 actionsqaci.lintCoverage workflow is not linted",
		"rule-doc actionsqaci.lintCoverage",
		"rule-doc actionsqaci.templateDrift",
		"3 findings",
		"",
	}, "\n")
	if got != want {
		t.Errorf("agent output:\n%s\nwant:\n%s", got, want)
	}
}

func TestAgentFormatSaysSoWhenThereAreNoFindings(t *testing.T) {
	if got, want := render(t, output.Agent, nil), "0 findings\n"; got != want {
		t.Errorf("agent output = %q, want %q", got, want)
	}
}

func TestAgentFormatCountsASingleFindingInTheSingular(t *testing.T) {
	got := render(t, output.Agent, []finding.Finding{at(ciPath, 1, 1, templateDrift, "drifted")})

	if !strings.HasSuffix(got, "1 finding\n") {
		t.Errorf("agent output should end \"1 finding\":\n%s", got)
	}
}

func TestAgentFormatKeepsEachFindingOnOneLine(t *testing.T) {
	got := render(t, output.Agent, []finding.Finding{at(ciPath, 1, 1, templateDrift, "first line\nsecond line\r\nthird")})

	if !strings.Contains(got, ciPath+":1:1 "+templateDrift+" first line second line third\n") {
		t.Errorf("a multi-line message should be joined onto one line:\n%s", got)
	}
}

func TestAnUnknownColumnIsReportedAsTheStartOfTheLine(t *testing.T) {
	got := render(t, output.Agent, []finding.Finding{at(ciPath, 9, 0, templateDrift, "drifted")})

	if !strings.HasPrefix(got, ciPath+":9:1 ") {
		t.Errorf("column 0 should print as 1:\n%s", got)
	}
}

func TestExceptedFindingsAreCountedButNotListed(t *testing.T) {
	excepted := at(ciPath, 5, 1, lintCoverage, "recorded exception")
	excepted.Excepted = true
	excepted.ExceptionKey = "ci.yml#lintCoverage"

	findings := []finding.Finding{at(ciPath, 12, 7, templateDrift, "drifted"), excepted}

	for _, format := range []output.Format{output.Agent, output.Text} {
		got := render(t, format, findings)

		if strings.Contains(got, "recorded exception") || strings.Contains(got, "rule-doc "+lintCoverage) {
			t.Errorf("%s lists an excepted finding:\n%s", format, got)
		}
		if !strings.Contains(got, "1 finding (1 excepted)") {
			t.Errorf("%s should say one finding and one excepted:\n%s", format, got)
		}
	}
}

func TestTextFormatGroupsFindingsByFile(t *testing.T) {
	findings := []finding.Finding{
		at(ciPath, 12, 7, templateDrift, "workflow drifted"),
		at(ciPath, 20, 3, lintCoverage, "not linted"),
		at("action.yml", 4, 1, lintCoverage, "not linted"),
	}

	got := render(t, output.Text, findings)

	want := strings.Join([]string{
		".github/workflows/ci.yml",
		"  12:7  error  workflow drifted  actionsqaci.templateDrift",
		"  20:3  error  not linted  actionsqaci.lintCoverage",
		"",
		"action.yml",
		"  4:1  error  not linted  actionsqaci.lintCoverage",
		"",
		"rule-doc actionsqaci.lintCoverage",
		"rule-doc actionsqaci.templateDrift",
		"3 findings",
		"",
	}, "\n")
	if got != want {
		t.Errorf("text output:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextAndAgentOutputDoNotDependOnTheOrderFindingsArrivedIn(t *testing.T) {
	a := at(ciPath, 12, 7, templateDrift, "drifted")
	b := at("action.yml", 4, 1, lintCoverage, "not linted")

	for _, format := range []output.Format{output.Text, output.Agent, output.JSON} {
		forward := render(t, format, []finding.Finding{a, b})
		backward := render(t, format, []finding.Finding{b, a})

		if forward != backward {
			t.Errorf("%s output depends on input order:\n%s\n--\n%s", format, forward, backward)
		}
	}
}

func TestJSONFormatPrintsEveryFindingIncludingExceptedOnes(t *testing.T) {
	excepted := at(ciPath, 5, 1, lintCoverage, "recorded exception")
	excepted.Excepted = true
	excepted.ExceptionKey = "ci.yml#lintCoverage"

	got := render(t, output.JSON, []finding.Finding{at(ciPath, 12, 7, templateDrift, "drifted"), excepted})

	var decoded []finding.Finding
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("json output is not a list of findings: %v\n%s", err, got)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d findings, want 2", len(decoded))
	}
	if !decoded[0].Excepted && !decoded[1].Excepted {
		t.Error("the excepted finding lost its flag")
	}
}

func TestJSONFormatPrintsAnEmptyListNotNull(t *testing.T) {
	if got := strings.TrimSpace(render(t, output.JSON, nil)); got != "[]" {
		t.Errorf("json output for no findings = %q, want []", got)
	}
}

func TestWriteRefusesAFindingThatCannotBeRead(t *testing.T) {
	broken := at(ciPath, 0, 1, templateDrift, "no line")

	for _, format := range []output.Format{output.Text, output.Agent, output.JSON} {
		var buffer bytes.Buffer

		if err := output.Write(&buffer, format, []finding.Finding{broken}); err == nil {
			t.Errorf("%s: an invalid finding was printed", format)
		}
		if buffer.Len() != 0 {
			t.Errorf("%s: wrote %q before refusing", format, buffer.String())
		}
	}
}

func TestWriteRefusesAFormatItCannotWriteAndPrintsNothing(t *testing.T) {
	var buffer bytes.Buffer

	err := output.Write(&buffer, output.Format("sarif"), []finding.Finding{at(ciPath, 1, 1, templateDrift, "drifted")})

	if err == nil {
		t.Fatal("an unknown format was written")
	}
	if buffer.Len() != 0 {
		t.Errorf("wrote %q before refusing", buffer.String())
	}
}

func TestWriteReportsAWriterThatFails(t *testing.T) {
	err := output.Write(failingWriter{}, output.Agent, []finding.Finding{at(ciPath, 1, 1, templateDrift, "drifted")})

	if err == nil {
		t.Fatal("a failed write was reported as success")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriterBroken
}

var errWriterBroken = errors.New("the writer is broken")
