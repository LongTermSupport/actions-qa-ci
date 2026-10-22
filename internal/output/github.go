package output

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// annotationLimit is how many annotations of one level a step shows. GitHub's documentation
// does not state the figure; ten is the number it is reported to show, and a lower limit than
// the real one loses nothing, because every finding is also in the plain list.
const annotationLimit = 10

// annotationLevels are the levels of workflow annotation and the severities that map to them,
// in the order they are printed.
var annotationLevels = []struct {
	level    string
	severity finding.Severity
}{
	{"error", finding.SeverityError},
	{"warning", finding.SeverityWarning},
	{"notice", finding.SeverityNote},
}

// renderGitHub prints the annotations, then says how many a level had to leave out, then the
// full list in the agent format, which the log keeps whatever GitHub shows.
func renderGitHub(out *bytes.Buffer, findings []finding.Finding) error {
	active, excepted := split(findings)

	var omitted []string
	for _, level := range annotationLevels {
		shown := 0
		left := 0
		for _, f := range active {
			if f.Severity != level.severity {
				continue
			}
			if shown == annotationLimit {
				left++

				continue
			}
			shown++
			out.WriteString(annotation(level.level, f))
		}
		if left > 0 {
			omitted = append(omitted, fmt.Sprintf("%d %s annotations were left out", left, level.level))
		}
	}
	if len(omitted) > 0 {
		fmt.Fprintf(out, "actions-qa: %s because GitHub shows a limited number per step; every finding is listed below\n", strings.Join(omitted, " and "))
	}

	for _, f := range active {
		fmt.Fprintf(out, "%s:%d:%d %s %s\n", displayPath(f.Path), f.Line, column(f), f.ID, oneLine(f.Message))
	}
	writeTrailer(out, active, excepted)

	return nil
}

// annotation is the workflow command that shows one finding on its line. The message goes
// through oneLine first, so it holds nothing the runner or a terminal would act on.
func annotation(level string, f finding.Finding) string {
	properties := fmt.Sprintf("file=%s,line=%d,col=%d", escapeProperty(f.Path), f.Line, column(f))
	if f.EndLine > 0 {
		properties += fmt.Sprintf(",endLine=%d", f.EndLine)
	}
	properties += ",title=" + escapeProperty(f.ID)

	return "::" + level + " " + properties + "::" + escapeData(oneLine(f.Message)) + "\n"
}

var (
	dataEscape     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	propertyEscape = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// escapeData escapes the text after the second `::` of a workflow command, as the runner reads it.
func escapeData(text string) string { return dataEscape.Replace(text) }

// escapeProperty escapes a value among a command's properties, where the comma and the colon
// are syntax as well.
func escapeProperty(text string) string { return propertyEscape.Replace(text) }
