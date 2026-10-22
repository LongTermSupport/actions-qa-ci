// Package output prints findings. Every format prints the identifier unaltered and sorts its
// input, so the same findings print the same text whatever order the lanes reported them in.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Format names a way of printing findings.
type Format string

// The formats Write understands.
const (
	// Text is grouped by file, for a person at a terminal.
	Text Format = "text"
	// Agent is one line per finding, for a reader that parses lines rather than layout.
	Agent Format = "agent"
	// JSON is the finding schema, every finding including the excepted ones.
	JSON Format = "json"
)

var formats = []Format{Text, Agent, JSON}

// ParseFormat turns a name into a Format, or says which names exist.
func ParseFormat(name string) (Format, error) {
	for _, format := range formats {
		if string(format) == name {
			return format, nil
		}
	}

	return "", fmt.Errorf("unknown format %q; the formats are %s", name, knownFormats())
}

func knownFormats() string {
	known := make([]string, 0, len(formats))
	for _, format := range formats {
		known = append(known, string(format))
	}

	return strings.Join(known, ", ")
}

// Write prints the findings in the format. It validates every finding and renders the whole
// output before writing any of it, so an invalid finding or a bad format prints nothing.
func Write(w io.Writer, format Format, findings []finding.Finding) error {
	for _, f := range findings {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("refusing to print an invalid finding: %w", err)
		}
	}

	sorted := slices.Clone(findings)
	finding.Sort(sorted)

	var rendered bytes.Buffer
	var err error
	switch format {
	case Text:
		err = renderText(&rendered, sorted)
	case Agent:
		err = renderAgent(&rendered, sorted)
	case JSON:
		err = renderJSON(&rendered, sorted)
	default:
		err = fmt.Errorf("cannot write format %q; the formats are %s", format, knownFormats())
	}
	if err != nil {
		return err
	}

	if _, err := w.Write(rendered.Bytes()); err != nil {
		return fmt.Errorf("writing %s output: %w", format, err)
	}

	return nil
}

func renderJSON(out *bytes.Buffer, findings []finding.Finding) error {
	if findings == nil {
		findings = []finding.Finding{}
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	return encoder.Encode(findings)
}

// split separates the findings to list from the excepted ones, which are only counted.
func split(findings []finding.Finding) (active []finding.Finding, excepted int) {
	for _, f := range findings {
		if f.Excepted {
			excepted++

			continue
		}
		active = append(active, f)
	}

	return active, excepted
}

func renderAgent(out *bytes.Buffer, findings []finding.Finding) error {
	active, excepted := split(findings)

	for _, f := range active {
		fmt.Fprintf(out, "%s:%d:%d %s %s\n", f.Path, f.Line, column(f), f.ID, oneLine(f.Message))
	}
	writeTrailer(out, active, excepted)

	return nil
}

func renderText(out *bytes.Buffer, findings []finding.Finding) error {
	active, excepted := split(findings)

	previous := ""
	for index, f := range active {
		if index == 0 || f.Path != previous {
			if index > 0 {
				out.WriteString("\n")
			}
			out.WriteString(f.Path + "\n")
			previous = f.Path
		}
		fmt.Fprintf(out, "  %d:%d  %s  %s  %s\n", f.Line, column(f), f.Severity, oneLine(f.Message), f.ID)
	}
	if len(active) > 0 {
		out.WriteString("\n")
	}
	writeTrailer(out, active, excepted)

	return nil
}

// writeTrailer prints one rule-doc line per distinct identifier, then the count.
func writeTrailer(out *bytes.Buffer, active []finding.Finding, excepted int) {
	seen := map[string]bool{}
	var ids []string
	for _, f := range active {
		if !seen[f.ID] {
			seen[f.ID] = true
			ids = append(ids, f.ID)
		}
	}
	slices.Sort(ids)

	for _, id := range ids {
		out.WriteString("rule-doc " + id + "\n")
	}

	noun := "findings"
	if len(active) == 1 {
		noun = "finding"
	}
	out.WriteString(strconv.Itoa(len(active)) + " " + noun)
	if excepted > 0 {
		out.WriteString(" (" + strconv.Itoa(excepted) + " excepted)")
	}
	out.WriteString("\n")
}

// column is 1-based; a lane that did not know the column reports the start of the line.
func column(f finding.Finding) int {
	return max(f.Col, 1)
}

var lineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

func oneLine(message string) string {
	return lineBreaks.Replace(message)
}
