// Package check decides which files an analysis covers and runs the lanes over them. It holds
// no rule of its own: a lane is handed in, and a failure of any lane stops the run, because a
// run that skipped something has no verdict to give.
package check

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Lane is one kind of analysis: the files it covers and how it runs on one of them.
type Lane struct {
	// Name is how a person asks for the lane alone.
	Name string
	// Selects says whether the lane analyses the file at this repository-relative path.
	Selects func(path string) bool
	// Run analyses one file and returns what it found.
	Run func(path string) ([]finding.Finding, error)
}

// LaneError is a lane failing on a file. It is a fault of the toolchain or of the file's
// readability, never a finding.
type LaneError struct {
	Lane string
	Path string
	Err  error
}

func (e *LaneError) Error() string {
	return fmt.Sprintf("lane %s failed on %s: %v", e.Lane, e.Path, e.Err)
}

func (e *LaneError) Unwrap() error { return e.Err }

// Run gives each lane, in the order given, the files it selects, in path order and each once,
// and returns everything they found, sorted. The first failure stops the run and no findings
// are returned: a partial list would read as a verdict.
func Run(lanes []Lane, files []string) ([]finding.Finding, error) {
	if err := validate(lanes); err != nil {
		return nil, err
	}

	ordered := slices.Clone(files)
	slices.Sort(ordered)
	ordered = slices.Compact(ordered)

	var all []finding.Finding
	for _, lane := range lanes {
		for _, file := range ordered {
			if !lane.Selects(file) {
				continue
			}
			found, err := lane.Run(file)
			if err != nil {
				return nil, &LaneError{Lane: lane.Name, Path: file, Err: err}
			}
			all = append(all, found...)
		}
	}
	finding.Sort(all)

	return all, nil
}

func validate(lanes []Lane) error {
	seen := map[string]bool{}
	for _, lane := range lanes {
		switch {
		case strings.TrimSpace(lane.Name) == "":
			return errors.New("a lane has no name")
		case lane.Selects == nil:
			return fmt.Errorf("lane %s cannot say which files it analyses", lane.Name)
		case lane.Run == nil:
			return fmt.Errorf("lane %s cannot run", lane.Name)
		case seen[lane.Name]:
			return fmt.Errorf("lane %s is defined twice", lane.Name)
		}
		seen[lane.Name] = true
	}

	return nil
}

// Selecting returns the lanes that analyse the file at this path, in order.
func Selecting(lanes []Lane, path string) []Lane {
	var selected []Lane
	for _, lane := range lanes {
		if lane.Selects(path) {
			selected = append(selected, lane)
		}
	}

	return selected
}

// ErrNotAnalysable marks a file in the default set that cannot be read as a plain file. It is
// refused rather than skipped, because a skipped file is a file nobody checked.
var ErrNotAnalysable = errors.New("cannot analyse")

var yamlName = regexp.MustCompile(`\.ya?ml$`)

// Discover lists the files analysed by default, repository-relative and sorted: the workflows
// in .github/workflows (GitHub reads only that level), every action.yml or action.yaml under
// .github/actions and at the root, .github/dependabot.yml and the project record. A symlink
// or any other thing that is not a plain file, where the set looks, is an error.
func Discover(fsys fs.FS) ([]string, error) {
	var found []string

	add := func(file string, entry fs.DirEntry) error {
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w %s: it is not a regular file", ErrNotAnalysable, file)
		}
		found = append(found, file)

		return nil
	}

	rootEntries, err := entries(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, entry := range rootEntries {
		switch {
		case entry.Name() == ".github" && !entry.IsDir():
			return nil, fmt.Errorf("%w .github: it is not a directory", ErrNotAnalysable)
		case isActionName(entry.Name()):
			if err := add(entry.Name(), entry); err != nil {
				return nil, err
			}
		}
	}

	githubEntries, err := entries(fsys, ".github")
	if err != nil {
		return nil, err
	}
	for _, entry := range githubEntries {
		file := ".github/" + entry.Name()
		switch entry.Name() {
		case "workflows", "actions":
			if !entry.IsDir() {
				return nil, fmt.Errorf("%w %s: it is not a directory", ErrNotAnalysable, file)
			}
		case "dependabot.yml", "dependabot.yaml", "actions-qa-ci.yml":
			if err := add(file, entry); err != nil {
				return nil, err
			}
		}
	}

	workflows, err := entries(fsys, ".github/workflows")
	if err != nil {
		return nil, err
	}
	for _, entry := range workflows {
		file := ".github/workflows/" + entry.Name()
		if entry.Type()&fs.ModeSymlink != 0 || yamlName.MatchString(entry.Name()) {
			if err := add(file, entry); err != nil {
				return nil, err
			}
		}
	}

	if err := discoverActions(fsys, add); err != nil {
		return nil, err
	}

	slices.Sort(found)

	return slices.Compact(found), nil
}

func discoverActions(fsys fs.FS, add func(string, fs.DirEntry) error) error {
	err := fs.WalkDir(fsys, ".github/actions", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && file == ".github/actions" {
				return nil
			}

			return err
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w %s: a symbolic link under .github/actions", ErrNotAnalysable, file)
		case entry.IsDir():
			return nil
		case isActionName(path.Base(file)):
			return add(file, entry)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("reading .github/actions: %w", err)
	}

	return nil
}

func isActionName(name string) bool {
	return name == "action.yml" || name == "action.yaml"
}

// entries lists a directory, and nothing for one that does not exist.
func entries(fsys fs.FS, dir string) ([]fs.DirEntry, error) {
	list, err := fs.ReadDir(fsys, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	return list, nil
}
