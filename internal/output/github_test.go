package output_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
)

func annotations(text string) []string {
	var found []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "::") {
			found = append(found, line)
		}
	}

	return found
}

func TestParseFormatKnowsTheGitHubFormat(t *testing.T) {
	format, err := output.ParseFormat("github")

	if err != nil || format != output.GitHub {
		t.Errorf("github parsed as %q, %v", format, err)
	}
}

func TestAFindingIsOneAnnotationAtItsPositionThenTheFullListInTheLog(t *testing.T) {
	f := at(ciPath, 3, 5, templateDrift, "workflow drifted")

	got := render(t, output.GitHub, []finding.Finding{f})

	want := "::error file=.github/workflows/ci.yml,line=3,col=5,endLine=3,title=actionsqaci.templateDrift::workflow drifted\n" +
		".github/workflows/ci.yml:3:5 actionsqaci.templateDrift workflow drifted\n" +
		"rule-doc actionsqaci.templateDrift\n" +
		"1 finding\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEachSeverityIsTheAnnotationLevelGitHubKnows(t *testing.T) {
	cases := map[finding.Severity]string{
		finding.SeverityError:   "::error ",
		finding.SeverityWarning: "::warning ",
		finding.SeverityNote:    "::notice ",
	}

	for severity, prefix := range cases {
		f := at(ciPath, 1, 1, templateDrift, "m")
		f.Severity = severity

		got := annotations(render(t, output.GitHub, []finding.Finding{f}))

		if len(got) != 1 || !strings.HasPrefix(got[0], prefix) {
			t.Errorf("%s: annotations %q; want one starting %q", severity, got, prefix)
		}
	}
}

func TestAnAnnotationEscapesWhatTheRunnerReadsAsSyntax(t *testing.T) {
	f := at("dir,a:b/ci.yml", 2, 0, templateDrift, "100% sure\r\nnext: line, here")

	got := annotations(render(t, output.GitHub, []finding.Finding{f}))

	want := "::error file=dir%2Ca%3Ab/ci.yml,line=2,col=1,endLine=2,title=actionsqaci.templateDrift::100%25 sure next: line, here"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func manyFindings(severity finding.Severity, n int) []finding.Finding {
	var list []finding.Finding
	for i := 1; i <= n; i++ {
		f := at(ciPath, i, 1, templateDrift, fmt.Sprintf("problem %d", i))
		f.Severity = severity
		list = append(list, f)
	}

	return list
}

func TestAnnotationsAreCappedPerLevelAndTheFullListStaysInTheLog(t *testing.T) {
	findings := append(manyFindings(finding.SeverityError, 12), manyFindings(finding.SeverityWarning, 3)...)
	for i := range findings[12:] {
		findings[12+i].Path = "other.yml"
	}

	got := render(t, output.GitHub, findings)

	var errorsAnnotated, warningsAnnotated int
	for _, line := range annotations(got) {
		switch {
		case strings.HasPrefix(line, "::error "):
			errorsAnnotated++
		case strings.HasPrefix(line, "::warning "):
			warningsAnnotated++
		}
	}
	if errorsAnnotated != 10 || warningsAnnotated != 3 {
		t.Errorf("annotated %d errors and %d warnings; want 10 and 3", errorsAnnotated, warningsAnnotated)
	}
	for i := 1; i <= 12; i++ {
		if !strings.Contains(got, fmt.Sprintf("\n%s:%d:1 %s problem %d\n", ciPath, i, templateDrift, i)) {
			t.Errorf("problem %d is not in the plain list:\n%s", i, got)
		}
	}
	if !strings.Contains(got, "2 error annotations were left out") || !strings.HasSuffix(got, "15 findings\n") {
		t.Errorf("the omission is not said, or the count is wrong:\n%s", got)
	}
}

func TestTheOmissionLineAgreesWithTheNumberLeftOut(t *testing.T) {
	findings := append(manyFindings(finding.SeverityError, 12), manyFindings(finding.SeverityWarning, 11)...)
	for i := range findings[12:] {
		findings[12+i].Path = "other.yml"
	}

	got := render(t, output.GitHub, findings)

	if !strings.Contains(got, "2 error annotations were left out and 1 warning annotation was left out") {
		t.Errorf("the omission line is not worded for its counts:\n%s", got)
	}
}

func TestAnExceptedFindingIsCountedAndNotAnnotated(t *testing.T) {
	active := at(ciPath, 1, 1, templateDrift, "active")
	accepted := at(ciPath, 2, 1, templateDrift, "accepted")
	accepted.Excepted, accepted.ExceptionKey = true, "k"

	got := render(t, output.GitHub, []finding.Finding{active, accepted})

	if len(annotations(got)) != 1 || strings.Contains(got, "accepted") || !strings.HasSuffix(got, "1 finding (1 excepted)\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestAFileNamedLikeAWorkflowCommandIsNeverACommandInAnyFormat(t *testing.T) {
	for _, name := range []string{"::stop-commands::token", "##[add-mask]secret", "::error::forged"} {
		f := at(name, 1, 1, templateDrift, "m")

		for _, format := range []output.Format{output.Text, output.Agent, output.GitHub} {
			got := render(t, format, []finding.Finding{f})

			for _, line := range strings.Split(got, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "##[") {
					t.Errorf("%s: path %q printed a line the runner reads as a command: %q", format, name, line)
				}
				if strings.HasPrefix(strings.TrimSpace(line), "::") && !strings.HasPrefix(line, "::error file=") {
					t.Errorf("%s: path %q printed a line the runner reads as a command: %q", format, name, line)
				}
			}
		}
	}
}

func TestACleanRunPrintsOnlyTheCount(t *testing.T) {
	if got := render(t, output.GitHub, nil); got != "0 findings\n" {
		t.Errorf("got %q", got)
	}
}
