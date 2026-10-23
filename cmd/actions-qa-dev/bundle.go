package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/LongTermSupport/actions-qa-ci/internal/bundle"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const bundleUsage = "usage: actions-qa-dev bundle --version X.Y.Z --platform linux_amd64 --program <actions-qa> --tools <directory> --licences <directory> --out <directory>"

// bundleCommand builds the release archive of one platform into a directory that holds nothing
// of the release yet, and writes the checksums beside it. The manifest is the one the program
// pins; the release workflow runs this once per platform.
func bundleCommand(manifest toolchain.Manifest, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("bundle", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	version := flags.String("version", "", "the release, as X.Y.Z")
	platform := flags.String("platform", "", "the platform, such as linux_amd64")
	program := flags.String("program", "", "the actions-qa executable built for the platform")
	tools := flags.String("tools", "", "the directory holding the pinned tools")
	licences := flags.String("licences", "", "the directory of licence files")
	out := flags.String("out", "", "the directory to write the release into")

	if err := flags.Parse(args); err != nil {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: %v\n%s", err, bundleUsage))
	}
	if flags.NArg() != 0 {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unexpected argument %q\n%s", flags.Arg(0), bundleUsage))
	}
	for _, option := range []struct{ name, value string }{
		{"version", *version}, {"platform", *platform}, {"program", *program},
		{"tools", *tools}, {"licences", *licences}, {"out", *out},
	} {
		if option.value == "" {
			return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: --%s is needed\n%s", option.name, bundleUsage))
		}
	}
	toolsDir, err := filepath.Abs(*tools)
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot resolve %s: %v", *tools, err))
	}

	built, err := bundle.Build(bundle.Options{
		Manifest: manifest,
		Platform: *platform,
		Version:  *version,
		Program:  *program,
		ToolDirs: []string{toolsDir},
		Licences: *licences,
	})
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	written, err := writeRelease(*out, built)
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	for _, path := range written {
		if _, err := fmt.Fprintln(stdout, path); err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
		}
	}

	return 0
}

// writeRelease writes the archive and checksums.txt into dir, creating it. A file already there
// is an error: a release is written once, never replaced.
func writeRelease(dir string, built bundle.Bundle) ([]string, error) {
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symbolic link; write the release into a directory", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot inspect %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}

	files := []struct {
		name    string
		content []byte
	}{
		{built.Name, built.Data},
		{"checksums.txt", []byte(bundle.Checksums([]bundle.Bundle{built}))},
	}

	// Nothing is written unless neither file is there, so a refused release leaves no half of one.
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("%s exists; a release is written once and never replaced", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("cannot inspect %s: %w", path, err)
		}
	}

	var written []string
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := writeNew(path, f.content); err != nil {
			for _, earlier := range written {
				if removeErr := os.Remove(earlier); removeErr != nil {
					err = errors.Join(err, fmt.Errorf("cannot remove %s: %w", earlier, removeErr))
				}
			}

			return nil, err
		}
		written = append(written, path)
	}

	return written, nil
}

// writeNew writes a file that must not exist yet.
func writeNew(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return fmt.Errorf("%s exists; a release is written once and never replaced", path)
	}
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}

	return writeAndClose(file, path, content)
}

// writeAndClose writes the content and closes the file, whatever the write did. A failure of
// either is returned by the name of the file; the write's comes first.
func writeAndClose(file io.WriteCloser, name string, content []byte) error {
	_, writeErr := file.Write(content)
	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("cannot write %s: %w", name, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("cannot close %s: %w", name, closeErr)
	}

	return nil
}
