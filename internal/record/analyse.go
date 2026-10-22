package record

import (
	"fmt"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
)

// Analyse says what the project's workflow-shaped files are, beyond the default set.
type Analyse struct {
	// Include are patterns of tracked YAML files outside the default set that the project
	// asks to have analysed.
	Include []Included `yaml:"include"`
	// Unanalysed are workflow-shaped files the project deliberately does not analyse.
	Unanalysed []Unanalysed `yaml:"unanalysed"`
}

// analyseDocument is Analyse as the decoder reads it, with entries that can be null.
type analyseDocument struct {
	Include    []*string     `yaml:"include"`
	Unanalysed []*Unanalysed `yaml:"unanalysed"`
}

// Included is a pattern of paths of files to analyse, beyond the default set.
type Included struct {
	// Path is a pattern (see package glob) of repository-relative paths.
	Path string

	// Line is where the entry is in the record file.
	Line int

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// Justification is why a file is left out of the analysis.
type Justification struct {
	// Hazard says what a workflow-shaped file there would be checked for, and Scope why leaving
	// it unchecked is accepted only here.
	Hazard string `yaml:"hazard"`
	Scope  string `yaml:"scope"`
}

// Unanalysed is a pattern of paths that are workflow-shaped and deliberately not analysed, with
// the reason.
type Unanalysed struct {
	// Path is a pattern (see package glob) of repository-relative paths.
	Path          string        `yaml:"path"`
	Justification Justification `yaml:"justification"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// lintCoverageID is the identifier of the defence an unanalysed entry turns off for the files
// it names; the justification may not only repeat it.
const lintCoverageID = finding.ToolPrefix + ".lintCoverage"

// Faults lists what is wrong with the entry's justification, at most one fault for each of
// hazard and scope. The standard is the one the exceptions are held to.
func (u Unanalysed) Faults() []Fault {
	return hazardScopeFaults(u.Justification.Hazard, u.Justification.Scope, lintCoverageID)
}

// analyseFaults judges the unanalysed section as a whole: each entry names a pattern that can be
// used, and no pattern is listed twice.
func analyseFaults(entries []Unanalysed) []string {
	var faults []string

	listed := map[string]bool{}
	for i, u := range entries {
		if u.null {
			continue // reported once, by Parse
		}
		where := fmt.Sprintf("unanalysed entry %d (line %d)", i+1, u.Line)

		if u.Path == "" {
			faults = append(faults, where+": path is missing")

			continue
		}
		if err := glob.Check(u.Path); err != nil {
			faults = append(faults, fmt.Sprintf("%s: path %q %s", where, u.Path, err))

			continue
		}
		if listed[u.Path] {
			faults = append(faults, fmt.Sprintf("%s: path %q is listed twice", where, u.Path))
		}
		listed[u.Path] = true
	}

	return faults
}

// includeFaults judges the include section as a whole: each entry names a pattern that can be
// used, and no pattern is listed twice.
func includeFaults(entries []Included) []string {
	var faults []string

	listed := map[string]bool{}
	for i, inc := range entries {
		if inc.null {
			continue // reported once, by Parse
		}
		where := fmt.Sprintf("include entry %d (line %d)", i+1, inc.Line)

		if inc.Path == "" {
			faults = append(faults, where+": path is missing")

			continue
		}
		if err := glob.Check(inc.Path); err != nil {
			faults = append(faults, fmt.Sprintf("%s: path %q %s", where, inc.Path, err))

			continue
		}
		if listed[inc.Path] {
			faults = append(faults, fmt.Sprintf("%s: path %q is listed twice", where, inc.Path))
		}
		listed[inc.Path] = true
	}

	return faults
}

// writtenEntries is the lines of the entries of a list under analyse (include or unanalysed) as
// written.
func writtenEntries(analyse *yaml.Node, key string) []int {
	var lines []int
	if analyse.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(analyse.Content); i += 2 {
		if analyse.Content[i].Value != key || analyse.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range analyse.Content[i+1].Content {
			lines = append(lines, item.Line)
		}
	}

	return lines
}
