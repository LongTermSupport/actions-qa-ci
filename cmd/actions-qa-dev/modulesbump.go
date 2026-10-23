package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/changelog"
	"github.com/LongTermSupport/actions-qa-ci/internal/modbump"
)

const (
	modulesBumpUsage = "usage: actions-qa-dev modules bump --gomod <go.mod> --module <path> [--changelog <CHANGELOG.md>] [--proxy https://proxy.golang.org] [--cooldown 72h]"

	defaultProxy = "https://proxy.golang.org"
)

// modulesBumpCommand says whether a Go module go.mod requires has a newer version past the
// cooldown, as "<module> <from> -> <to>" when it has, and records the move in the changelog when
// one is named. It does not touch go.mod: moving the requirement is go get's work, which checks
// the new version against the checksum database.
func modulesBumpCommand(client *http.Client, now time.Time, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("modules bump", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	goModPath := flags.String("gomod", "", "the go.mod file to read")
	module := flags.String("module", "", "the module path to look for a newer version of")
	changelogPath := flags.String("changelog", "", "a changelog file to record the move in, under Unreleased")
	proxy := flags.String("proxy", defaultProxy, "the https address of the module proxy")
	cooldown := flags.Duration("cooldown", defaultCooldown, "how old a version must be before it is taken")

	if err := flags.Parse(args); err != nil {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: %v\n%s", err, modulesBumpUsage))
	}
	if flags.NArg() != 0 {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unexpected argument %q\n%s", flags.Arg(0), modulesBumpUsage))
	}
	if *goModPath == "" || *module == "" {
		return complain(stderr, exitUsage, "actions-qa-dev: --gomod and --module are needed\n"+modulesBumpUsage)
	}
	if *cooldown < 0 {
		return complain(stderr, exitUsage, "actions-qa-dev: --cooldown must not be negative\n"+modulesBumpUsage)
	}

	goMod, err := readBounded(*goModPath, "go.mod")
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := modbump.Run(ctx, client, goMod, *module, modbump.Options{ProxyAddress: *proxy, Cooldown: *cooldown, Now: now})
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	line := result.Module + " is up to date"
	switch {
	case result.To != "":
		line = fmt.Sprintf("%s %s -> %s", result.Module, result.From, result.To)
		if err := recordModuleMove(*changelogPath, result); err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
		}
	case result.Note != "":
		line = result.Note
	}
	if _, err := fmt.Fprintln(stdout, line); err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
	}

	return 0
}

// recordModuleMove writes the move into the named changelog, if one is named.
func recordModuleMove(changelogPath string, result modbump.Result) error {
	if changelogPath == "" {
		return nil
	}
	recorded, err := recordMoves(changelogPath, []changelog.Move{{Tool: result.Module, From: result.From, To: result.To}})
	if err != nil {
		return err
	}

	return replaceFile(changelogPath, recorded)
}

// recordMoves is the changelog file's content with the moves recorded in it.
func recordMoves(path string, moves []changelog.Move) ([]byte, error) {
	current, err := readBounded(path, "the changelog")
	if err != nil {
		return nil, err
	}
	recorded, err := changelog.Record(current, moves)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return recorded, nil
}
