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

const usage = "usage: actions-qa-dev fetch <directory>\n       actions-qa-dev bundle --version X.Y.Z --platform linux_amd64 --program <actions-qa> --tools <directory> --licences <directory> --out <directory>\n       actions-qa-dev install-script --version X.Y.Z --checksums <checksums.txt> --out <install.sh>\n       actions-qa-dev tools bump --manifest <manifest.json> [--api https://api.github.com] [--cooldown 72h]"

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
	if args[0] == "tools" {
		if len(args) < 2 || args[1] != "bump" {
			return complain(stderr, exitUsage, toolsBumpUsage)
		}
		client := &http.Client{Timeout: 5 * time.Minute}

		return toolsBumpCommand(client, os.Getenv("GITHUB_TOKEN"), time.Now(), args[2:], stdout, stderr)
	}
	if args[0] == "install-script" {
		return installScriptCommand(args[1:], stdout, stderr)
	}
	if args[0] != "fetch" && args[0] != "bundle" {
		return complain(stderr, exitUsage, fmt.Sprintf("actions-qa-dev: unknown command %q\n%s", args[0], usage))
	}
	if args[0] == "fetch" && len(args) != 2 {
		return complain(stderr, exitUsage, usage)
	}

	manifest, err := toolchain.Load()
	if err != nil {
		return complain(stderr, exitFault, fmt.Sprintf("actions-qa-dev: %v", err))
	}
	if args[0] == "bundle" {
		return bundleCommand(manifest, args[1:], stdout, stderr)
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
