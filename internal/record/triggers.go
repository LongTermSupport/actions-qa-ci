package record

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
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
// touches, each of which must start no run on its own. Inputs and Exempt are the only ways it
// narrows the check, and each entry states its reason.
type DocsOnly struct {
	// Events are the trigger events checked as well as push.
	Events []string
	// Paths are repository-relative files checked as well as the defaults.
	Paths []string
	// Inputs are documentation files the build or the tests read, which are not documentation
	// to the check.
	Inputs []DocsOnlyInput
	// Exempt are workflows that must run for a docs-only change.
	Exempt []DocsOnlyExempt

	eventsStated, pathsStated, inputsStated, exemptStated bool
}

// DocsOnlyInput is a pattern (see package glob) of documentation files that are a real input of
// the build or the tests, with the reason.
type DocsOnlyInput struct {
	Path   string `yaml:"path"`
	Reason string `yaml:"reason"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// DocsOnlyExempt is a workflow that is not held to actionsqaci.docsOnlyPush, with the reason.
type DocsOnlyExempt struct {
	Workflow string `yaml:"workflow"`
	Reason   string `yaml:"reason"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// docsOnlyPushID is the identifier of the defence the inputs and exempt entries narrow; a reason
// may not only repeat it.
const docsOnlyPushID = finding.ToolPrefix + ".docsOnlyPush"

// Faults lists what is wrong with the input's reason, held to the standard of an exception's
// hazard.
func (i DocsOnlyInput) Faults() []Fault {
	return reasonFaults(i.Reason)
}

// Faults lists what is wrong with the exempt entry's reason, held to the standard of an
// exception's hazard.
func (e DocsOnlyExempt) Faults() []Fault {
	return reasonFaults(e.Reason)
}

func reasonFaults(text string) []Fault {
	if reason := reasonFault(normalise(text), docsOnlyPushID); reason != "" {
		return []Fault{{"reason", reason}}
	}

	return nil
}

// docsOnlyDocument is DocsOnly as the decoder reads it: a list written empty is told apart from
// one not written.
type docsOnlyDocument struct {
	Events *[]string          `yaml:"events"`
	Paths  *[]string          `yaml:"paths"`
	Inputs *[]*DocsOnlyInput  `yaml:"inputs"`
	Exempt *[]*DocsOnlyExempt `yaml:"exempt"`
}

// EffectiveEvents are the events checked: push always, and those the record adds. The record can
// widen the events and never narrow them; a workflow that must run anyway is exempt.
func (d DocsOnly) EffectiveEvents() []string {
	return union([]string{"push"}, d.Events)
}

// EffectivePaths are the files a docs-only change touches: the defaults, and those the record
// adds, less those an input names.
func (d DocsOnly) EffectivePaths() []string {
	var out []string
	for _, p := range union(DefaultDocsOnlyPaths, d.Paths) {
		if !d.IsInput(p) {
			out = append(out, p)
		}
	}

	return out
}

// IsInput says whether an input names the repository-relative path.
func (d DocsOnly) IsInput(p string) bool {
	for _, input := range d.Inputs {
		if glob.Match(input.Path, p) {
			return true
		}
	}

	return false
}

// Exempts says whether the workflow at the repository-relative path is exempt.
func (d DocsOnly) Exempts(workflow string) bool {
	for _, e := range d.Exempt {
		if e.Workflow == workflow {
			return true
		}
	}

	return false
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

// docsOnlyFrom builds DocsOnly from what was decoded and the lines its inputs and exempt entries
// are written on, with a fault for each entry written as nothing at all.
func docsOnlyFrom(doc *docsOnlyDocument, inputLines, exemptLines []int) (DocsOnly, []string) {
	var d DocsOnly
	var faults []string
	if doc == nil {
		return d, nil
	}
	if doc.Events != nil {
		d.Events, d.eventsStated = *doc.Events, true
	}
	if doc.Paths != nil {
		d.Paths, d.pathsStated = *doc.Paths, true
	}
	if doc.Inputs != nil {
		d.inputsStated = true
		for i, entry := range *doc.Inputs {
			if entry == nil {
				faults = append(faults, fmt.Sprintf("docsOnly: input %d (line %d) is empty", i+1, inputLines[i]))
				d.Inputs = append(d.Inputs, DocsOnlyInput{Line: inputLines[i], null: true})

				continue
			}
			entry.Line = inputLines[i]
			d.Inputs = append(d.Inputs, *entry)
		}
	}
	if doc.Exempt != nil {
		d.exemptStated = true
		for i, entry := range *doc.Exempt {
			if entry == nil {
				faults = append(faults, fmt.Sprintf("docsOnly: exempt entry %d (line %d) is empty", i+1, exemptLines[i]))
				d.Exempt = append(d.Exempt, DocsOnlyExempt{Line: exemptLines[i], null: true})

				continue
			}
			entry.Line = exemptLines[i]
			d.Exempt = append(d.Exempt, *entry)
		}
	}

	return d, faults
}

// writtenInputs and writtenExempt are how many inputs and exempt entries the decoded section
// holds, to be matched to the lines they are written on.
func (doc *docsOnlyDocument) writtenInputs() int {
	if doc == nil || doc.Inputs == nil {
		return 0
	}

	return len(*doc.Inputs)
}

func (doc *docsOnlyDocument) writtenExempt() int {
	if doc == nil || doc.Exempt == nil {
		return 0
	}

	return len(*doc.Exempt)
}

// isWorkflowPath says whether a clean repository-relative path is where GitHub reads a workflow:
// a .yml or .yaml file directly in .github/workflows.
func isWorkflowPath(p string) bool {
	name, inWorkflows := strings.CutPrefix(p, ".github/workflows/")

	return inWorkflows && !strings.Contains(name, "/") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

// docsOnlyNarrowingFaults are what is wrong with the inputs and exempt entries as written. A
// reason is judged by Faults and reported as a finding; these are the shapes no reason can mend.
func docsOnlyNarrowingFaults(d DocsOnly) []string {
	var faults []string

	if d.inputsStated && len(d.Inputs) == 0 {
		faults = append(faults, "docsOnly: inputs is empty; leave it out, or name the documentation files the build or the tests read")
	}
	listed := map[string]bool{}
	for i, input := range d.Inputs {
		if input.null {
			continue // reported once, by docsOnlyFrom
		}
		where := fmt.Sprintf("docsOnly: input %d (line %d)", i+1, input.Line)
		if input.Path == "" {
			faults = append(faults, where+": path is missing")

			continue
		}
		if err := glob.Check(input.Path); err != nil {
			faults = append(faults, fmt.Sprintf("%s: path %q %s", where, input.Path, err))

			continue
		}
		if listed[input.Path] {
			faults = append(faults, fmt.Sprintf("%s: path %q is listed twice", where, input.Path))
		}
		listed[input.Path] = true
		for _, p := range d.Paths {
			if glob.Match(input.Path, p) {
				faults = append(faults, fmt.Sprintf("%s: path %q names %s, which docsOnly.paths says a docs-only change touches; a file is documentation or an input, not both", where, input.Path, p))
			}
		}
	}
	if len(d.Inputs) > 0 && len(d.EffectivePaths()) == 0 {
		faults = append(faults, "docsOnly: inputs take out every file a docs-only change touches, which turns the check off; a workflow that must run for documentation is listed under docsOnly.exempt with its reason")
	}

	if d.exemptStated && len(d.Exempt) == 0 {
		faults = append(faults, "docsOnly: exempt is empty; leave it out, or name the workflows that must run for a docs-only change")
	}
	exempt := map[string]bool{}
	for i, e := range d.Exempt {
		if e.null {
			continue // reported once, by docsOnlyFrom
		}
		where := fmt.Sprintf("docsOnly: exempt entry %d (line %d)", i+1, e.Line)
		switch problem := pathProblem(e.Workflow); {
		case e.Workflow == "":
			faults = append(faults, where+": workflow is missing")
		case problem != "":
			faults = append(faults, fmt.Sprintf("%s: workflow %q %s", where, e.Workflow, problem))
		case !isWorkflowPath(e.Workflow):
			faults = append(faults, fmt.Sprintf("%s: %q is not a workflow; name a .yml or .yaml file directly in .github/workflows", where, e.Workflow))
		case exempt[e.Workflow]:
			faults = append(faults, fmt.Sprintf("%s: workflow %q is listed twice", where, e.Workflow))
		}
		exempt[e.Workflow] = true
	}

	return faults
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

	faults = append(faults, docsOnlyNarrowingFaults(d)...)

	if s.stated && (s.MaxRunsPerDay < 1 || s.MaxRunsPerDay > DefaultMaxRunsPerDay) {
		faults = append(faults, fmt.Sprintf("scheduleBudget: maxRunsPerDay %d must be from 1 to %d; a workflow that must run more often than hourly takes an exception with a justification", s.MaxRunsPerDay, DefaultMaxRunsPerDay))
	}

	return faults
}
