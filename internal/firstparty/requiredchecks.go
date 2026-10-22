package firstparty

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// RequiredChecks is the identifier of the check that every status check the record requires is
// one that a job of a pull request workflow produces.
const RequiredChecks = finding.ToolPrefix + ".requiredChecks"

const workflowDirectory = ".github/workflows"

// job is one job of a workflow, as far as the name of its check run goes.
type job struct {
	workflow    string
	id          string
	name        string // as written; empty when the job has no name
	pullRequest bool   // its workflow is triggered on pull_request
	expression  bool   // its name holds an expression
	matrix      bool   // it runs a matrix, so each check run's name has a suffix
	reusable    bool   // it calls a reusable workflow, so its check runs are named under its own name
}

// label is what the job's check run is called before any suffix: its name, else its id.
func (j job) label() string {
	if j.name != "" {
		return j.name
	}

	return j.id
}

// produces says whether a required context is one of this job's check runs.
func (j job) produces(context string) bool {
	switch {
	case j.expression, j.matrix:
		return false
	case j.reusable:
		return strings.HasPrefix(context, j.label()+" / ")
	}

	return context == j.label()
}

// CheckRequiredChecks is the check on the project record at a path: a finding at the record's
// entry for each required status check that no job of a pull request workflow produces, with
// the reason when a job nearly does. It reads the workflows directly in .github/workflows, as
// GitHub does. A workflow it cannot read is a finding at that workflow, since the checks it
// produces are then unknown. Comparing with the branch protection a repository has is not done
// here (see the known gap required-checks-online-comparison).
func CheckRequiredChecks(known func(id string) bool) func(path string) ([]finding.Finding, error) {
	return func(recordFile string) ([]finding.Finding, error) {
		rec, err := record.Load(recordFile, known)
		if err != nil {
			return nil, err
		}
		if len(rec.RequiredChecks) == 0 {
			return nil, nil
		}

		root, err := workingRoot()
		if err != nil {
			return nil, err
		}

		jobs, unread, err := workflowJobs(root)
		if err != nil {
			return nil, err
		}

		var found []finding.Finding
		for workflow, problem := range unread {
			f, err := newFinding(RequiredChecks, workflow, 1, 1, fmt.Sprintf("%s %s, so the checks it produces are unknown", workflow, problem))
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}

		for _, required := range rec.RequiredChecks {
			if reason := unproducedBecause(required.Context, jobs); reason != "" {
				message := fmt.Sprintf("the required check %q on branch %s: %s", required.Context, required.EffectiveBranch(), reason)
				f, err := newFinding(RequiredChecks, recordFile, required.Line, 1, message)
				if err != nil {
					return nil, err
				}
				found = append(found, f)
			}
		}
		finding.Sort(found)

		return found, nil
	}
}

// unproducedBecause is why no job of a pull request workflow produces the context, or "" when
// one does.
func unproducedBecause(context string, jobs []job) string {
	var notPullRequest, expression, matrix []string
	for _, j := range jobs {
		switch {
		case j.pullRequest && j.produces(context):
			return ""
		case !j.pullRequest && !j.expression && !j.matrix && j.produces(context):
			notPullRequest = append(notPullRequest, fmt.Sprintf("job %s in %s", j.id, j.workflow))
		case j.pullRequest && j.expression && (j.id == context || j.name == context):
			expression = append(expression, fmt.Sprintf("job %s in %s", j.id, j.workflow))
		case j.pullRequest && j.matrix && j.label() == context:
			matrix = append(matrix, fmt.Sprintf("job %s in %s", j.id, j.workflow))
		}
	}

	switch {
	case len(expression) > 0:
		return strings.Join(expression, ", ") + " has an expression in its name, so its check run is not called that and the name cannot be a stable required check"
	case len(matrix) > 0:
		return strings.Join(matrix, ", ") + " runs a matrix, so each of its check runs carries a suffix and the plain name is never one"
	case len(notPullRequest) > 0:
		return strings.Join(notPullRequest, ", ") + " would produce it, but its workflow is not triggered on pull_request"
	}

	return "no job in " + workflowDirectory + " produces it"
}

// workflowJobs reads the jobs of every workflow directly in .github/workflows. The second
// result names the workflows that could not be read, with why.
func workflowJobs(root string) ([]job, map[string]string, error) {
	entries, err := os.ReadDir(filepath.FromSlash(workflowDirectory))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot list %s: %w", workflowDirectory, err)
	}

	var jobs []job
	unread := map[string]string{}
	for _, entry := range entries {
		name := path.Join(workflowDirectory, entry.Name())
		if !check.IsWorkflow(name) {
			continue
		}

		data, problem, err := readTemplateFile(root, name)
		if err != nil {
			return nil, nil, err
		}
		if problem != "" {
			unread[name] = problem

			continue
		}

		read, err := jobsOf(name, data)
		if err != nil {
			unread[name] = "is not valid YAML or not a workflow (" + err.Error() + ")"

			continue
		}
		jobs = append(jobs, read...)
	}

	return jobs, unread, nil
}

// jobsOf reads the jobs of one workflow.
func jobsOf(workflow string, data []byte) ([]job, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, errors.New(strings.SplitN(err.Error(), "\n", 2)[0])
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}

	top := document.Content[0]
	pullRequest := triggersOnPullRequest(valueOf(top, "on"))

	var jobs []job
	jobsNode := valueOf(top, "jobs")
	if jobsNode == nil || jobsNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(jobsNode.Content); i += 2 {
		body := resolve(jobsNode.Content[i+1])
		j := job{workflow: workflow, id: jobsNode.Content[i].Value, pullRequest: pullRequest}
		if body.Kind == yaml.MappingNode {
			if name := valueOf(body, "name"); name != nil && name.Kind == yaml.ScalarNode {
				j.name = name.Value
				j.expression = strings.Contains(name.Value, "${{")
			}
			if strategy := valueOf(body, "strategy"); strategy != nil && strategy.Kind == yaml.MappingNode && valueOf(strategy, "matrix") != nil {
				j.matrix = true
			}
			j.reusable = valueOf(body, "uses") != nil
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

// triggersOnPullRequest reads the value of a workflow's `on` key.
func triggersOnPullRequest(on *yaml.Node) bool {
	on = resolve(on)
	if on == nil {
		return false
	}

	switch on.Kind {
	case yaml.ScalarNode:
		return on.Value == "pull_request"
	case yaml.SequenceNode:
		for _, item := range on.Content {
			if resolve(item).Value == "pull_request" {
				return true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			if on.Content[i].Value == "pull_request" {
				return true
			}
		}
	}

	return false
}

// valueOf is the value of a key of a mapping, or nil.
func valueOf(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// resolve follows an alias to the node it stands for.
func resolve(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}

	return node
}
