// Package cli is the actions-qa command line: argument handling, exit codes and the
// commands themselves. Apart from the working directory and the file a command is pointed
// at, it reads no process state, so a test drives it with plain writers and an Env holding
// fakes.
package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
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
  check [-t lane[,lane…]] [--format text|agent|json] [--] [path…]
                     run the lanes over the project's files, or over the files named
  rule [--format text|agent|json] <id> <path>
                     run one rule on one file and print only its findings
  help               print this text
`

// Env is what the commands read from outside themselves: the rules that exist, the lanes that
// run them and the pages that describe them. The program passes the real ones; a test passes
// fakes.
type Env struct {
	Registry *registry.Registry
	// Pages is rooted at the repository, so a rule's page is at registry.DocPath(id).
	Pages fs.FS
	// Lanes are the analyses `check` runs, in the order it runs them.
	Lanes []check.Lane
	// DefaultFormat names the output format `check` uses when none is asked for; empty is agent.
	DefaultFormat string
}

// Run executes one invocation against the real rules and the pages embedded in the binary.
func Run(args []string, stdout, stderr io.Writer) int {
	reg, err := catalogue.Registry()
	if err != nil {
		return fault(stderr, fmt.Errorf("the built-in rule registry is invalid: %w", err))
	}

	known := func(id string) bool {
		_, ok := reg.Lookup(id)

		return ok
	}
	env := Env{Registry: reg, Pages: actionsqaci.Pages, Lanes: catalogue.Lanes(known), DefaultFormat: defaultFormat(stdout)}

	return RunWith(env, args, stdout, stderr)
}

// defaultFormat is the format ACTIONS_QA_FORMAT names, else text for a terminal, else agent.
func defaultFormat(stdout io.Writer) string {
	if named := os.Getenv("ACTIONS_QA_FORMAT"); named != "" {
		return named
	}
	if file, ok := stdout.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return string(output.Text)
		}
	}

	return string(output.Agent)
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
	case "check":
		return checkCommand(env, args[1:], stdout, stderr)
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

// Every diagnostic goes through finding.Escape: a message can carry a name or a line from the
// repository under analysis, and that must not be able to start a line of its own or reach the
// terminal as a command.
func usageError(stderr io.Writer, problem string) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n%s", finding.Escape(problem), usage); err != nil {
		return ExitToolchainFault
	}

	return ExitUsage
}

// fault reports a failure of the toolchain itself and returns its exit code.
func fault(stderr io.Writer, cause error) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n", finding.Escape(cause.Error())); err != nil {
		return ExitToolchainFault
	}

	return ExitToolchainFault
}

// refusal reports a request the program cannot satisfy because of what was asked.
func refusal(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: %s\n", finding.Escape(message)); err != nil {
		return ExitToolchainFault
	}

	return ExitUsage
}

// refuseInvalid reports a project record that is not valid: one line saying so, naming the
// file, then a bullet for each reason, each escaped on its own so that none can add a line.
// Any other error is reported as a refusal.
func refuseInvalid(stderr io.Writer, err error) int {
	var invalid *record.InvalidError
	if !errors.As(err, &invalid) {
		return refusal(stderr, err.Error())
	}

	var text strings.Builder
	text.WriteString("actions-qa: ")
	if invalid.Path != "" {
		text.WriteString(finding.DisplayPath(invalid.Path) + ": ")
	}
	text.WriteString(record.ErrInvalid.Error() + ":\n")
	for _, reason := range invalid.Reasons {
		text.WriteString("  - " + finding.Escape(reason) + "\n")
	}
	if _, writeErr := io.WriteString(stderr, text.String()); writeErr != nil {
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
	// NarrowedBy says how the project record turns the defence off, which is a narrowing of the
	// method's standard and has to be visible wherever the defences are listed.
	NarrowedBy []string `json:"narrowedBy,omitempty"`
}

func rules(env Env, args []string, stdout, stderr io.Writer) int {
	asJSON, problem := jsonFlag("rules", args)
	if problem != "" {
		return usageError(stderr, problem)
	}

	active := activeEntries(env.Registry)
	if code := narrowByRecord(env, active, stderr); code != ExitOK {
		return code
	}

	if asJSON {
		return printJSON(stdout, stderr, active)
	}

	var out strings.Builder
	for _, entry := range active {
		out.WriteString(entry.ID + "  " + entry.Summary + "\n")
		for _, narrowing := range entry.NarrowedBy {
			out.WriteString("    narrowed: " + narrowing + "\n")
		}
	}
	if len(active) == 0 {
		out.WriteString("no rules are active\n")
	}

	if _, err := io.WriteString(stdout, out.String()); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}

// activeEntries lists every individual rule and every family, in identifier order. A family
// is listed once, under its placeholder name, not once per member.
func activeEntries(reg *registry.Registry) []ruleEntry {
	entries := []ruleEntry{}
	for _, rule := range reg.All() {
		entries = append(entries, ruleEntry{ID: rule.ID, Summary: rule.Summary, Upstream: rule.Upstream, DocPath: registry.DocPath(rule.ID)})
	}
	for _, family := range reg.Families() {
		entries = append(entries, ruleEntry{ID: family.Name, Summary: family.Summary, Upstream: family.Upstream, DocPath: pages.Directory + "/" + family.Page + ".md"})
	}
	slices.SortFunc(entries, func(a, b ruleEntry) int { return cmp.Compare(a.ID, b.ID) })

	return entries
}

// suggestionLimit is how many nearest identifiers an unknown one is answered with.
const suggestionLimit = 3

// lookupRule resolves an identifier a person typed to a registered rule. When it cannot, the
// returned string says why, naming the nearest identifiers where there are any.
func lookupRule(env Env, id string) (registry.Rule, string) {
	if _, err := finding.ClassifyIdentifier(id); err != nil {
		return registry.Rule{}, withSuggestions(env, err.Error(), id)
	}

	rule, known := env.Registry.Lookup(id)
	if known {
		return rule, ""
	}

	return registry.Rule{}, withSuggestions(env, fmt.Sprintf("unknown rule %q", id), id)
}

// withSuggestions adds the nearest registered identifiers to a message, where there are any.
func withSuggestions(env Env, message, id string) string {
	if nearest := env.Registry.Nearest(id, suggestionLimit); len(nearest) > 0 {
		message += "; did you mean " + strings.Join(nearest, ", ") + "?"
	}

	return message
}

func ruleDoc(env Env, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return usageError(stderr, "rule-doc takes exactly one rule identifier")
	}
	id := args[0]

	var page string
	if family, listed := env.Registry.FamilyNamed(id); listed {
		content, err := fs.ReadFile(env.Pages, pages.Directory+"/"+family.Page+".md")
		if err != nil {
			return fault(stderr, fmt.Errorf("family %s is registered but its page cannot be read: %w", id, err))
		}
		page = string(content)
	} else {
		if _, problem := lookupRule(env, id); problem != "" {
			return refusal(stderr, problem)
		}

		var err error
		page, err = pages.Page(env.Pages, id)
		if err != nil {
			return fault(stderr, fmt.Errorf("rule %s is registered but its page cannot be read: %w", id, err))
		}
	}

	if _, err := io.WriteString(stdout, page); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}

	return ExitOK
}
