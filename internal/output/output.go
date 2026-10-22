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
	"unicode/utf16"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Format names a way of printing findings.
type Format string

// The formats Write understands.
const (
	// Text is grouped by file, for a person at a terminal.
	Text Format = "text"
	// Agent is one line per finding, for a reader that parses lines rather than layout. A path
	// that is plain prints as it is; any other path prints as a double-quoted Go string, so a
	// file name can never pass for a trailer line or split a finding line.
	Agent Format = "agent"
	// JSON is the finding schema, every finding including the excepted ones.
	JSON Format = "json"
	// GitHub is for the log of a workflow step: workflow annotations, up to GitHub's limit for
	// each level, then the full list as the agent format prints it.
	GitHub Format = "github"
)

var formats = []Format{Text, Agent, JSON, GitHub}

// ParseFormat turns a name into a Format, or says which names exist.
func ParseFormat(name string) (Format, error) {
	for _, format := range formats {
		if string(format) == name {
			return format, nil
		}
	}

	return "", fmt.Errorf("unknown format %q; the formats are %s", name, knownFormats())
}

// FormatNames lists the names of the formats, in the order they are documented.
func FormatNames() []string {
	names := make([]string, 0, len(formats))
	for _, format := range formats {
		names = append(names, string(format))
	}

	return names
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
	case GitHub:
		err = renderGitHub(&rendered, sorted)
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

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(findings); err != nil {
		return err
	}

	out.WriteString(escapeUnprintable(encoded.String()))

	return nil
}

// escapeUnprintable writes as \u escapes the characters of encoded JSON that a terminal
// would act on and the encoder leaves as they are: delete, the C1 controls, direction
// overrides and other format characters. Outside a string the encoder writes only ASCII, so
// every such character is inside one, where the escape reads back as the same character.
func escapeUnprintable(encoded string) string {
	var escaped strings.Builder
	for _, r := range encoded {
		if r < 0x7f || !finding.IsControlOrSeparator(r) {
			escaped.WriteRune(r)

			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&escaped, `\u%04x`, unit)
		}
	}

	return escaped.String()
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
		fmt.Fprintf(out, "%s:%d:%d %s %s\n", displayPath(f.Path), f.Line, column(f), f.ID, oneLine(f.Message))
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
			out.WriteString(displayPath(f.Path) + "\n")
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

func displayPath(path string) string { return finding.DisplayPath(path) }

var crlf = strings.NewReplacer("\r\n", "\n")

// oneLine makes a message safe to print on a single line of a line-oriented format: every
// control character and every Unicode line or paragraph separator becomes a space, so a
// message cannot start another line, forge a trailer or move the terminal cursor.
func oneLine(message string) string {
	return strings.Map(func(r rune) rune {
		if finding.IsControlOrSeparator(r) {
			return ' '
		}

		return r
	}, crlf.Replace(message))
}
