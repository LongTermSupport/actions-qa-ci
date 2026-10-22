package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// ruleHarness runs one rule on one file and prints only that rule's findings: the way to
// confirm a detector fires on its fixture, and to see exactly what it prints, without any
// other rule's output in the way.
func ruleHarness(env Env, args []string, stdout, stderr io.Writer) int {
	format, positional, problem := harnessArguments(args)
	if problem != "" {
		return usageError(stderr, problem)
	}

	if _, listed := env.Registry.FamilyNamed(positional[0]); listed {
		return refusal(stderr, fmt.Sprintf("%s is a family of rules, not a rule; name one code of it", positional[0]))
	}

	rule, problem := lookupRule(env, positional[0])
	if problem != "" {
		return refusal(stderr, problem)
	}
	if rule.Check == nil {
		return refusal(stderr, fmt.Sprintf("rule %s cannot run on its own; it is reported only as part of its lane", rule.ID))
	}

	path, problem, err := repositoryPath(positional[1])
	if err != nil {
		return fault(stderr, err)
	}
	if problem != "" {
		return refusal(stderr, problem)
	}

	found, err := rule.Check(path)
	if errors.Is(err, record.ErrInvalid) {
		// A mistake in the project record is the project's to fix; no verdict is reached.
		return refusal(stderr, fmt.Sprintf("%s: %v", path, err))
	}
	if err != nil {
		return fault(stderr, fmt.Errorf("rule %s failed on %s: %w", rule.ID, path, err))
	}
	for _, f := range found {
		if f.ID != rule.ID {
			return fault(stderr, fmt.Errorf("rule %s reported a finding for %s, which is not its identifier", rule.ID, f.ID))
		}
	}

	if err := output.Write(stdout, format, found); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}
	if hasUnexcepted(found) {
		return ExitFindings
	}

	return ExitOK
}

// harnessArguments reads `[--format F] [--] <id> <path>`.
func harnessArguments(args []string) (output.Format, []string, string) {
	format := output.Agent

	for len(args) > 0 {
		switch {
		case args[0] == "--":
			args = args[1:]

			return harnessPositional(format, args)
		case args[0] == "--format":
			if len(args) < 2 {
				return "", nil, "--format needs a value"
			}
			parsed, err := output.ParseFormat(args[1])
			if err != nil {
				return "", nil, err.Error()
			}
			format = parsed
			args = args[2:]
		case strings.HasPrefix(args[0], "--"):
			return "", nil, fmt.Sprintf("unknown flag %q for rule", args[0])
		default:
			return harnessPositional(format, args)
		}
	}

	return harnessPositional(format, args)
}

func harnessPositional(format output.Format, args []string) (output.Format, []string, string) {
	if len(args) != 2 {
		return "", nil, "rule takes a rule identifier and a path"
	}

	return format, args, ""
}

func hasUnexcepted(found []finding.Finding) bool {
	for _, f := range found {
		if !f.Excepted {
			return true
		}
	}

	return false
}

// repositoryPath turns the path a person typed into the clean, slash-separated path relative
// to the working directory (the repository root) that rules and findings use. The second
// result says why a path cannot be used; the error is a failure to find the working directory.
func repositoryPath(typed string) (string, string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("cannot find the working directory: %w", err)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve the working directory: %w", err)
	}

	absolute := typed
	if !filepath.IsAbs(typed) {
		absolute = filepath.Join(root, typed)
	}
	absolute = filepath.Clean(absolute)

	// A working directory reached through a symlink can be named by either spelling.
	relative, inside := relativeTo(root, absolute)
	if !inside {
		relative, inside = relativeTo(realRoot, absolute)
	}
	if !inside {
		return "", fmt.Sprintf("%s is outside the repository (the working directory)", typed), nil
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Sprintf("cannot use %s: %v", typed, err), nil
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Sprintf("%s is not a file", typed), nil
	}

	// The name stays the one typed, but what it leads to must also be inside: a link in the
	// repository must not hand a rule a file from outside it.
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Sprintf("cannot use %s: %v", typed, err), nil
	}
	if _, inside := relativeTo(realRoot, resolved); !inside {
		return "", fmt.Sprintf("%s leads outside the repository (the working directory)", typed), nil
	}

	return filepath.ToSlash(relative), "", nil
}

// relativeTo is target relative to base, and whether target is base or inside it. A name that
// merely begins with two dots, such as ..hidden, is inside.
func relativeTo(base, target string) (string, bool) {
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}

	return relative, true
}
