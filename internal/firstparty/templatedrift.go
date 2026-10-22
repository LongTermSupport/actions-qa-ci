package firstparty

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// TemplateDrift is the identifier of the check that each declared copy of a template is
// byte-identical to its source.
const TemplateDrift = finding.ToolPrefix + ".templateDrift"

// maxTemplateSize bounds what is compared: a workflow is nowhere near it, and a file over it is
// reported rather than read.
const maxTemplateSize = 1 << 20

// CheckTemplateDrift is the check on the project record at a path: a finding at each copy that
// differs from its source, and one at the record's entry for a source or copy that is missing
// or is not a plain file. A missing file is a finding, never silence, so renaming a copy cannot
// turn the check off.
func CheckTemplateDrift(known func(id string) bool) func(path string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		rec, err := record.Load(path, known)
		if err != nil {
			return nil, err
		}

		working, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot find the working directory: %w", err)
		}
		root, err := filepath.EvalSymlinks(working)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve the working directory: %w", err)
		}

		var found []finding.Finding
		report := func(file string, line int, message string) error {
			f, err := driftFinding(file, line, message)
			if err != nil {
				return err
			}
			found = append(found, f)

			return nil
		}

		for _, template := range rec.Templates {
			source, problem, err := readTemplateFile(root, template.Source)
			if err != nil {
				return nil, err
			}
			if problem != "" {
				if err := report(path, template.Line, fmt.Sprintf("the source %s of a template %s", template.Source, problem)); err != nil {
					return nil, err
				}

				continue
			}

			for _, copied := range template.Copies {
				data, problem, err := readTemplateFile(root, copied)
				if err != nil {
					return nil, err
				}
				switch {
				case problem != "":
					err = report(path, template.Line, fmt.Sprintf("the copy %s of %s %s", copied, template.Source, problem))
				case !bytes.Equal(source, data):
					err = report(copied, 1, fmt.Sprintf("%s differs from its source %s: the first difference is on line %d", copied, template.Source, firstDifferingLine(source, data)))
				}
				if err != nil {
					return nil, err
				}
			}
		}
		finding.Sort(found)

		return found, nil
	}
}

// readTemplateFile reads a file to compare. The second result says why a file cannot be
// compared (missing, not a plain file, too large); the error is a failure to look at all. The
// file is opened without blocking and without following a link, and judged by what was opened,
// so a pipe or a link in its place is reported and never read.
func readTemplateFile(root, name string) ([]byte, string, error) {
	// The last component is not followed (below); a link in a directory of the path may be, but
	// only to somewhere inside the repository.
	directory, err := filepath.EvalSymlinks(filepath.Dir(filepath.FromSlash(name)))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, "is missing", nil
	case err != nil:
		return nil, fmt.Sprintf("cannot be reached: %v", err), nil
	}
	if !withinRoot(root, directory) {
		return nil, "leads outside the repository through a link in its path and is not read", nil
	}

	file, err := os.OpenFile(filepath.FromSlash(name), os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, "is missing", nil
	case errors.Is(err, syscall.ELOOP):
		return nil, "is not a regular file: it is a symbolic link", nil
	case err != nil:
		return nil, fmt.Sprintf("cannot be opened: %v", err), nil
	}
	defer closeQuietly(file)

	info, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("cannot look at %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, "is not a regular file", nil
	}

	data, err := io.ReadAll(io.LimitReader(file, maxTemplateSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("cannot read %s: %w", name, err)
	}
	if len(data) > maxTemplateSize {
		return nil, fmt.Sprintf("is larger than %d bytes and cannot be compared", maxTemplateSize), nil
	}

	return data, "", nil
}

// withinRoot says whether a directory, with its links resolved, is the root or inside it. root
// is itself resolved; a directory that is not absolute is relative to the working directory,
// which is the root, so it is joined to the resolved root and not to a working directory
// reached through a link.
func withinRoot(root, directory string) bool {
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(root, directory)
	}
	relative, err := filepath.Rel(root, directory)

	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// firstDifferingLine is the line, counted in line feeds from 1, where the two byte strings first
// differ, or, when one is the start of the other, where the shorter one ends.
func firstDifferingLine(a, b []byte) int {
	same := 0
	for same < len(a) && same < len(b) && a[same] == b[same] {
		same++
	}

	return 1 + bytes.Count(a[:same], []byte("\n"))
}

func driftFinding(file string, line int, message string) (finding.Finding, error) {
	return newFinding(TemplateDrift, file, line, 1, message)
}

func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close %s: %v\n", file.Name(), err)
	}
}
