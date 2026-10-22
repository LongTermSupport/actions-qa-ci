// Package listing is the list of active defences as `rules` and the agent summary state it: one
// entry for every rule and for every family of rules, which is listed once under its name.
package listing

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
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

// Narrow marks, on the entries, each defence the project record turns off everywhere. A narrowing
// of the method's standard is the Owner's, so wherever the defences are listed it is shown beside
// the defence it narrows. A family's entry carries the narrowing of any member. A nil record
// narrows nothing.
func Narrow(reg *registry.Registry, entries []Entry, rec *record.Record) {
	if rec == nil {
		return
	}

	for _, e := range rec.Exceptions {
		if !e.DisablesDefence() {
			continue
		}
		narrowing := fmt.Sprintf("%s is disabled everywhere by %s:%d", e.ID, record.Path, e.Line)
		for i := range entries {
			if entries[i].ID == e.ID {
				entries[i].NarrowedBy = append(entries[i].NarrowedBy, narrowing)

				break
			}
			if family, listed := reg.FamilyNamed(entries[i].ID); listed && family.Member(e.ID) {
				entries[i].NarrowedBy = append(entries[i].NarrowedBy, narrowing)

				break
			}
		}
	}
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
