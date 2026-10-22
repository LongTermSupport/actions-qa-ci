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

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)

		return 2
	}
	if args[0] != "fetch" {
		fmt.Fprintf(stderr, "actions-qa-dev: unknown command %q\n%s\n", args[0], usage)

		return 2
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, usage)

		return 2
	}

	manifest, err := toolchain.Load()
	if err != nil {
		fmt.Fprintf(stderr, "actions-qa-dev: %v\n", err)

		return 3
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	paths, err := toolchain.FetchAll(ctx, http.DefaultClient, manifest, toolchain.Platform(), args[1])
	if err != nil {
		fmt.Fprintf(stderr, "actions-qa-dev: %v\n", err)

		return 3
	}
	for _, path := range paths {
		fmt.Fprintln(stdout, path)
	}

	return 0
}
