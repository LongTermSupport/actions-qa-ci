// Package locate says which job and which step of a workflow a line of its file is in, so that
// an exception in the project record can name a place by the job and step a person sees rather
// than by a line number that drifts on every edit.
//
// It places a line only when it can do so without guessing. A workflow with an alias, jobs that
// share a line, or steps that share a line cannot be told apart by line, and then the part it
// cannot tell is left unplaced: an exception that names it matches nothing, and the finding
// stays reported.
package locate

import (
	"bytes"

	"go.yaml.in/yaml/v4"
)

// Location is the job and step a line is in. A job or step is named by its id, its name, or
// both; any part that is not there, or is not text, is empty.
type Location struct {
	Job      string
	JobName  string
	Step     string
	StepName string
}

// Workflow is a workflow file read for the extent of each of its jobs and steps. The zero
// value places nothing.
type Workflow struct {
	jobs []job
}

type job struct {
	id, name   string
	start, end int
	steps      []step
}

type step struct {
	id, name   string
	start, end int
}

// Find reads data and locates a 1-based line of it.
func Find(data []byte, line int) Location {
	return Parse(data).Find(line)
}

// Parse reads a workflow file as YAML. A file that is not one it can read, or that it cannot
// place without guessing, gives a Workflow that places nothing.
func Parse(data []byte) *Workflow {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode || hasAlias(&root) {
		return &Workflow{}
	}

	last := lineCount(data)
	top := root.Content[0]
	index := keyIndex(top, "jobs")
	if index < 0 || top.Content[index+1].Kind != yaml.MappingNode {
		return &Workflow{}
	}
	jobs := top.Content[index+1]
	jobsLimit := nextLine(top, index, last+1)

	workflow := &Workflow{}
	seen := map[int]bool{}
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		key, body := jobs.Content[i], jobs.Content[i+1]
		if seen[key.Line] {
			return &Workflow{}
		}
		seen[key.Line] = true

		parsed := job{id: text(key), start: key.Line, end: nextLine(jobs, i, jobsLimit) - 1}
		if body.Kind == yaml.MappingNode {
			parsed.name = text(value(body, "name"))
			parsed.steps = steps(body, parsed.end+1)
		}
		workflow.jobs = append(workflow.jobs, parsed)
	}

	return workflow
}

// steps reads the steps of a job whose mapping is body and which ends before jobLimit.
func steps(body *yaml.Node, jobLimit int) []step {
	index := keyIndex(body, "steps")
	if index < 0 || body.Content[index+1].Kind != yaml.SequenceNode {
		return nil
	}
	list := body.Content[index+1]
	limit := nextLine(body, index, jobLimit)

	var parsed []step
	seen := map[int]bool{}
	for i, item := range list.Content {
		if seen[item.Line] {
			return nil
		}
		seen[item.Line] = true

		end := limit - 1
		if i+1 < len(list.Content) {
			end = list.Content[i+1].Line - 1
		}
		current := step{start: item.Line, end: end}
		if item.Kind == yaml.MappingNode {
			current.id = text(value(item, "id"))
			current.name = text(value(item, "name"))
		}
		parsed = append(parsed, current)
	}

	return parsed
}

// Find locates a 1-based line. A line outside every job has no location, and a line in a job but
// in no step (or in a step that cannot be told from another) has the job alone.
func (w *Workflow) Find(line int) Location {
	j, ok := w.jobAt(line)
	if !ok {
		return Location{}
	}

	found := Location{Job: j.id, JobName: j.name}
	if s, ok := j.stepAt(line); ok {
		found.Step, found.StepName = s.id, s.name
	}

	return found
}

// Within says whether a line is in the job and step an exception names; either may be empty,
// meaning any. A job or step is named by its id, and only when no job (or, in the job, no step)
// has that id by its name: an id that is also somebody's name means the job it is the id of. A
// name that two jobs (or two steps of the job) share names none of them, so the locator has to
// use the id.
func (w *Workflow) Within(line int, jobWanted, stepWanted string) bool {
	j, ok := w.jobAt(line)
	if !ok || !named(jobWanted, j.id, j.name, w.jobLabels()) {
		return false
	}
	if stepWanted == "" {
		return true
	}

	s, ok := j.stepAt(line)

	return ok && named(stepWanted, s.id, s.name, j.stepLabels())
}

func (w *Workflow) jobAt(line int) (job, bool) {
	for _, j := range w.jobs {
		if line >= j.start && line <= j.end {
			return j, true
		}
	}

	return job{}, false
}

func (j job) stepAt(line int) (step, bool) {
	for _, s := range j.steps {
		if line >= s.start && line <= s.end {
			return s, true
		}
	}

	return step{}, false
}

// label is the id and name of a job or step, for deciding what a locator names.
type label struct{ id, name string }

func (w *Workflow) jobLabels() []label {
	labels := make([]label, 0, len(w.jobs))
	for _, j := range w.jobs {
		labels = append(labels, label{j.id, j.name})
	}

	return labels
}

func (j job) stepLabels() []label {
	labels := make([]label, 0, len(j.steps))
	for _, s := range j.steps {
		labels = append(labels, label{s.id, s.name})
	}

	return labels
}

// named says whether a locator names the thing with this id and name among all of them.
func named(wanted, id, name string, all []label) bool {
	if wanted == "" {
		return true
	}

	for _, other := range all {
		if other.id == wanted {
			return id == wanted
		}
	}

	if name != wanted {
		return false
	}
	sharing := 0
	for _, other := range all {
		if other.name == wanted {
			sharing++
		}
	}

	return sharing == 1
}

// keyIndex is the index in a mapping's content of the key with this text, or -1.
func keyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return i
		}
	}

	return -1
}

// nextLine is the line of the key after the one at index in a mapping, or fallback when it is
// the last: the point where what that key holds ends.
func nextLine(mapping *yaml.Node, index, fallback int) int {
	if index+2 < len(mapping.Content) {
		return mapping.Content[index+2].Line
	}

	return fallback
}

// value is the node a mapping holds under a key, or nil.
func value(mapping *yaml.Node, key string) *yaml.Node {
	index := keyIndex(mapping, key)
	if index < 0 {
		return nil
	}

	return mapping.Content[index+1]
}

// text is a scalar's text, and empty for anything else.
func text(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}

	return node.Value
}

// hasAlias says whether an alias appears anywhere in the document.
func hasAlias(node *yaml.Node) bool {
	if node.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range node.Content {
		if hasAlias(child) {
			return true
		}
	}

	return false
}

// lineCount is the number of the last line of the file, a line ending being a line feed, a
// carriage return or both.
func lineCount(data []byte) int {
	lines := 0
	for i, b := range data {
		if b == '\n' || (b == '\r' && (i+1 >= len(data) || data[i+1] != '\n')) {
			lines++
		}
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) && !bytes.HasSuffix(data, []byte("\r")) {
		lines++
	}

	return lines
}
