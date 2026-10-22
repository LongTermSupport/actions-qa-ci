package firstparty

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
	"github.com/LongTermSupport/actions-qa-ci/internal/scope"
)

// LintCoverage is the identifier of the check that every tracked file that looks like a
// workflow or an action is analysed or declared not to be.
const LintCoverage = finding.ToolPrefix + ".lintCoverage"

// shape is what a YAML file looks like it is.
type shape int

const (
	shapeNone shape = iota
	shapeWorkflow
	shapeAction
)

// CheckLintCoverage is the check on a project: a finding for each tracked file that is
// workflow-shaped (a top-level `on` and `jobs`) or action-shaped (an action.yml with a top-level
// `runs`) and that is neither in the default set nor declared in analyse.unanalysed of the
// project record, and for each declaration that excuses no such file. The file at the path is
// the project record, which may be absent: the check is about the project, not about it.
func CheckLintCoverage(known func(id string) bool) func(recordFile string) ([]finding.Finding, error) {
	return func(recordFile string) ([]finding.Finding, error) {
		rec, err := record.Load(recordFile, known)
		if errors.Is(err, fs.ErrNotExist) {
			rec, err = &record.Record{}, nil
		}
		if err != nil {
			return nil, err
		}

		root, err := workingRoot()
		if err != nil {
			return nil, err
		}
		tracked, err := gitindex.Read(root)
		if err != nil {
			return nil, err
		}

		resolution, err := scope.Resolve(root, tracked, rec.Analyse.Include)
		if err != nil {
			return nil, err
		}
		analysed := resolution.Included.Files()

		unanalysed := rec.Analyse.Unanalysed
		used := make([]bool, len(unanalysed))

		var found []finding.Finding
		for i, inc := range rec.Analyse.Include {
			if resolution.Used[i] {
				continue
			}
			message := fmt.Sprintf("include entry %d (%s) includes no YAML file that is tracked and not analysed anyway; remove it: a pattern that names nothing would pull in a file added later without anyone deciding", i+1, inc.Path)
			f, err := newFinding(LintCoverage, recordFile, max(inc.Line, 1), 1, message)
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}

		for _, entry := range tracked {
			if !entry.IsRegular() || !examined(entry.Path) || slices.Contains(analysed, entry.Path) {
				continue
			}
			data, problem, err := readTemplateFile(root, entry.Path)
			if err != nil {
				return nil, err
			}
			if problem != "" {
				continue // missing, not a plain file, or too large: nothing there to examine
			}

			kind, line := shapeOf(entry.Path, data)
			if kind == shapeNone || check.IsWorkflow(entry.Path) || check.IsActionFile(entry.Path) {
				continue
			}

			excused := false
			for i, u := range unanalysed {
				if glob.Match(u.Path, entry.Path) {
					used[i], excused = true, true
				}
			}
			if excused {
				continue
			}

			f, err := uncoveredFinding(entry.Path, kind, line)
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}

		for i, u := range unanalysed {
			if used[i] {
				continue
			}
			message := fmt.Sprintf("unanalysed entry %d (%s) excuses no workflow-shaped or action-shaped file that is tracked and not analysed anyway; remove it: a pattern that names nothing would excuse a file added later without anyone deciding", i+1, u.Path)
			f, err := newFinding(LintCoverage, recordFile, max(u.Line, 1), 1, message)
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}
		finding.Sort(found)

		return found, nil
	}
}

// examined says whether a tracked path is one that might be a workflow or an action: a YAML file.
func examined(file string) bool {
	return strings.HasSuffix(file, ".yml") || strings.HasSuffix(file, ".yaml")
}

func uncoveredFinding(file string, kind shape, line int) (finding.Finding, error) {
	if strings.ContainsFunc(file, finding.IsControlOrSeparator) {
		return finding.Finding{}, fmt.Errorf("%w %s: its name has a character that cannot be printed on a line", check.ErrNotAnalysable, finding.Escape(file))
	}

	var message string
	switch kind {
	case shapeWorkflow:
		message = fmt.Sprintf("%s is workflow-shaped (a top-level on and jobs) but no lane analyses it, so nothing in it is checked: declare it under analyse.unanalysed in the project record with the hazard and scope that make that acceptable, or move it to .github/workflows", file)
	default:
		message = fmt.Sprintf("%s is action-shaped (an action.yml with a top-level runs) but no lane analyses it, so nothing in it is checked: declare it under analyse.unanalysed in the project record with the hazard and scope that make that acceptable, or move it under .github/actions", file)
	}

	return newFinding(LintCoverage, file, line, 1, message)
}

// shapeOf says what a YAML file looks like, and the line of the key that made it so. Anything
// that is not YAML, or not a mapping, looks like nothing.
func shapeOf(file string, data []byte) (shape, int) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return shapeNone, 0
	}

	top := document.Content[0]
	keys := map[string]*yaml.Node{}
	for i := 0; i+1 < len(top.Content); i += 2 {
		keys[top.Content[i].Value] = top.Content[i]
	}

	jobs := valueOf(top, "jobs")
	switch {
	case keys["on"] != nil && keys["jobs"] != nil && jobs != nil && jobs.Kind == yaml.MappingNode:
		return shapeWorkflow, keys["jobs"].Line
	case slices.Contains([]string{"action.yml", "action.yaml"}, path.Base(file)) && keys["runs"] != nil:
		return shapeAction, keys["runs"].Line
	}

	return shapeNone, 0
}

// lintCoverageRule is the registry entry of the check.
func lintCoverageRule(known func(id string) bool) registry.Rule {
	return registry.Rule{
		ID:      LintCoverage,
		Summary: "Every git-tracked file that is workflow-shaped or action-shaped is analysed, or declared unanalysed with a justification",
		Check:   CheckLintCoverage(known),
	}
}
