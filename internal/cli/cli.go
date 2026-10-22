// Package cli is the actions-qa command line: argument handling, exit codes and the
// commands themselves. It reads no process state, so a test drives it with plain writers.
package cli

import (
	"encoding/json"
	"fmt"
	"io"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
)

// The exit codes the program answers with (design 7.2). A non-zero code other than
// ExitFindings means no verdict was reached about the code under analysis.
const (
	ExitOK             = 0 // analysed, nothing to report
	ExitFindings       = 1 // analysed, findings reported
	ExitUsage          = 2 // the command line or a project record was wrong; nothing analysed
	ExitToolchainFault = 3 // a bundled tool failed or its pinned binary did not verify
	ExitNoVerdict      = 4 // the analysis could not reach a verdict
)

const usage = `usage: actions-qa <command> [flags]

commands:
  version [--json]   print the specification versions this build is audited against
  help               print this text
`

// Run executes one invocation and returns the process exit code. Results go to stdout,
// diagnostics to stderr; a usage error writes nothing to stdout because it is not a verdict.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}

	switch args[0] {
	case "help", "--help", "-h":
		return printUsage(stdout, stderr)
	case "version":
		return version(args[1:], stdout, stderr)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func printUsage(stdout, stderr io.Writer) int {
	if _, err := io.WriteString(stdout, usage); err != nil {
		return writeFailure(stderr, err)
	}

	return ExitOK
}

func usageError(stderr io.Writer, problem string) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n%s", problem, usage); err != nil {
		return ExitToolchainFault
	}

	return ExitUsage
}

func writeFailure(stderr io.Writer, cause error) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: cannot write output: %v\n", cause); err != nil {
		return ExitToolchainFault
	}

	return ExitToolchainFault
}

func version(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, arg := range args {
		if arg != "--json" {
			return usageError(stderr, fmt.Sprintf("unknown flag %q for version", arg))
		}
		asJSON = true
	}

	declaration, err := actionsqaci.LoadDeclaration()
	if err != nil {
		if _, printErr := fmt.Fprintf(stderr, "actions-qa: %v\n", err); printErr != nil {
			return ExitToolchainFault
		}

		return ExitUsage
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(declaration); err != nil {
			return writeFailure(stderr, err)
		}

		return ExitOK
	}

	if _, err := fmt.Fprintf(stdout, "actions-qa (method %s, toolchain %s, %d known gaps)\n",
		declaration.Method, declaration.Toolchain, len(declaration.KnownGaps)); err != nil {
		return writeFailure(stderr, err)
	}

	return ExitOK
}
