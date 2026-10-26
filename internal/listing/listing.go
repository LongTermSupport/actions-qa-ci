// Package listing is the list of active defences as `rules` and the agent summary state it: one
// entry for every rule and for every family of rules, which is listed once under its name.
package listing

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

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
	// Absorbs names the reports of other tools this defence stands for, which are dropped where
	// those tools report them, so that a hazard has the one owner and the one identifier.
	Absorbs []string `json:"absorbs,omitempty"`
	// NarrowedBy says how the project record turns the defence off, which is a narrowing of the
	// method's standard and has to be visible wherever the defences are listed.
	NarrowedBy []string `json:"narrowedBy,omitempty"`
	// EnabledBy says how the project record asks for a defence the lanes run only on request.
	EnabledBy []string `json:"enabledBy,omitempty"`
}

// Enable marks, on the entries, each defence the project record asks the lanes to run. A nil
// record asks for none.
func Enable(entries []Entry, rec *record.Record) {
	if rec == nil {
		return
	}

	for _, enabled := range rec.Calibrations.Enable {
		for i := range entries {
			if entries[i].ID == enabled.ID {
				entries[i].EnabledBy = append(entries[i].EnabledBy, fmt.Sprintf("%s is enabled by %s:%d", enabled.ID, record.Path, enabled.Line))

				break
			}
		}
	}
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
		summary := rule.Summary
		if len(rule.Absorbs) > 0 {
			summary += " (also stands for " + strings.Join(rule.Absorbs, "; ") + "; the other tool's report is dropped)"
		}
		entries = append(entries, Entry{ID: rule.ID, Summary: summary, Upstream: rule.Upstream, DocPath: registry.DocPath(rule.ID), Absorbs: rule.Absorbs})
	}
	for _, family := range reg.Families() {
		entries = append(entries, Entry{ID: family.Name, Summary: family.Summary, Upstream: family.Upstream, DocPath: pages.Directory + "/" + family.Page + ".md"})
	}
	slices.SortFunc(entries, func(a, b Entry) int { return cmp.Compare(a.ID, b.ID) })

	return entries
}
