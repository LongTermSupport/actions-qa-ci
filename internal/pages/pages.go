// Package pages is everything about the one page per rule identifier: finding a page,
// generating the identifier index from the registry, and auditing that the pages and the
// registry agree. The pages are read from a file system rooted at the repository, so the
// program reads the copy embedded in its own binary and a test reads the working tree.
package pages

import (
	"cmp"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Directory is where the pages live, relative to the root of the file system.
const Directory = "docs/rules"

// IndexFile is the generated identifier index inside Directory.
const IndexFile = "README.md"

// RequiredSections are the headings every page has, in the order they appear.
var RequiredSections = []string{
	"## What it forbids or requires",
	"## Hazard",
	"## Correct construction",
	"## Exceptions",
	"## Upstream",
}

// Page returns the page for a rule identifier. A missing page wraps fs.ErrNotExist; an
// identifier that is not of the identifier scheme is refused, so it can never name a path.
func Page(fsys fs.FS, id string) (string, error) {
	if _, err := finding.ClassifyIdentifier(id); err != nil {
		return "", fmt.Errorf("no page for %q: %w", id, err)
	}

	content, err := fs.ReadFile(fsys, registry.DocPath(id))
	if err != nil {
		return "", fmt.Errorf("page for %s: %w", id, err)
	}

	return string(content), nil
}

// Index generates the identifier index from the registry. It is the whole content of the
// index file, so a test can compare it byte for byte with the file in the repository.
func Index(reg *registry.Registry) string {
	var out strings.Builder

	out.WriteString("# Rule identifiers\n\n")
	out.WriteString("Generated from the registry; do not edit. Regenerate it with\n")
	out.WriteString("`go test . -run TestTheIdentifierIndexIsCurrent -update`.\n\n")

	type row struct{ id, summary, page string }
	var rows []row
	for _, rule := range reg.All() {
		rows = append(rows, row{rule.ID, rule.Summary, rule.ID})
	}
	for _, family := range reg.Families() {
		rows = append(rows, row{family.Name, family.Summary, family.Page})
	}
	if len(rows) == 0 {
		out.WriteString("No rules are registered yet.\n")

		return out.String()
	}
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(a.id, b.id) })

	out.WriteString("| Identifier | Standing instruction | Page |\n")
	out.WriteString("| --- | --- | --- |\n")
	for _, r := range rows {
		fmt.Fprintf(&out, "| `%s` | %s | [page](%s.md) |\n", r.id, strings.ReplaceAll(r.summary, "|", `\|`), r.page)
	}

	return out.String()
}

// Audit lists every way the pages and the registry disagree: a rule with no page, a page
// missing a required section, a page for no registered rule. An unreadable pages directory is
// a problem, never a pass.
func Audit(reg *registry.Registry, fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, Directory)
	if err != nil {
		return []string{fmt.Sprintf("cannot read %s: %v", Directory, err)}
	}

	onDisk := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != IndexFile && path.Ext(entry.Name()) == ".md" {
			onDisk[strings.TrimSuffix(entry.Name(), ".md")] = true
		}
	}

	var problems []string
	registered := map[string]bool{}
	check := func(name, stem string) {
		registered[stem] = true

		if !onDisk[stem] {
			problems = append(problems, fmt.Sprintf("%s has no page at %s", name, pagePath(stem)))

			return
		}
		problems = append(problems, sectionProblems(fsys, stem)...)
	}
	for _, rule := range reg.All() {
		check(rule.ID, rule.ID)
	}
	for _, family := range reg.Families() {
		check(family.Name, family.Page)
	}

	for name := range onDisk {
		if !registered[name] {
			problems = append(problems, fmt.Sprintf("%s is a page for no registered rule", pagePath(name)))
		}
	}

	slices.Sort(problems)

	return problems
}

func pagePath(stem string) string {
	return Directory + "/" + stem + ".md"
}

func sectionProblems(fsys fs.FS, stem string) []string {
	data, err := fs.ReadFile(fsys, pagePath(stem))
	if err != nil {
		return []string{fmt.Sprintf("page %s: %v", stem, err)}
	}
	where := pagePath(stem)

	sections, closed := parseSections(string(data))
	if !closed {
		// Everything after the open fence reads as code, so the sections it hides would
		// be reported missing; say what is actually wrong instead.
		return []string{where + " has a code fence that is never closed"}
	}

	var problems []string
	next := 0 // the first section not yet used, so the required ones are found in order

	for _, required := range RequiredSections {
		found := slices.IndexFunc(sections[next:], func(s section) bool { return s.heading == required })
		if found < 0 {
			if slices.ContainsFunc(sections, func(s section) bool { return s.heading == required }) {
				problems = append(problems, fmt.Sprintf("%s has the section %q out of order", where, required))
			} else {
				problems = append(problems, fmt.Sprintf("%s lacks the section %q", where, required))
			}

			continue
		}

		found += next
		next = found + 1
		if strings.TrimSpace(sections[found].body) == "" {
			problems = append(problems, fmt.Sprintf("%s has an empty section %q", where, required))
		}
	}

	return problems
}

// section is a level-two heading and the text under it up to the next one.
type section struct {
	heading string
	body    string
}

// parseSections splits a page at its level-two headings. A heading inside a fenced code
// block is example text, not a heading, so fences are tracked; the second result is false
// when a fence is still open at the end of the page. Both line endings are accepted.
func parseSections(content string) ([]section, bool) {
	var sections []section
	var body strings.Builder
	var fenceChar byte // 0 outside a fence
	fenceLength := 0

	flush := func() {
		if len(sections) > 0 {
			sections[len(sections)-1].body = body.String()
		}
		body.Reset()
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimLeft(line, " ")

		if fenceChar != 0 {
			if closesFence(trimmed, fenceChar, fenceLength) {
				fenceChar = 0
			}
			body.WriteString(line + "\n")

			continue
		}

		if char, length := openingFence(trimmed); length > 0 {
			fenceChar, fenceLength = char, length
		} else if strings.HasPrefix(line, "## ") {
			flush()
			sections = append(sections, section{heading: strings.TrimRight(line, " \t")})

			continue
		}
		body.WriteString(line + "\n")
	}
	flush()

	return sections, fenceChar == 0
}

// openingFence reads a line that opens a fenced code block: three or more backticks or
// tildes. The length is 0 for any other line.
func openingFence(trimmed string) (byte, int) {
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0
	}

	length := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	if length < 3 {
		return 0, 0
	}

	return trimmed[0], length
}

// closesFence is true for a line made only of the opening character, at least as many of
// them as opened the fence: a shorter run, another character or trailing text do not close it.
func closesFence(trimmed string, char byte, length int) bool {
	run := strings.TrimRight(trimmed, " \t")

	return len(run) >= length && strings.Trim(run, string(char)) == ""
}
