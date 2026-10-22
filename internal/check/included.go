package check

import (
	"slices"
)

// Coverage is the kinds of included file a lane analyses beyond what it selects by itself.
type Coverage uint8

const (
	// Workflows: a lane that analyses an included file that is a workflow.
	Workflows Coverage = 1 << iota
	// Actions: a lane that analyses an included file that is a composite action.
	Actions
)

// Included is the files the project record asks to have analysed beyond the default set,
// repository-relative: those that are analysed as workflows, and those that are actions.
type Included struct {
	Workflows []string
	Actions   []string
}

// Files lists every included file, sorted.
func (i Included) Files() []string {
	files := slices.Concat(i.Workflows, i.Actions)
	slices.Sort(files)

	return files
}

// Widen returns the lanes with each one that covers a kind of included file also selecting the
// included files of that kind. The lanes given are left as they were.
func Widen(lanes []Lane, included Included) []Lane {
	widened := make([]Lane, len(lanes))
	for i, lane := range lanes {
		widened[i] = lane

		var extra []string
		if lane.Covers&Workflows != 0 {
			extra = append(extra, included.Workflows...)
		}
		if lane.Covers&Actions != 0 {
			extra = append(extra, included.Actions...)
		}
		if len(extra) == 0 {
			continue
		}

		own := lane.Selects
		widened[i].Selects = func(path string) bool { return own(path) || slices.Contains(extra, path) }
	}

	return widened
}

// defaultSingles are the files analysed by default that are named, whatever directory they
// might be looked for in.
var defaultSingles = []string{
	".github/dependabot.yml",
	".github/dependabot.yaml",
	".github/actions-qa-ci.yml",
}

// IsDefaultFile says whether a repository-relative path is a file Discover finds by itself.
func IsDefaultFile(file string) bool {
	return IsWorkflow(file) || IsActionFile(file) || IsNativeSuppressionConfig(file) || slices.Contains(defaultSingles, file)
}
