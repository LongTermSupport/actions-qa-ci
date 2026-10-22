// Command actions-qa-dev is the development-only companion of actions-qa. It downloads the
// pinned third-party binaries for the machine it runs on, so tests that run them have them. The
// shipped toolchain never downloads anything.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const usage = "usage: actions-qa-dev fetch <directory>"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// exit codes: 2 for a wrong command line, 3 for a failure of the tool or of its output.
const (
	exitUsage = 2
	exitFault = 3
)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return complain(stderr, exitUsage, usage)
	}
	if args[0] != "fetch" {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unknown command %q\n%s", args[0], usage))
	}
	if len(args) != 2 {
		return complain(stderr, exitUsage, usage)
	}

	manifest, err := toolchain.Load()
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	paths, err := toolchain.FetchAll(ctx, http.DefaultClient, manifest, toolchain.Platform(), args[1])
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	for _, path := range paths {
		if _, err := fmt.Fprintln(stdout, path); err != nil {
			return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: cannot write output: %v", err))
		}
	}

	return 0
}

// complain writes a message to stderr and returns the exit code; a message that cannot be
// written is itself a fault.
func complain(stderr io.Writer, code int, message string) int {
	if _, err := fmt.Fprintln(stderr, message); err != nil {
		return exitFault
	}

	return code
}
