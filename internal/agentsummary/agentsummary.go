// Package agentsummary writes the active defences as a delimited, generated region of the
// markdown file an agent reads (CLAUDE.md, AGENTS.md), and reads that region back so it can be
// compared with what it should say. Everything outside the markers is the project's own and is
// kept byte for byte.
package agentsummary

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
)

const (
	// Begin and End each stand alone on a line and delimit the generated region.
	Begin = "<!-- actions-qa-ci:begin -->"
	End   = "<!-- actions-qa-ci:end -->"
)

const header = "## actions-qa-ci — Active defences\n\n" +
	"Generated from this project's active configuration by `actions-qa rules --write-agent-summary`; do not\n" +
	"edit. Each line is a standing rule `actions-qa check` enforces; `actions-qa rule-doc <identifier>` prints\n" +
	"its page offline.\n\n"

// Region renders the entries as the generated region, markers included and no final newline.
// Text that could end the region early, start another line or open a comment is refused.
func Region(entries []listing.Entry) (string, error) {
	var lines []string
	for _, entry := range entries {
		if err := plain(entry.ID, "identifier", true); err != nil {
			return "", err
		}
		if err := plain(entry.Summary, "summary of "+entry.ID, false); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("- `%s` — %s", entry.ID, entry.Summary))

		for _, narrowing := range entry.NarrowedBy {
			if err := plain(narrowing, "narrowing of "+entry.ID, false); err != nil {
				return "", err
			}
			lines = append(lines, "  - narrowed: "+narrowing)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "No defence is active.")
	}

	return Begin + "\n\n" + header + strings.Join(lines, "\n") + "\n\n" + End, nil
}

// plain refuses text that is not one plain line of a markdown list.
func plain(text, what string, code bool) error {
	switch {
	case strings.ContainsFunc(text, finding.IsControlOrSeparator):
		return fmt.Errorf("the %s %q has a control character or a line break", what, finding.Escape(text))
	case strings.Contains(text, "<!--") || strings.Contains(text, "-->"):
		return fmt.Errorf("the %s %q holds an HTML comment marker", what, finding.Escape(text))
	case code && strings.Contains(text, "`"):
		return fmt.Errorf("the %s %q holds a backtick", what, finding.Escape(text))
	}

	return nil
}

// locate finds the generated region in data: where it starts and where its end marker ends. It
// is an error for the markers to be anything but one begin line and, after it, one end line.
func locate(data []byte) (start, end int, found bool, err error) {
	begins, ends := 0, 0
	beginAt, endAt := -1, -1
	for offset := 0; offset <= len(data); {
		next := bytes.IndexByte(data[offset:], '\n')
		lineEnd := len(data)
		if next >= 0 {
			lineEnd = offset + next
		}

		text := data[offset:lineEnd]
		carriage := 0
		if bytes.HasSuffix(text, []byte("\r")) {
			text, carriage = text[:len(text)-1], 1
		}

		switch string(text) {
		case Begin:
			begins++
			beginAt = offset
		case End:
			ends++
			endAt = lineEnd - carriage
		}

		if next < 0 {
			break
		}
		offset = lineEnd + 1
	}

	switch {
	case begins == 0 && ends == 0:
		return 0, 0, false, nil
	case begins != 1 || ends != 1:
		return 0, 0, false, fmt.Errorf("the file has %d lines %s and %d lines %s; it needs exactly one of each, or neither", begins, Begin, ends, End)
	case endAt < beginAt:
		return 0, 0, false, errors.New("the end marker of the generated region comes before its begin marker")
	}

	return beginAt, endAt, true, nil
}

// Extract returns the generated region of data, markers included and with plain line endings
// whatever the file uses, and whether there is one.
func Extract(data []byte) (string, bool, error) {
	start, end, found, err := locate(data)
	if err != nil || !found {
		return "", false, err
	}

	return strings.ReplaceAll(string(data[start:end]), "\r\n", "\n"), true, nil
}

// BeginLine is the line, counting from one, on which the generated region of data begins.
func BeginLine(data []byte) (int, bool, error) {
	start, _, found, err := locate(data)
	if err != nil || !found {
		return 0, false, err
	}

	return bytes.Count(data[:start], []byte("\n")) + 1, true, nil
}

// Splice returns data with the region put in: in place of the one already there, or else after
// everything else. A file with markers that do not form one region is refused, not rewritten.
// A file whose lines end in CRLF gets the region with the same line endings.
func Splice(data []byte, region string) ([]byte, error) {
	start, end, found, err := locate(data)
	if err != nil {
		return nil, err
	}

	newline := "\n"
	if endsLinesWithCRLF(data) {
		newline = "\r\n"
		region = strings.ReplaceAll(region, "\n", newline)
	}

	if found {
		return bytes.Join([][]byte{data[:start], []byte(region), data[end:]}, nil), nil
	}

	base := data
	for bytes.HasSuffix(base, []byte(newline)) {
		base = base[:len(base)-len(newline)]
	}
	if len(base) == 0 {
		return []byte(region + newline), nil
	}

	return bytes.Join([][]byte{base, []byte(newline + newline + region + newline)}, nil), nil
}

// endsLinesWithCRLF says whether the first line of data ends in CRLF, which is how the file is
// taken to end all its lines.
func endsLinesWithCRLF(data []byte) bool {
	first := bytes.IndexByte(data, '\n')

	return first > 0 && data[first-1] == '\r'
}
