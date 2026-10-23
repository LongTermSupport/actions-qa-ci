// Package changelog records a pinned tool's move in CHANGELOG.md, under the Unreleased section's
// Changed heading, so a consumer reading the changelog sees that what the program bundles changed.
package changelog

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Move is one tool taken from one version to another.
type Move struct {
	Tool, From, To string
}

const (
	unreleased = "## Unreleased"
	changed    = "### Changed"

	// toolExpression and versionExpression are all a move may contain, so that nothing a release
	// lookup returned can become markdown of its own.
	toolExpression    = `[A-Za-z0-9][A-Za-z0-9._/-]{0,99}`
	versionExpression = `v?(?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8})`
)

var (
	toolPattern    = regexp.MustCompile(`^` + toolExpression + `$`)
	versionPattern = regexp.MustCompile(`^` + versionExpression + `$`)
	entryPattern   = regexp.MustCompile("^- `(" + toolExpression + ")` moves from (" + versionExpression + ") to (" + versionExpression + `)\.$`)

	// rank is the order Keep a Changelog gives the headings of a section.
	rank = map[string]int{"### Added": 0, changed: 1, "### Deprecated": 2, "### Removed": 3, "### Fixed": 4, "### Security": 5}
)

// Record writes the moves into the changelog as entries under the Unreleased section's Changed
// heading, which it makes when there is none. A tool that already has an entry there keeps the
// version it started from, and a tool that is back where it started has no entry. The result has
// the line endings of the input, and no moves is the input unchanged.
func Record(content []byte, moves []Move) ([]byte, error) {
	for _, move := range moves {
		if err := check(move); err != nil {
			return nil, err
		}
	}
	if len(moves) == 0 {
		return slices.Clone(content), nil
	}

	eol := "\n"
	if strings.Contains(string(content), "\r\n") {
		eol = "\r\n"
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	finalNewline := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")

	start := slices.Index(lines, unreleased)
	if start < 0 {
		return nil, errors.New("the changelog has no \"" + unreleased + "\" section to record the move in")
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i

			break
		}
	}

	lines = record(lines, start, end, moves)

	out := strings.Join(lines, eol)
	if finalNewline {
		out += eol
	}

	return []byte(out), nil
}

func check(move Move) error {
	if !toolPattern.MatchString(move.Tool) {
		return fmt.Errorf("%q is not a tool name that can be written into the changelog", move.Tool)
	}
	if !versionPattern.MatchString(move.From) || !versionPattern.MatchString(move.To) {
		return fmt.Errorf("%s: %q to %q are not both versions", move.Tool, move.From, move.To)
	}
	if move.From == move.To {
		return fmt.Errorf("%s: a move from %s to itself is not a move", move.Tool, move.From)
	}

	return nil
}

// record returns the lines with the moves written into the Unreleased section, which is
// lines[start:end] and starts at its heading.
func record(lines []string, start, end int, moves []Move) []string {
	section := slices.Index(lines[start+1:end], changed)
	if section < 0 {
		return insert(lines, start, end, entries(nil, moves))
	}
	section += start + 1

	stop := end
	for i := section + 1; i < end; i++ {
		if strings.HasPrefix(lines[i], "### ") {
			stop = i

			break
		}
	}

	var kept []string
	existing := map[string][2]string{}
	for _, line := range lines[section+1 : stop] {
		if match := entryPattern.FindStringSubmatch(line); match != nil {
			existing[match[1]] = [2]string{match[2], match[3]}

			continue
		}
		kept = append(kept, line)
	}
	block := entries(existing, moves)
	kept = trimBlank(kept)

	var replacement []string
	if len(kept) > 0 || len(block) > 0 {
		replacement = append(replacement, changed, "")
		replacement = append(replacement, kept...)
		replacement = append(replacement, block...)
		if stop < len(lines) {
			replacement = append(replacement, "")
		}
	}

	return slices.Concat(lines[:section], replacement, lines[stop:])
}

// entries is the generated entries as lines, in tool order, once the moves are applied to the
// ones already there. A tool back at its first version has none.
func entries(existing map[string][2]string, moves []Move) []string {
	span := map[string][2]string{}
	for tool, versions := range existing {
		span[tool] = versions
	}
	for _, move := range moves {
		versions, ok := span[move.Tool]
		if !ok {
			versions[0] = move.From
		}
		versions[1] = move.To
		span[move.Tool] = versions
	}

	tools := make([]string, 0, len(span))
	for tool, versions := range span {
		if versions[0] != versions[1] {
			tools = append(tools, tool)
		}
	}
	slices.Sort(tools)

	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		lines = append(lines, "- `"+tool+"` moves from "+span[tool][0]+" to "+span[tool][1]+".")
	}

	return lines
}

// insert puts a new Changed section into the Unreleased section lines[start:end]: before the
// first heading that comes after Changed, or at the end of the section.
func insert(lines []string, start, end int, block []string) []string {
	if len(block) == 0 {
		return lines
	}

	for i := start + 1; i < end; i++ {
		order, known := rank[lines[i]]
		if strings.HasPrefix(lines[i], "### ") && (!known || order > rank[changed]) {
			section := slices.Concat([]string{changed, ""}, block, []string{""})

			return slices.Concat(lines[:i], section, lines[i:])
		}
	}

	last := end
	for last > start+1 && strings.TrimSpace(lines[last-1]) == "" {
		last--
	}
	section := slices.Concat([]string{"", changed, ""}, block)

	return slices.Concat(lines[:last], section, lines[last:])
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}
