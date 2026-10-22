package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// pagesAddress is where a rule's page can be read from outside the repository. It is the
// default branch: the program has no release version yet to name a tag with (the known gap
// sarif-help-uri-not-at-release-tag).
const pagesAddress = "https://github.com/LongTermSupport/actions-qa-ci/blob/main/"

// writeSARIF renders the findings as a SARIF log and writes it to the path, then says so on
// standard error. The log is rendered whole before the file is touched, so a finding that
// cannot be reported leaves the old file as it was.
func writeSARIF(env Env, path string, findings []finding.Finding, stderr io.Writer) error {
	var rendered bytes.Buffer
	options := output.SARIFOptions{
		Extensions: env.Extensions,
		Describe: func(id string) (string, bool) {
			rule, ok := env.Registry.Lookup(id)

			return rule.Summary, ok
		},
		HelpURI: func(id string) string { return pagesAddress + registry.DocPath(id) },
	}
	if err := output.WriteSARIF(&rendered, findings, options); err != nil {
		return err
	}

	if err := writeOutputFile(path, rendered.Bytes()); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stderr, "actions-qa: wrote the SARIF log to %s\n", finding.DisplayPath(path))

	return err
}

// writeOutputFile replaces the file at a path with the data. It never writes through a
// symbolic link at the last component, and never to a pipe or device there. A path inside the
// repository must not reach outside it through a link in one of its directories: the
// repository is the thing under analysis, and a link it holds must not choose what a run in
// its working directory overwrites. A path outside the repository, typed absolute, is the
// operator's own and is not confined.
func writeOutputFile(typed string, data []byte) error {
	working, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot find the working directory: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(working)
	if err != nil {
		return fmt.Errorf("cannot resolve the working directory: %w", err)
	}

	absolute := typed
	if !filepath.IsAbs(typed) {
		absolute = filepath.Join(working, typed)
	}
	absolute = filepath.Clean(absolute)

	if info, err := os.Lstat(absolute); err == nil && info.IsDir() {
		return fmt.Errorf("cannot write %s: it is a directory", finding.DisplayPath(typed))
	}

	_, insideWorking := relativeTo(working, absolute)
	_, insideReal := relativeTo(realRoot, absolute)
	inside := insideWorking || insideReal

	directory, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", finding.DisplayPath(typed), err)
	}
	if _, stays := relativeTo(realRoot, directory); inside && !stays {
		return fmt.Errorf("cannot write %s: it leads outside the repository through a link in its path", finding.DisplayPath(typed))
	}

	target := filepath.Join(directory, filepath.Base(absolute))
	mode := os.FileMode(0o644)
	switch existing, err := os.Lstat(target); {
	case err == nil && existing.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("cannot write %s: it is a symbolic link", finding.DisplayPath(typed))
	case err == nil && !existing.Mode().IsRegular():
		return fmt.Errorf("cannot write %s: it is not a regular file", finding.DisplayPath(typed))
	case err == nil:
		mode = existing.Mode().Perm()
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("cannot write %s: %w", finding.DisplayPath(typed), err)
	}

	return replaceFile(directory, target, data, mode, typed)
}

// replaceFile writes the data to a new file beside the target and moves it over the target, so
// the target is either as it was or whole: a write that fails half way cannot leave it cut short.
// The new file never follows a link at the target, because the move replaces the name.
func replaceFile(directory, target string, data []byte, mode os.FileMode, typed string) error {
	temporary, err := os.CreateTemp(directory, ".actions-qa-*")
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", finding.DisplayPath(typed), err)
	}

	written := errors.Join(chmodAndWrite(temporary, data, mode), temporary.Close())
	if written == nil {
		written = os.Rename(temporary.Name(), target)
	}
	if written != nil {
		return errors.Join(fmt.Errorf("cannot write %s: %w", finding.DisplayPath(typed), written), os.Remove(temporary.Name()))
	}

	return nil
}

func chmodAndWrite(file *os.File, data []byte, mode os.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return err
	}
	_, err := file.Write(data)

	return err
}
