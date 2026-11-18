package record

import (
	"fmt"
	"slices"
)

// DefaultDocsOnlyPaths are the files a docs-only change touches when the record does not say:
// Markdown at the root and deeper, and a page and an image under docs/. A workflow that a push
// of any one of them alone would start runs full CI for documentation.
var DefaultDocsOnlyPaths = []string{"README.md", "notes/design.md", "docs/guide/install.md", "docs/images/diagram.png"}

// DefaultMaxRunsPerDay is the schedule budget when the record does not state one: hourly.
const DefaultMaxRunsPerDay = 24

// docsOnlyEvents are the events whose `paths` filter decides whether a docs-only change runs a
// workflow.
var docsOnlyEvents = []string{"push", "pull_request"}

// DocsOnly says what a docs-only change is for actionsqaci.docsOnlyPush: the events, beyond push,
// on which a workflow must not run for one, and files, beyond DefaultDocsOnlyPaths, such a change
// touches, each of which must start no run on its own.
type DocsOnly struct {
	// Events are the trigger events checked as well as push.
	Events []string
	// Paths are repository-relative files checked as well as the defaults.
	Paths []string

	eventsStated, pathsStated bool
}

// docsOnlyDocument is DocsOnly as the decoder reads it: a list written empty is told apart from
// one not written.
type docsOnlyDocument struct {
	Events *[]string `yaml:"events"`
	Paths  *[]string `yaml:"paths"`
}

// EffectiveEvents are the events checked: push always, and those the record adds. The record can
// widen the check and never narrow it; a workflow it should not hold is an exception.
func (d DocsOnly) EffectiveEvents() []string {
	return union([]string{"push"}, d.Events)
}

// EffectivePaths are the files a docs-only change touches: the defaults, and those the record
// adds.
func (d DocsOnly) EffectivePaths() []string {
	return union(DefaultDocsOnlyPaths, d.Paths)
}

// union is a followed by each value of b not already in it.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, v := range b {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}

	return out
}

// ScheduleBudget bounds how often a scheduled workflow runs, for actionsqaci.scheduleBudget.
type ScheduleBudget struct {
	// MaxRunsPerDay is the most runs a day one workflow's schedule may start.
	MaxRunsPerDay int

	stated bool
}

type scheduleBudgetDocument struct {
	MaxRunsPerDay *int `yaml:"maxRunsPerDay"`
}

// EffectiveMaxRunsPerDay is the budget: the stated one, or hourly.
func (s ScheduleBudget) EffectiveMaxRunsPerDay() int {
	if !s.stated {
		return DefaultMaxRunsPerDay
	}

	return s.MaxRunsPerDay
}

func docsOnlyFrom(doc *docsOnlyDocument) DocsOnly {
	var d DocsOnly
	if doc == nil {
		return d
	}
	if doc.Events != nil {
		d.Events, d.eventsStated = *doc.Events, true
	}
	if doc.Paths != nil {
		d.Paths, d.pathsStated = *doc.Paths, true
	}

	return d
}

func scheduleBudgetFrom(doc *scheduleBudgetDocument, written bool) (ScheduleBudget, []string) {
	if !written {
		return ScheduleBudget{}, nil
	}
	if doc == nil || doc.MaxRunsPerDay == nil {
		return ScheduleBudget{}, []string{"scheduleBudget: maxRunsPerDay is missing; state the most runs a day a scheduled workflow may start, or leave scheduleBudget out for hourly"}
	}

	return ScheduleBudget{MaxRunsPerDay: *doc.MaxRunsPerDay, stated: true}, nil
}

// triggerFaults are what is wrong with the docsOnly and scheduleBudget sections. Neither may
// turn its defence off: a list written empty, or a budget above hourly, is refused, and a
// workflow that needs more is an exception with a justification.
func triggerFaults(d DocsOnly, s ScheduleBudget) []string {
	var faults []string

	if d.eventsStated {
		if len(d.Events) == 0 {
			faults = append(faults, "docsOnly: events is empty; leave it out to check push alone, or add pull_request")
		}
		for i, event := range d.Events {
			switch {
			case !slices.Contains(docsOnlyEvents, event):
				faults = append(faults, fmt.Sprintf("docsOnly: events entry %d %q is not push or pull_request, the events a paths filter applies to", i+1, event))
			case slices.Index(d.Events, event) != i:
				faults = append(faults, fmt.Sprintf("docsOnly: events names %s twice", event))
			}
		}
	}
	if d.pathsStated {
		if len(d.Paths) == 0 {
			faults = append(faults, "docsOnly: paths is empty; leave it out for the defaults, or name the files a docs-only change touches beyond them")
		}
		for i, p := range d.Paths {
			switch problem := pathProblem(p); {
			case p == "":
				faults = append(faults, fmt.Sprintf("docsOnly: paths entry %d is empty", i+1))
			case problem != "":
				faults = append(faults, fmt.Sprintf("docsOnly: paths entry %d %q %s", i+1, p, problem))
			}
		}
	}

	if s.stated && (s.MaxRunsPerDay < 1 || s.MaxRunsPerDay > DefaultMaxRunsPerDay) {
		faults = append(faults, fmt.Sprintf("scheduleBudget: maxRunsPerDay %d must be from 1 to %d; a workflow that must run more often than hourly takes an exception with a justification", s.MaxRunsPerDay, DefaultMaxRunsPerDay))
	}

	return faults
}
