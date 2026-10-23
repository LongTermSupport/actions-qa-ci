package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/LongTermSupport/actions-qa-ci/internal/installscript"
)

const installScriptUsage = "usage: actions-qa-dev install-script --version X.Y.Z --checksums <checksums.txt> --out <install.sh>"

// maxChecksums bounds the checksums file: a release has a line for each platform.
const maxChecksums = 1 << 20

// installScriptCommand fills the install.sh template for a release from the version and the
// checksums.txt the bundle build wrote, and writes it, executable, to a path that holds nothing
// yet.
func installScriptCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("install-script", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	version := flags.String("version", "", "the release, as X.Y.Z")
	checksums := flags.String("checksums", "", "the checksums.txt of the release")
	out := flags.String("out", "", "where to write the script")

	if err := flags.Parse(args); err != nil {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: %v\n%s", err, installScriptUsage))
	}
	if flags.NArg() != 0 {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unexpected argument %q\n%s", flags.Arg(0), installScriptUsage))
	}
	for _, option := range []struct{ name, value string }{{"version", *version}, {"checksums", *checksums}, {"out", *out}} {
		if option.value == "" {
			return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: --%s is needed\n%s", option.name, installScriptUsage))
		}
	}

	text, err := readChecksums(*checksums)
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	script, err := installscript.Fill(*version, text)
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot create the directory of %s: %v", *out, err))
	}
	if err := writeNew(*out, script, 0o755); err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	if _, err := fmt.Fprintln(stdout, *out); err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
	}

	return 0
}

// readChecksums reads a regular checksums file within a size bound.
func readChecksums(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot read the checksums: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("the checksums (%s) are not a file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read the checksums: %w", err)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxChecksums+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("cannot read the checksums (%s): %w", path, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("cannot close the checksums (%s): %w", path, closeErr)
	}
	if len(content) > maxChecksums {
		return "", fmt.Errorf("the checksums (%s) are larger than %d bytes", path, maxChecksums)
	}

	return string(content), nil
}
