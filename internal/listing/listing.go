// Package listing is the list of active defences as `rules` and the agent summary state it: one
// entry for every rule and for every family of rules, which is listed once under its name.
package listing

import (
	"cmp"
	"slices"

	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Entry is one defence as `rules --json` prints it.
type Entry struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Upstream string `json:"upstream"`
	DocPath  string `json:"docPath"`
	// NarrowedBy says how the project record turns the defence off, which is a narrowing of the
	// method's standard and has to be visible wherever the defences are listed.
	NarrowedBy []string `json:"narrowedBy,omitempty"`
}

// Active lists every individual rule and every family, in identifier order. A family is listed
// once, under its placeholder name, not once per member.
func Active(reg *registry.Registry) []Entry {
	entries := []Entry{}
	for _, rule := range reg.All() {
		entries = append(entries, Entry{ID: rule.ID, Summary: rule.Summary, Upstream: rule.Upstream, DocPath: registry.DocPath(rule.ID)})
	}
	for _, family := range reg.Families() {
		entries = append(entries, Entry{ID: family.Name, Summary: family.Summary, Upstream: family.Upstream, DocPath: pages.Directory + "/" + family.Page + ".md"})
	}
	slices.SortFunc(entries, func(a, b Entry) int { return cmp.Compare(a.ID, b.ID) })

	return entries
}
