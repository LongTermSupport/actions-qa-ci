// Command actions-qa is the Defence Before Fix toolchain for GitHub Actions code.
package main

import (
	"os"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
