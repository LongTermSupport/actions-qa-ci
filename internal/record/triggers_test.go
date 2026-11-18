package record_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestARecordWithoutTheTriggerSectionsGetsTheDefaults(t *testing.T) {
	rec, err := record.Parse([]byte("version: 1\n"), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if got := rec.DocsOnly.EffectiveEvents(); !slices.Equal(got, []string{"push"}) {
		t.Errorf("events = %q, want push alone", got)
	}
	if got := rec.DocsOnly.EffectivePaths(); !slices.Equal(got, record.DefaultDocsOnlyPaths) {
		t.Errorf("paths = %q, want the defaults %q", got, record.DefaultDocsOnlyPaths)
	}
	for _, want := range []string{"README.md", "docs/images/diagram.png"} {
		if !slices.Contains(record.DefaultDocsOnlyPaths, want) {
			t.Errorf("the default docs-only paths %q lack %s", record.DefaultDocsOnlyPaths, want)
		}
	}
	if got := rec.ScheduleBudget.EffectiveMaxRunsPerDay(); got != 24 {
		t.Errorf("maxRunsPerDay = %d, want 24 (hourly)", got)
	}
}

func TestTheZeroRecordGetsTheDefaultsToo(t *testing.T) {
	var rec record.Record
	if got := rec.DocsOnly.EffectiveEvents(); !slices.Equal(got, []string{"push"}) {
		t.Errorf("events = %q", got)
	}
	if got := rec.ScheduleBudget.EffectiveMaxRunsPerDay(); got != record.DefaultMaxRunsPerDay {
		t.Errorf("maxRunsPerDay = %d", got)
	}
}

func TestARecordStatesItsDocsOnlyChangeAndScheduleBudget(t *testing.T) {
	data := "version: 1\ndocsOnly:\n  events: [push, pull_request]\n  paths:\n    - README.md\n    - guide/usage.md\nscheduleBudget:\n  maxRunsPerDay: 4\n"
	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if got := rec.DocsOnly.EffectiveEvents(); !slices.Equal(got, []string{"push", "pull_request"}) {
		t.Errorf("events = %q", got)
	}
	if got, want := rec.DocsOnly.EffectivePaths(), append(slices.Clone(record.DefaultDocsOnlyPaths), "guide/usage.md"); !slices.Equal(got, want) {
		t.Errorf("paths = %q, want the defaults and the record's own, once each: %q", got, want)
	}
	if got := rec.ScheduleBudget.EffectiveMaxRunsPerDay(); got != 4 {
		t.Errorf("maxRunsPerDay = %d, want 4", got)
	}
}

func TestDocsOnlyEventsAndPathsWidenTheCheckAndNeverNarrowIt(t *testing.T) {
	rec, err := record.Parse([]byte("version: 1\ndocsOnly:\n  events: [pull_request]\n  paths: [guide/usage.md]\n"), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.DocsOnly.EffectiveEvents(); !slices.Equal(got, []string{"push", "pull_request"}) {
		t.Errorf("events = %q; push is always checked", got)
	}
	for _, p := range record.DefaultDocsOnlyPaths {
		if !slices.Contains(rec.DocsOnly.EffectivePaths(), p) {
			t.Errorf("paths %q lack the default %s", rec.DocsOnly.EffectivePaths(), p)
		}
	}
}

func TestATriggerSectionThatWouldSilenceOrMisstateADefenceIsRefused(t *testing.T) {
	cases := map[string]struct{ data, mention string }{
		"no events":                  {"version: 1\ndocsOnly:\n  events: []\n", "events"},
		"an event with no filter":    {"version: 1\ndocsOnly:\n  events: [schedule]\n", "schedule"},
		"an event twice":             {"version: 1\ndocsOnly:\n  events: [push, push]\n", "twice"},
		"no paths":                   {"version: 1\ndocsOnly:\n  paths: []\n", "paths"},
		"an empty path":              {"version: 1\ndocsOnly:\n  paths:\n    -\n", "empty"},
		"a pattern for a path":       {"version: 1\ndocsOnly:\n  paths: ['**.md']\n", "glob"},
		"an absolute path":           {"version: 1\ndocsOnly:\n  paths: [/README.md]\n", "absolute"},
		"an unknown key":             {"version: 1\ndocsOnly:\n  reinclude: [CLAUDE.md]\n", "reinclude"},
		"a budget of nothing":        {"version: 1\nscheduleBudget:\n  maxRunsPerDay: 0\n", "maxRunsPerDay"},
		"a budget above hourly":      {"version: 1\nscheduleBudget:\n  maxRunsPerDay: 48\n", "exception"},
		"a budget that is no number": {"version: 1\nscheduleBudget:\n  maxRunsPerDay: often\n", "often"},
		"a budget not stated":        {"version: 1\nscheduleBudget: {}\n", "maxRunsPerDay"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), everythingIsKnown)
		var invalid *record.InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error %v, want an invalid record", name, err)

			continue
		}
		if !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error %q must mention %q", name, err.Error(), c.mention)
		}
	}
}
