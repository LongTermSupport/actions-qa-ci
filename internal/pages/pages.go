// Package pages is everything about the one page per rule identifier: finding a page,
// generating the identifier index from the registry, and auditing that the pages and the
// registry agree. The pages are read from a file system rooted at the repository, so the
// program reads the copy embedded in its own binary and a test reads the working tree.
package pages

import (
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

	rules := reg.All()
	if len(rules) == 0 {
		out.WriteString("No rules are registered yet.\n")

		return out.String()
	}

	out.WriteString("| Identifier | Standing instruction | Page |\n")
	out.WriteString("| --- | --- | --- |\n")
	for _, rule := range rules {
		fmt.Fprintf(&out, "| `%s` | %s | [page](%s.md) |\n", rule.ID, rule.Summary, rule.ID)
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
	for _, rule := range reg.All() {
		registered[rule.ID] = true

		if !onDisk[rule.ID] {
			problems = append(problems, fmt.Sprintf("%s has no page at %s", rule.ID, registry.DocPath(rule.ID)))

			continue
		}
		problems = append(problems, sectionProblems(fsys, rule.ID)...)
	}

	for name := range onDisk {
		if !registered[name] {
			problems = append(problems, fmt.Sprintf("%s is a page for no registered rule", registry.DocPath(name)))
		}
	}

	slices.Sort(problems)

	return problems
}

func sectionProblems(fsys fs.FS, id string) []string {
	content, err := Page(fsys, id)
	if err != nil {
		return []string{err.Error()}
	}

	var problems []string
	for _, section := range RequiredSections {
		if !hasHeading(content, section) {
			problems = append(problems, fmt.Sprintf("%s lacks the section %q", registry.DocPath(id), section))
		}
	}

	return problems
}

func hasHeading(content, heading string) bool {
	return slices.Contains(strings.Split(content, "\n"), heading)
}
