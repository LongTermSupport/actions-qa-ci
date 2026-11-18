package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	changelogrelease "github.com/LongTermSupport/actions-qa-ci/internal/release"
)

const releaseUsage = "usage: actions-qa-dev release next --changelog <CHANGELOG.md>\n       actions-qa-dev release cut --changelog <CHANGELOG.md> --date YYYY-MM-DD --version-file <VERSION>\n       actions-qa-dev release pending --changelog <CHANGELOG.md> --tags <file of tag names>\n       actions-qa-dev release notes --changelog <CHANGELOG.md> --version X.Y.Z"

// releaseCommand answers the release workflow's questions from the changelog: what releases
// next, cutting it, which released version has no tag yet, and a version's notes. It prints a
// version, or nothing when there is none, so the workflow branches on the output alone.
func releaseCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return complain(stderr, exitUsage, releaseUsage)
	}
	sub := args[0]
	flags := flag.NewFlagSet("release "+sub, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	changelog := flags.String("changelog", "", "the changelog")
	date := flags.String("date", "", "the release date, as YYYY-MM-DD")
	versionFile := flags.String("version-file", "", "the VERSION file to write")
	tags := flags.String("tags", "", "a file listing the repository's tags, one a line")
	version := flags.String("version", "", "the released version")

	type option struct {
		name  string
		value *string
	}
	needed := map[string][]option{
		"next":    {{"changelog", changelog}},
		"cut":     {{"changelog", changelog}, {"date", date}, {"version-file", versionFile}},
		"pending": {{"changelog", changelog}, {"tags", tags}},
		"notes":   {{"changelog", changelog}, {"version", version}},
	}
	options, known := needed[sub]
	if !known {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unknown release command %q\n%s", sub, releaseUsage))
	}
	if err := flags.Parse(args[1:]); err != nil {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: %v\n%s", err, releaseUsage))
	}
	if flags.NArg() != 0 {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unexpected argument %q\n%s", flags.Arg(0), releaseUsage))
	}
	for _, o := range options {
		if *o.value == "" {
			return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: release %s needs --%s\n%s", sub, o.name, releaseUsage))
		}
	}

	content, err := readBounded(*changelog, "the changelog")
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	var out string
	switch sub {
	case "next":
		out, err = changelogrelease.Next(content)
	case "cut":
		out, err = cutRelease(*changelog, content, *date, *versionFile)
	case "pending":
		var list []byte
		list, err = readBounded(*tags, "the tag list")
		if err == nil {
			out, err = changelogrelease.Pending(content, strings.Split(string(list), "\n"))
		}
	case "notes":
		var notes string
		notes, err = changelogrelease.Notes(content, *version)
		if err == nil {
			_, err = io.WriteString(stdout, notes)
		}
		if err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
		}

		return 0
	}
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	if out != "" {
		if _, err := fmt.Fprintln(stdout, out); err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
		}
	}

	return 0
}

// cutRelease rewrites the changelog with the next version's section and writes that version to
// the VERSION file. Nothing is written when the cut is refused.
func cutRelease(path string, content []byte, date, versionFile string) (string, error) {
	version, err := changelogrelease.Next(content)
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("the Unreleased section of %s has no entry, so there is nothing to release", path)
	}
	cut, err := changelogrelease.Cut(content, version, date)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, cut, 0o644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := os.WriteFile(versionFile, []byte(version+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", versionFile, err)
	}

	return version, nil
}
