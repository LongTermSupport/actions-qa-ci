package firstparty

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/ghfilter"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// The identifiers of the trigger-hygiene checks: what starts a workflow, and how often.
const (
	DocsOnlyPush      = finding.ToolPrefix + ".docsOnlyPush"
	PushConcurrency   = finding.ToolPrefix + ".pushConcurrency"
	WorkflowRunFanout = finding.ToolPrefix + ".workflowRunFanout"
	ScheduleBudget    = finding.ToolPrefix + ".scheduleBudget"
)

// dstar is the double star of a filter pattern, kept out of the literals below so that a
// pattern next to a slash never reads as a comment marker.
const dstar = "**"

// docsFilterExample is a paths filter that leaves documentation out and keeps the Markdown
// agents read as instructions in.
var docsFilterExample = fmt.Sprintf("paths: ['%[1]s', '!%[1]s.md', '!docs/%[1]s', '%[1]s/CLAUDE.md', '%[1]s/AGENTS.md']", dstar)

// TriggerRules returns the trigger-hygiene checks. known says whether an identifier belongs to
// a defence of this build, which the project record they read has to name.
func TriggerRules(known func(id string) bool) []registry.Rule {
	return []registry.Rule{{
		ID:      DocsOnlyPush,
		Summary: "A push that changes only documentation starts no run: every push-triggered workflow has a paths or paths-ignore filter that leaves docs out",
		Check:   CheckDocsOnlyPush(known),
	}, {
		ID:      PushConcurrency,
		Summary: "A workflow triggered by a push to main has a concurrency group keyed so that runs on main collapse",
		Check:   CheckPushConcurrency,
	}, {
		ID:      WorkflowRunFanout,
		Summary: "A job started by workflow_run gates on an allowlist of the watched run's conclusions, so a cancelled run starts nothing",
		Check:   CheckWorkflowRunFanout,
	}, {
		ID:      ScheduleBudget,
		Summary: "A workflow's cron schedules start no more runs a day than the budget, hourly unless the project record states less",
		Check:   CheckScheduleBudget(known),
	}}
}

// trigger is one event a workflow is triggered on: where it is written, and its configuration
// (nil when the event is named with none).
type trigger struct {
	line   int
	config *yaml.Node
}

// readWorkflow reads the top mapping of a workflow. A file that is not YAML, or whose top is not
// a mapping, gives nil and no error: actionsqaci.actionlint.syntax-check reports it, and these
// checks have nothing in it to judge. A file that cannot be read is an error.
func readWorkflow(file string) (*yaml.Node, error) {
	data, err := os.ReadFile(filepath.FromSlash(file))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}

	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil || len(document.Content) == 0 {
		return nil, nil
	}
	if top := resolve(document.Content[0]); top.Kind == yaml.MappingNode {
		return top, nil
	}

	return nil, nil
}

// entry is the key and the value of a key of a mapping, or two nils.
func entry(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	mapping = resolve(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i], resolve(mapping.Content[i+1])
		}
	}

	return nil, nil
}

// triggers reads the events of a workflow's `on`, in any of its three forms.
func triggers(top *yaml.Node) map[string]trigger {
	found := map[string]trigger{}
	_, on := entry(top, "on")
	if on == nil {
		return found
	}

	switch on.Kind {
	case yaml.ScalarNode:
		found[on.Value] = trigger{line: on.Line}
	case yaml.SequenceNode:
		for _, item := range on.Content {
			item = resolve(item)
			found[item.Value] = trigger{line: item.Line}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			config := resolve(on.Content[i+1])
			if config != nil && config.Kind == yaml.ScalarNode && config.ShortTag() == "!!null" {
				config = nil
			}
			found[on.Content[i].Value] = trigger{line: on.Content[i].Line, config: config}
		}
	}

	return found
}

// list is the strings of a key of a mapping, and whether the key is there.
func list(mapping *yaml.Node, key string) ([]string, bool) {
	k, v := entry(mapping, key)
	if k == nil {
		return nil, false
	}

	var values []string
	switch v.Kind {
	case yaml.ScalarNode:
		if v.ShortTag() != "!!null" {
			values = append(values, v.Value)
		}
	case yaml.SequenceNode:
		for _, item := range v.Content {
			if item = resolve(item); item.Kind == yaml.ScalarNode {
				values = append(values, item.Value)
			}
		}
	}

	return values, true
}

// written says whether a key is in a mapping.
func written(mapping *yaml.Node, key string) bool {
	k, v := entry(mapping, key)

	return k != nil || v != nil
}

// tagsOnly says whether a push trigger is filtered to tags and not to branches, so a push of a
// branch never starts it and its paths filter does not apply.
func tagsOnly(config *yaml.Node) bool {
	tags := written(config, "tags") || written(config, "tags-ignore")
	branches := written(config, "branches") || written(config, "branches-ignore")

	return tags && !branches
}

// loadRecord reads the project record, or gives the defaults when the project has none. An
// invalid record is an error: a check never guesses what a broken record meant.
func loadRecord(known func(id string) bool) (*record.Record, error) {
	rec, err := record.Load(record.Path, known)
	if errors.Is(err, fs.ErrNotExist) {
		return &record.Record{}, nil
	}

	return rec, err
}

// CheckDocsOnlyPush is the check on one workflow: a finding for each event of the record's
// docsOnly.events (push unless stated) on which a change touching only one of the record's
// docsOnly.paths, less its inputs, would start the workflow, judged through its paths or
// paths-ignore filter as GitHub reads it. A workflow the record exempts has none. Given the
// project record itself, it is the check on the record's inputs and exempt entries
// (checkDocsOnlyRecord).
func CheckDocsOnlyPush(known func(id string) bool) func(file string) ([]finding.Finding, error) {
	return func(file string) ([]finding.Finding, error) {
		if file == record.Path {
			return checkDocsOnlyRecord(known, file)
		}

		rec, err := loadRecord(known)
		if err != nil {
			return nil, err
		}
		if rec.DocsOnly.Exempts(file) {
			return nil, nil
		}

		return docsOnlyFindings(rec, file)
	}
}

// docsOnlyFindings are the findings of the check on one workflow, exempt or not.
func docsOnlyFindings(rec *record.Record, file string) ([]finding.Finding, error) {
	top, err := readWorkflow(file)
	if err != nil || top == nil {
		return nil, err
	}

	on := triggers(top)
	var found []finding.Finding
	for _, event := range rec.DocsOnly.EffectiveEvents() {
		t, ok := on[event]
		if !ok || (event == "push" && tagsOnly(t.config)) {
			continue
		}

		paths, filtered := list(t.config, "paths")
		ignore, ignoring := list(t.config, "paths-ignore")
		var runs []string
		for _, p := range rec.DocsOnly.EffectivePaths() {
			switch {
			case filtered && !ghfilter.Included(paths, p):
			case !filtered && ignoring && ghfilter.Ignored(ignore, p):
			default:
				runs = append(runs, p)
			}
		}
		if len(runs) == 0 {
			continue
		}

		why := "its " + event + " trigger has no paths or paths-ignore filter. Add one that leaves documentation out and keeps instruction Markdown in"
		if filtered || ignoring {
			why = "its " + event + " filter lets it through. Make the filter leave documentation out"
		}
		message := fmt.Sprintf("a %s that changes only documentation (%s) runs this workflow in full: %s, for example %s. In %s, docsOnly.paths adds what a docs-only change touches, docsOnly.inputs names a documentation file the build or the tests read, and docsOnly.exempt names a workflow that must run for documentation, each input and exemption with its reason",
			event, strings.Join(runs, ", "), why, docsFilterExample, record.Path)
		f, err := newFinding(DocsOnlyPush, file, t.line, 1, message)
		if err != nil {
			return nil, err
		}
		found = append(found, f)
	}
	finding.Sort(found)

	return found, nil
}

// checkDocsOnlyRecord is the check on the record's docsOnly inputs and exempt entries: a finding
// at each input that matches no tracked file, and at each exempt entry that names no tracked
// workflow or one the check would not report anyway. An entry that names nothing would narrow
// the check for a file added later without anyone deciding.
func checkDocsOnlyRecord(known func(id string) bool, recordFile string) ([]finding.Finding, error) {
	rec, err := record.Load(recordFile, known)
	if err != nil {
		return nil, err
	}
	d := rec.DocsOnly
	if len(d.Inputs) == 0 && len(d.Exempt) == 0 {
		return nil, nil
	}

	root, err := workingRoot()
	if err != nil {
		return nil, err
	}
	index, err := gitindex.Read(root)
	if err != nil {
		return nil, err
	}
	var tracked []string
	for _, entry := range index {
		if entry.IsRegular() {
			tracked = append(tracked, entry.Path)
		}
	}

	var found []finding.Finding
	report := func(line int, message string) error {
		f, err := newFinding(DocsOnlyPush, recordFile, max(line, 1), 1, message)
		if err == nil {
			found = append(found, f)
		}

		return err
	}

	for i, input := range d.Inputs {
		if slices.ContainsFunc(tracked, func(p string) bool { return glob.Match(input.Path, p) }) {
			continue
		}
		if err := report(input.Line, fmt.Sprintf("docsOnly input %d (%s) matches no tracked file; remove it, or name the documentation file the build or the tests read", i+1, input.Path)); err != nil {
			return nil, err
		}
	}

	for i, e := range d.Exempt {
		if !slices.Contains(tracked, e.Workflow) {
			if err := report(e.Line, fmt.Sprintf("docsOnly exempt entry %d names %s, and there is no tracked workflow there; remove it, or name the workflow that must run for documentation", i+1, e.Workflow)); err != nil {
				return nil, err
			}

			continue
		}
		would, err := docsOnlyFindings(rec, e.Workflow)
		if err != nil {
			return nil, err
		}
		if len(would) == 0 {
			if err := report(e.Line, fmt.Sprintf("docsOnly exempt entry %d (%s) excuses nothing: a docs-only change does not start that workflow; remove it", i+1, e.Workflow)); err != nil {
				return nil, err
			}
		}
	}
	finding.Sort(found)

	return found, nil
}

// defaultBranches are the branches a push to which is a push to main. The default branch of the
// repository is not read (dbf.json, push-concurrency-default-branch-is-main).
var defaultBranches = []string{"main"}

// perRunContexts are the expression contexts that differ in every run, so a concurrency group
// built from one never holds two runs and collapses nothing.
var perRunContexts = []string{"github.sha", "github.run_id", "github.run_number", "github.run_attempt", "github.event.after", "github.event.head_commit.id"}

// admitsDefaultBranch says whether a push trigger starts the workflow for a push to main.
func admitsDefaultBranch(config *yaml.Node) bool {
	if tagsOnly(config) {
		return false
	}
	branches, filtered := list(config, "branches")
	ignore, ignoring := list(config, "branches-ignore")
	for _, name := range defaultBranches {
		switch {
		case filtered && ghfilter.Included(branches, name):
			return true
		case !filtered && ignoring && !ghfilter.Ignored(ignore, name):
			return true
		case !filtered && !ignoring:
			return true
		}
	}

	return false
}

// groupOf is the group of a concurrency value: the value itself when it is a string, else its
// group key. The second result says whether a group is written.
func groupOf(concurrency *yaml.Node) (string, bool) {
	switch {
	case concurrency == nil:
		return "", false
	case concurrency.Kind == yaml.ScalarNode:
		return concurrency.Value, strings.TrimSpace(concurrency.Value) != ""
	}
	_, group := entry(concurrency, "group")
	if group == nil || group.Kind != yaml.ScalarNode || strings.TrimSpace(group.Value) == "" {
		return "", false
	}

	return group.Value, true
}

// perRunOnPushOnly matches, in a group with its whitespace removed, the expression that keys a
// pull request's runs on the first arm and every other event's run on the second:
// github.event_name == 'pull_request' && <shared> || <per run>. The first arm holds no ||
// and does not leave the expression.
var perRunOnPushOnly = regexp.MustCompile(`\$\{\{github\.event_name=='pull_request'&&((?:[^|}]|\}[^|}])+?)\|\|[^|}]+\}\}`)

// cancelOnPullRequestOnly is the cancel-in-progress, whitespace removed, that cancels a
// superseded run of a pull request and never a run of any other event.
const cancelOnPullRequestOnly = "${{github.event_name=='pull_request'}}"

// perRunOf names the per-run context a concurrency value's group is built from, or is empty.
// A per-run value in the fallback arm of perRunOnPushOnly is not counted when the value also
// cancels on pull_request alone: then a pull request's runs collapse and every push's run is
// built to the end, which is that workflow's stated choice.
func perRunOf(concurrency *yaml.Node, group string) string {
	compact := strings.Join(strings.Fields(group), "")
	_, cancel := entry(concurrency, "cancel-in-progress")
	if concurrency != nil && concurrency.Kind == yaml.MappingNode && cancel != nil && cancel.Kind == yaml.ScalarNode &&
		strings.Join(strings.Fields(cancel.Value), "") == cancelOnPullRequestOnly {
		compact = perRunOnPushOnly.ReplaceAllString(compact, "$1")
	}

	return perRun(compact)
}

// perRun names the per-run context a group is built from, or is empty.
func perRun(group string) string {
	compact := strings.Join(strings.Fields(group), "")
	for _, context := range perRunContexts {
		if strings.Contains(compact, context) {
			return context
		}
	}

	return ""
}

const perRunOnPushFix = "a run per push is kept only in the shape group: \"...${{ github.event_name == 'pull_request' && <shared value> || github.run_id }}\" with cancel-in-progress: \"${{ github.event_name == 'pull_request' }}\""

const concurrencyFix = "add at the top of the workflow concurrency: {group: \"${{ github.workflow }}-${{ github.ref }}\"}; cancel-in-progress stays this repository's choice (true cancels a superseded run, false lets the running one finish and keeps only the newest pending run)"

// CheckPushConcurrency is the check on one workflow triggered by a push to main: a finding when
// it has no concurrency group, or one built from a value that differs in every run, so each
// push to main runs to the end however many follow it. Jobs that each carry a group are
// accepted in place of a workflow-level one.
func CheckPushConcurrency(file string) ([]finding.Finding, error) {
	top, err := readWorkflow(file)
	if err != nil || top == nil {
		return nil, err
	}
	push, ok := triggers(top)["push"]
	if !ok || !admitsDefaultBranch(push.config) {
		return nil, nil
	}

	key, concurrency := entry(top, "concurrency")
	group, grouped := groupOf(concurrency)
	var message string
	line := push.line
	switch {
	case key == nil && everyJobHasAGroup(top):
		return nil, nil
	case key == nil:
		message = "pushes to main start this workflow and it has no concurrency, so every push runs to the end however many follow it; " + concurrencyFix
	case !grouped:
		message, line = "pushes to main start this workflow and its concurrency has no group, so nothing collapses; "+concurrencyFix, key.Line
	case perRunOf(concurrency, group) != "":
		message, line = fmt.Sprintf("pushes to main start this workflow and its concurrency group is built from %s, which differs in every run, so no two runs share it and nothing collapses; %s; %s", perRunOf(concurrency, group), concurrencyFix, perRunOnPushFix), key.Line
	default:
		return nil, nil
	}

	f, err := newFinding(PushConcurrency, file, line, 1, message)
	if err != nil {
		return nil, err
	}

	return []finding.Finding{f}, nil
}

// everyJobHasAGroup says whether every job of the workflow has its own concurrency group that
// is not built per run.
func everyJobHasAGroup(top *yaml.Node) bool {
	_, jobs := entry(top, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		return false
	}
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		_, concurrency := entry(jobs.Content[i+1], "concurrency")
		group, grouped := groupOf(concurrency)
		if !grouped || perRunOf(concurrency, group) != "" {
			return false
		}
	}

	return true
}

var (
	conclusionAllowed = regexp.MustCompile(`workflow_run\.conclusion\s*==\s*'|'[a-z_]*'\s*==\s*github\.event\.workflow_run\.conclusion|contains\(.*workflow_run\.conclusion`)
	conclusionDenied  = regexp.MustCompile(`workflow_run\.conclusion\s*!=|!=\s*github\.event\.workflow_run\.conclusion|!\s*contains\(.*workflow_run\.conclusion`)
	statusOverride    = regexp.MustCompile(`\b(always|cancelled)\(\)`)
)

// allowlisted says whether a job condition admits the watched run's conclusions by naming them,
// and names none it refuses: a denylist (!= 'success') admits cancelled and skipped.
func allowlisted(condition string) bool {
	return conclusionAllowed.MatchString(condition) && !conclusionDenied.MatchString(condition)
}

// CheckWorkflowRunFanout is the check on one workflow started by workflow_run on a completed
// run: a finding at each job that runs whatever the watched run's conclusion was. A job is
// gated when its if: compares the conclusion with the values it wants, or when it needs a job
// and does not override the skip it inherits with always() or cancelled().
func CheckWorkflowRunFanout(file string) ([]finding.Finding, error) {
	top, err := readWorkflow(file)
	if err != nil || top == nil {
		return nil, err
	}
	run, ok := triggers(top)["workflow_run"]
	if !ok {
		return nil, nil
	}
	if types, typed := list(run.config, "types"); typed && !slices.Contains(types, "completed") {
		return nil, nil
	}

	_, jobs := entry(top, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, nil
	}
	var found []finding.Finding
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		id, body := jobs.Content[i], resolve(jobs.Content[i+1])
		condition := ""
		if _, ifNode := entry(body, "if"); ifNode != nil && ifNode.Kind == yaml.ScalarNode {
			condition = ifNode.Value
		}
		needs, needing := list(body, "needs")
		inherits := needing && len(needs) > 0 && !statusOverride.MatchString(condition)
		if allowlisted(condition) || inherits {
			continue
		}

		message := fmt.Sprintf("job %s runs on every conclusion of the run it watches, cancelled and skipped included, so each superseded or cancelled run starts a runner for it. Gate it on the conclusions it is for, for example if: github.event.workflow_run.conclusion == 'failure'; if it must see every conclusion, record an exception in %s stating why", id.Value, record.Path)
		f, err := newFinding(WorkflowRunFanout, file, id.Line, id.Column, message)
		if err != nil {
			return nil, err
		}
		found = append(found, f)
	}
	finding.Sort(found)

	return found, nil
}

// CheckScheduleBudget is the check on one scheduled workflow: a finding when its cron schedules
// together start more runs a day than the record's scheduleBudget (hourly unless stated), and
// one at each cron this check cannot read, whose cadence is then unknown.
func CheckScheduleBudget(known func(id string) bool) func(file string) ([]finding.Finding, error) {
	return func(file string) ([]finding.Finding, error) {
		rec, err := loadRecord(known)
		if err != nil {
			return nil, err
		}
		top, err := readWorkflow(file)
		if err != nil || top == nil {
			return nil, err
		}
		schedule, ok := triggers(top)["schedule"]
		if !ok || schedule.config == nil || schedule.config.Kind != yaml.SequenceNode {
			return nil, nil
		}

		var found []finding.Finding
		total, first := 0, 0
		for _, item := range schedule.config.Content {
			_, cron := entry(item, "cron")
			if cron == nil || cron.Kind != yaml.ScalarNode {
				continue
			}
			if first == 0 {
				first = cron.Line
			}
			runs, readable := runsPerDay(cron.Value)
			if !readable {
				message := fmt.Sprintf("the cron %q cannot be read as five numeric fields, so how often it runs is unknown; write it with numbers, ranges, steps and lists, or record an exception in %s", cron.Value, record.Path)
				f, err := newFinding(ScheduleBudget, file, cron.Line, cron.Column, message)
				if err != nil {
					return nil, err
				}
				found = append(found, f)

				continue
			}
			total += runs
		}

		budget := rec.ScheduleBudget.EffectiveMaxRunsPerDay()
		if total > budget {
			message := fmt.Sprintf("the schedule starts %d runs a day, over the budget of %d; run it less often (17 * * * * is 24 a day, 41 0-23/6 * * * is 4), or, if it must run this often, record an exception in %s stating why", total, budget, record.Path)
			f, err := newFinding(ScheduleBudget, file, first, 1, message)
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}
		finding.Sort(found)

		return found, nil
	}
}

// cronFields are the bounds of the five fields of a cron expression, and the names the month
// and the day of the week may be written with.
var cronFields = []struct {
	low, high int
	names     []string
}{
	{0, 59, nil},
	{0, 23, nil},
	{1, 31, nil},
	{1, 12, []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}},
	{0, 6, []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}},
}

// runsPerDay is the most runs a cron expression starts in a day: its minutes times its hours,
// on a day it runs at all. The second result is false for an expression it cannot read.
func runsPerDay(cron string) (int, bool) {
	fields := strings.Fields(cron)
	if len(fields) != len(cronFields) {
		return 0, false
	}

	counts := make([]int, len(fields))
	for i, field := range fields {
		n, readable := cronValues(field, cronFields[i].low, cronFields[i].high, cronFields[i].names)
		if !readable {
			return 0, false
		}
		counts[i] = n
	}

	return counts[0] * counts[1], true
}

// cronValues counts the distinct values one cron field selects.
func cronValues(field string, low, high int, names []string) (int, bool) {
	selected := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		base, step := part, 1
		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			n, err := strconv.Atoi(part[slash+1:])
			if err != nil || n < 1 {
				return 0, false
			}
			base, step = part[:slash], n
		}

		from, to, readable := cronRange(base, step, low, high, names)
		if !readable {
			return 0, false
		}
		for v := from; v <= to; v += step {
			selected[v] = true
		}
	}

	return len(selected), true
}

// cronRange is the first and last value of one part of a field before its step: all of them for
// `*`, a range as written, and for a single value with a step, that value to the end.
func cronRange(base string, step, low, high int, names []string) (int, int, bool) {
	if base == "*" {
		return low, high, true
	}
	if a, b, isRange := strings.Cut(base, "-"); isRange {
		from, okFrom := cronNumber(a, low, high, names)
		to, okTo := cronNumber(b, low, high, names)

		return from, to, okFrom && okTo && from <= to
	}
	n, ok := cronNumber(base, low, high, names)
	if step > 1 {
		return n, high, ok
	}

	return n, n, ok
}

// cronNumber reads one value of a field, a number or a name, within its bounds.
func cronNumber(text string, low, high int, names []string) (int, bool) {
	for i, name := range names {
		if strings.EqualFold(text, name) {
			return low + i, true
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < low || n > high {
		return 0, false
	}

	return n, true
}
