// Package cli is the actions-qa command line: argument handling, exit codes and the
// commands themselves. Apart from the working directory and the file a command is pointed
// at, it reads no process state, so a test drives it with plain writers and an Env holding
// fakes.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// The exit codes the program answers with. A non-zero code other than
// ExitFindings means no verdict was reached about the code under analysis.
const (
	ExitOK             = 0 // analysed, nothing to report
	ExitFindings       = 1 // analysed, findings reported
	ExitUsage          = 2 // the command line or a project record was wrong; nothing analysed
	ExitToolchainFault = 3 // the toolchain itself failed: a rule, a bundled tool, a page, or unwritable output
	ExitNoVerdict      = 4 // the analysis could not reach a verdict
)

const usage = `usage: actions-qa <command> [flags]

commands:
  version [--json]   print the specification versions this build is audited against
  rules [--json]     list every active rule without running it
  rule-doc <id>      print the page for a rule identifier, offline
  rule [--format text|agent|json] <id> <path>
                     run one rule on one file and print only its findings
  help               print this text
`

// Env is what the commands read from outside themselves: the rules that exist and the
// pages that describe them. The program passes the real ones; a test passes fakes.
type Env struct {
	Registry *registry.Registry
	// Pages is rooted at the repository, so a rule's page is at registry.DocPath(id).
	Pages fs.FS
}

// Run executes one invocation against the real rules and the pages embedded in the binary.
func Run(args []string, stdout, stderr io.Writer) int {
	reg, err := catalogue.Registry()
	if err != nil {
		return fault(stderr, fmt.Errorf("the built-in rule registry is invalid: %w", err))
	}

	return RunWith(Env{Registry: reg, Pages: actionsqaci.Pages}, args, stdout, stderr)
}

// RunWith executes one invocation and returns the process exit code. Results go to stdout,
// diagnostics to stderr; a usage error writes nothing to stdout because it is not a verdict.
func RunWith(env Env, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}

	switch args[0] {
	case "help", "--help", "-h":
		return printUsage(stdout, stderr)
	case "version":
		return version(args[1:], stdout, stderr)
	case "rules":
		return rules(env, args[1:], stdout, stderr)
	case "rule-doc":
		return ruleDoc(env, args[1:], stdout, stderr)
	case "rule":
		return ruleHarness(env, args[1:], stdout, stderr)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func printUsage(stdout, stderr io.Writer) int {
	if _, err := io.WriteString(stdout, usage); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}

func usageError(stderr io.Writer, problem string) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n%s", problem, usage); err != nil {
		return ExitToolchainFault
	}

	return ExitUsage
}

// fault reports a failure of the toolchain itself and returns its exit code.
func fault(stderr io.Writer, cause error) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %v\n", cause); err != nil {
		return ExitToolchainFault
	}

	return ExitToolchainFault
}

// refusal reports a request the program cannot satisfy because of what was asked.
func refusal(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n", message); err != nil {
		return ExitToolchainFault
	}

	return ExitUsage
}

func version(args []string, stdout, stderr io.Writer) int {
	asJSON, problem := jsonFlag("version", args)
	if problem != "" {
		return usageError(stderr, problem)
	}

	declaration, err := actionsqaci.LoadDeclaration()
	if err != nil {
		return refusal(stderr, err.Error())
	}

	if asJSON {
		return printJSON(stdout, stderr, declaration)
	}

	if _, err := fmt.Fprintf(stdout, "actions-qa (method %s, toolchain %s, %d known gaps)\n",
		declaration.Method, declaration.Toolchain, len(declaration.KnownGaps)); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}

// jsonFlag reads the arguments of a command whose only flag is --json.
func jsonFlag(command string, args []string) (asJSON bool, problem string) {
	for _, arg := range args {
		if arg != "--json" {
			return false, fmt.Sprintf("unknown flag or argument %q for %s", arg, command)
		}
		asJSON = true
	}

	return asJSON, ""
}

func printJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}

// ruleEntry is one rule as `rules --json` prints it.
type ruleEntry struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Upstream string `json:"upstream"`
	DocPath  string `json:"docPath"`
}

func rules(env Env, args []string, stdout, stderr io.Writer) int {
	asJSON, problem := jsonFlag("rules", args)
	if problem != "" {
		return usageError(stderr, problem)
	}

	active := env.Registry.All()

	if asJSON {
		entries := make([]ruleEntry, 0, len(active))
		for _, rule := range active {
			entries = append(entries, ruleEntry{rule.ID, rule.Summary, rule.Upstream, registry.DocPath(rule.ID)})
		}

		return printJSON(stdout, stderr, entries)
	}

	var out strings.Builder
	for _, rule := range active {
		out.WriteString(rule.ID + "  " + rule.Summary + "\n")
	}
	if len(active) == 0 {
		out.WriteString("no rules are active\n")
	}

	if _, err := io.WriteString(stdout, out.String()); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}

// suggestionLimit is how many nearest identifiers an unknown one is answered with.
const suggestionLimit = 3

// lookupRule resolves an identifier a person typed to a registered rule. When it cannot, the
// returned string says why, naming the nearest identifiers where there are any.
func lookupRule(env Env, id string) (registry.Rule, string) {
	if _, err := finding.ClassifyIdentifier(id); err != nil {
		return registry.Rule{}, err.Error()
	}

	rule, known := env.Registry.Lookup(id)
	if known {
		return rule, ""
	}

	message := fmt.Sprintf("unknown rule %q", id)
	if nearest := env.Registry.Nearest(id, suggestionLimit); len(nearest) > 0 {
		message += "; did you mean " + strings.Join(nearest, ", ") + "?"
	}

	return registry.Rule{}, message
}

func ruleDoc(env Env, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return usageError(stderr, "rule-doc takes exactly one rule identifier")
	}
	id := args[0]

	if _, problem := lookupRule(env, id); problem != "" {
		return refusal(stderr, problem)
	}

	page, err := pages.Page(env.Pages, id)
	if err != nil {
		return fault(stderr, fmt.Errorf("rule %s is registered but its page cannot be read: %w", id, err))
	}

	if _, err := io.WriteString(stdout, page); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}
