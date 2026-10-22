// Package locate says which job and which step of a workflow a line of its file is in, so that
// an exception in the project record can name a place by the job and step a person sees rather
// than by a line number that drifts on every edit.
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

// Find locates a 1-based line of a workflow file. It reads the file as YAML without following
// an alias, and says nothing (the zero Location) for a line outside every job and for a file
// it cannot read as a workflow: an exception that names a job or step then matches nothing,
// so a finding it cannot place is never excepted.
func Find(data []byte, line int) Location {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return Location{}
	}

	jobs := value(root.Content[0], "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return Location{}
	}

	last := lineCount(data)
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		key, job := jobs.Content[i], jobs.Content[i+1]
		end := last
		if i+2 < len(jobs.Content) {
			end = jobs.Content[i+2].Line - 1
		}
		if line < key.Line || line > end {
			continue
		}

		found := Location{Job: text(key)}
		if job.Kind != yaml.MappingNode {
			return found
		}
		found.JobName = text(value(job, "name"))
		found.Step, found.StepName = step(value(job, "steps"), line, end)

		return found
	}

	return Location{}
}

// step names the step of a job's steps that holds the line, whose job ends at jobEnd.
func step(steps *yaml.Node, line, jobEnd int) (id, name string) {
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return "", ""
	}

	for i, item := range steps.Content {
		end := jobEnd
		if i+1 < len(steps.Content) {
			end = steps.Content[i+1].Line - 1
		}
		if line < item.Line || line > end {
			continue
		}
		if item.Kind != yaml.MappingNode {
			return "", ""
		}

		return text(value(item, "id")), text(value(item, "name"))
	}

	return "", ""
}

// value is the node a mapping holds under a key, or nil.
func value(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// text is a scalar's text, and empty for anything else.
func text(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}

	return node.Value
}

// lineCount is the number of the last line of the file.
func lineCount(data []byte) int {
	lines := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}

	return lines
}
