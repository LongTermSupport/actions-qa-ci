package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const (
	toolsBumpUsage = "usage: actions-qa-dev tools bump --manifest <manifest.json> [--api https://api.github.com] [--cooldown 72h]"

	defaultAPI      = "https://api.github.com"
	defaultCooldown = 72 * time.Hour
	maxManifest     = 1 << 20
)

// toolsBumpCommand moves each tool of a manifest file to its latest release once that release
// is older than the cooldown, and rewrites the file only when something moved. It proposes: the
// result is a change for review, never a release or a merge.
func toolsBumpCommand(client *http.Client, token string, now time.Time, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tools bump", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "the manifest file to update")
	api := flags.String("api", defaultAPI, "the https address of the GitHub API")
	cooldown := flags.Duration("cooldown", defaultCooldown, "how old a release must be before it is taken")

	if err := flags.Parse(args); err != nil {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: %v\n%s", err, toolsBumpUsage))
	}
	if flags.NArg() != 0 {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unexpected argument %q\n%s", flags.Arg(0), toolsBumpUsage))
	}
	if *manifestPath == "" {
		return complain(stderr, exitUsage, "actions-qa-dev: --manifest is needed\n"+toolsBumpUsage)
	}
	if *cooldown < 0 {
		return complain(stderr, exitUsage, "actions-qa-dev: --cooldown must not be negative\n"+toolsBumpUsage)
	}

	current, err := readManifest(*manifestPath)
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := bump.Run(ctx, client, current, bump.Options{APIAddress: *api, Token: token, Cooldown: *cooldown, Now: now})
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	var report []string
	for _, change := range result.Changes {
		report = append(report, fmt.Sprintf("%s %s -> %s", change.Tool, change.From, change.To))
	}
	report = append(report, result.Notes...)
	if len(result.Changes) == 0 {
		report = append(report, "every tool is up to date")
	} else if err := rewriteManifest(*manifestPath, result.Manifest); err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	for _, line := range report {
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
		}
	}

	return 0
}

// readManifest reads a regular manifest file within a size bound and parses it.
func readManifest(path string) (toolchain.Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return toolchain.Manifest{}, fmt.Errorf("cannot read the manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifest {
		return toolchain.Manifest{}, fmt.Errorf("the manifest (%s) is not a regular file of at most %d bytes", path, maxManifest)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return toolchain.Manifest{}, fmt.Errorf("cannot read the manifest: %w", err)
	}
	manifest, err := toolchain.Parse(content)
	if err != nil {
		return toolchain.Manifest{}, fmt.Errorf("%s: %w", path, err)
	}

	return manifest, nil
}

// rewriteManifest replaces the file with the manifest, in the form the file is kept in, through
// a temporary file beside it, keeping its mode. A manifest the toolchain would refuse is never
// written.
func rewriteManifest(path string, manifest toolchain.Manifest) error {
	written, err := bump.Marshal(manifest)
	if err != nil {
		return err
	}
	if _, err := toolchain.Parse(written); err != nil {
		return fmt.Errorf("the bumped manifest would not be accepted, so it was not written: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %w", path, err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".manifest-")
	if err != nil {
		return fmt.Errorf("cannot stage the manifest beside %s: %w", path, err)
	}
	staged := temp.Name()
	fail := func(cause error) error {
		return errors.Join(cause, temp.Close(), removeIfThere(staged))
	}
	if _, err := temp.Write(written); err != nil {
		return fail(fmt.Errorf("cannot write %s: %w", staged, err))
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return fail(fmt.Errorf("cannot set the mode of %s: %w", staged, err))
	}
	if err := temp.Close(); err != nil {
		return errors.Join(fmt.Errorf("cannot write %s: %w", staged, err), removeIfThere(staged))
	}
	if err := os.Rename(staged, path); err != nil {
		return errors.Join(fmt.Errorf("cannot replace %s: %w", path, err), removeIfThere(staged))
	}

	return nil
}

func removeIfThere(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
