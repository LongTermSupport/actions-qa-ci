// Package scope resolves which files beyond the default set a project record asks to have
// analysed (analyse.include): the tracked YAML files its patterns name. A file asked for that
// cannot be analysed is an error, not a skip, because a skipped file is a file nobody checked.
package scope

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// Resolution is what the include patterns of a record come to.
type Resolution struct {
	// Included is the files to analyse beyond the default set.
	Included check.Included
	// Used says, for each pattern in the order given, whether it named any such file.
	Used []bool
}

// Resolve lists the files the patterns name: tracked, regular, YAML, and not analysed by default.
// A file named action.yml or action.yaml is an action and every other is a workflow. root is the
// repository root the paths are looked up under.
func Resolve(root string, tracked []gitindex.Entry, includes []record.Included) (Resolution, error) {
	resolution := Resolution{Used: make([]bool, len(includes))}

	for _, entry := range tracked {
		if !isYAML(entry.Path) || check.IsDefaultFile(entry.Path) {
			continue
		}

		named := false
		for i, inc := range includes {
			if inc.Path != "" && glob.Match(inc.Path, entry.Path) {
				resolution.Used[i], named = true, true
			}
		}
		if !named {
			continue
		}

		if err := analysable(root, entry); err != nil {
			return Resolution{}, err
		}
		if base := path.Base(entry.Path); base == "action.yml" || base == "action.yaml" {
			resolution.Included.Actions = append(resolution.Included.Actions, entry.Path)
		} else {
			resolution.Included.Workflows = append(resolution.Included.Workflows, entry.Path)
		}
	}

	for _, files := range []*[]string{&resolution.Included.Workflows, &resolution.Included.Actions} {
		slices.Sort(*files)
		*files = slices.Compact(*files)
	}

	return resolution, nil
}

func isYAML(file string) bool {
	return strings.HasSuffix(file, ".yml") || strings.HasSuffix(file, ".yaml")
}

// analysable says whether an included file can be read as the plain file it has to be, and
// named on a line of output.
func analysable(root string, entry gitindex.Entry) error {
	shown := finding.Escape(entry.Path)
	if strings.ContainsFunc(entry.Path, finding.IsControlOrSeparator) {
		return fmt.Errorf("%w %s: its name has a character that cannot be printed on a line", check.ErrNotAnalysable, shown)
	}
	if !entry.IsRegular() {
		return fmt.Errorf("%w %s: the repository tracks it as a link or a submodule, not a regular file", check.ErrNotAnalysable, shown)
	}

	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(entry.Path)))
	if err != nil {
		return fmt.Errorf("%w %s: %v", check.ErrNotAnalysable, shown, finding.Escape(err.Error()))
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w %s: it is not a regular file", check.ErrNotAnalysable, shown)
	}

	return nil
}
