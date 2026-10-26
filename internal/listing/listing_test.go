package listing_test

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

func narrowingRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg, err := registry.NewWithFamilies([]registry.Family{{
		Name:    "actionsqaci.shellcheck.SC<nnnn>",
		Page:    "actionsqaci.shellcheck",
		Summary: "Shell is free of ShellCheck warnings",
		Member:  regexp.MustCompile(`^actionsqaci\.shellcheck\.SC[0-9]{4}$`).MatchString,
		Check:   func(string) func(string) ([]finding.Finding, error) { return nil },
	}}, registry.Rule{ID: "actionsqaci.templateDrift", Summary: "A workflow matches its template"})
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

func TestNarrowMarksTheDefencesTheRecordTurnsOffEverywhere(t *testing.T) {
	reg := narrowingRegistry(t)
	rec := &record.Record{Exceptions: []record.Exception{
		{ID: "actionsqaci.templateDrift", Line: 5},
		{ID: "actionsqaci.shellcheck.SC2086", Line: 9},
		{ID: "actionsqaci.templateDrift", Path: ".github/workflows/ci.yml", Line: 12},
	}}
	entries := listing.Active(reg)

	listing.Narrow(reg, entries, rec)

	got := map[string][]string{}
	for _, entry := range entries {
		got[entry.ID] = entry.NarrowedBy
	}
	want := map[string][]string{
		"actionsqaci.shellcheck.SC<nnnn>": {"actionsqaci.shellcheck.SC2086 is disabled everywhere by " + record.Path + ":9"},
		"actionsqaci.templateDrift":       {"actionsqaci.templateDrift is disabled everywhere by " + record.Path + ":5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("narrowed by %v; want %v (an exception with a path narrows nothing everywhere)", got, want)
	}
}

func TestAnOwnerListsTheReportsOfOtherToolsItAbsorbs(t *testing.T) {
	reg, err := registry.New(
		registry.Rule{ID: "actionsqaci.zizmor.template-injection", Summary: "No expansion puts attacker data into code", Absorbs: []string{"one report", "another report"}},
		registry.Rule{ID: "actionsqaci.templateDrift", Summary: "A workflow matches its template"},
	)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]listing.Entry{}
	for _, entry := range listing.Active(reg) {
		got[entry.ID] = entry
	}

	owner := got["actionsqaci.zizmor.template-injection"]
	if !reflect.DeepEqual(owner.Absorbs, []string{"one report", "another report"}) {
		t.Errorf("absorbs = %v", owner.Absorbs)
	}
	if want := "No expansion puts attacker data into code (also stands for one report; another report; the other tool's report is dropped)"; owner.Summary != want {
		t.Errorf("summary = %q, want %q", owner.Summary, want)
	}
	if other := got["actionsqaci.templateDrift"]; other.Summary != "A workflow matches its template" || len(other.Absorbs) != 0 {
		t.Errorf("a rule that absorbs nothing is listed as %+v", other)
	}
}

func TestEnableMarksTheDefencesTheRecordAsksFor(t *testing.T) {
	reg := narrowingRegistry(t)
	rec := &record.Record{Calibrations: record.Calibrations{Enable: []record.Enabled{
		{ID: "actionsqaci.templateDrift", Line: 5},
		{ID: "actionsqaci.nonsense", Line: 6},
	}}}
	entries := listing.Active(reg)

	listing.Enable(entries, rec)

	got := map[string][]string{}
	for _, entry := range entries {
		got[entry.ID] = entry.EnabledBy
	}
	want := map[string][]string{
		"actionsqaci.shellcheck.SC<nnnn>": nil,
		"actionsqaci.templateDrift":       {"actionsqaci.templateDrift is enabled by " + record.Path + ":5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enabled by %v; want %v", got, want)
	}
}

func TestEnableWithNoRecordChangesNothing(t *testing.T) {
	entries := listing.Active(narrowingRegistry(t))

	listing.Enable(entries, nil)

	for _, entry := range entries {
		if len(entry.EnabledBy) != 0 {
			t.Errorf("%s enabled by %v", entry.ID, entry.EnabledBy)
		}
	}
}

func TestNarrowWithNoRecordChangesNothing(t *testing.T) {
	reg := narrowingRegistry(t)
	entries := listing.Active(reg)

	listing.Narrow(reg, entries, nil)

	for _, entry := range entries {
		if len(entry.NarrowedBy) != 0 {
			t.Errorf("%s narrowed by %v", entry.ID, entry.NarrowedBy)
		}
	}
}

func TestActiveListsEveryRuleAndFamilyOnceInIdentifierOrder(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	got := listing.Active(reg)

	ids := make([]string, len(got))
	for i, entry := range got {
		ids[i] = entry.ID
		if entry.Summary == "" || entry.DocPath == "" {
			t.Errorf("%s has no summary or page: %+v", entry.ID, entry)
		}
	}
	if !slices.IsSorted(ids) {
		t.Errorf("identifiers are not in order: %q", ids)
	}
	if len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Errorf("an identifier is listed twice: %q", ids)
	}
	if want := len(reg.All()) + len(reg.Families()); len(got) != want {
		t.Errorf("listed %d entries, the registry holds %d rules and families", len(got), want)
	}
}
