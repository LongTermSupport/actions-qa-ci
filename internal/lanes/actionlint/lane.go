// Package actionlint is the lane that runs actionlint, compiled in as a library, over one
// workflow file and reports what it finds as this toolchain's findings.
package actionlint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// Version is the actionlint release compiled in. go.mod pins it and a test keeps the two equal.
const Version = "v1.7.12"

// Tool names the backend on every finding it produces.
const Tool = "actionlint " + Version

// ShellcheckVersion is the ShellCheck release the lane runs on shell in run: steps: the one the
// manifest pins, which is the only place the version is written.
func ShellcheckVersion() string {
	return toolchain.Pinned("shellcheck")
}

// shellcheckTool names the backend on a finding that came from ShellCheck.
func shellcheckTool() string {
	return "shellcheck " + ShellcheckVersion() + " via " + Tool
}

// Lint runs actionlint over the workflow at a repository-relative path, with the working
// directory as the repository root, and returns its findings in a stable order.
//
// Shell in run: steps is checked by the pinned ShellCheck, which is found beside the program and
// verified against the manifest before it runs; one that is absent or differs fails the lane, and
// the one on PATH is never used.
//
// Of the project's own actionlint configuration, only the self-hosted runner labels are applied:
// they declare which custom labels exist, so `runs-on` is judged against the project's real
// runner pool. Its ignore patterns and configuration variables are never applied: an ignore
// would suppress findings without a recorded justification, and suppression is the project
// record's job. A malformed configuration is an error here.
func Lint(path string) ([]finding.Finding, error) {
	// actionlint takes a directory for the project root only when it holds .git (a directory,
	// or a file in a linked worktree) and .github/workflows as a directory.
	for _, marker := range []struct {
		path      string
		directory bool
	}{{".git", false}, {".github/workflows", true}} {
		info, err := os.Stat(filepath.FromSlash(marker.path))
		if err != nil {
			return nil, fmt.Errorf("the working directory must be the repository root, which holds %s: %w", marker.path, err)
		}
		if marker.directory && !info.IsDir() {
			return nil, fmt.Errorf("the working directory must be the repository root, and %s is not a directory", marker.path)
		}
	}

	shellcheck, err := pinnedShellcheck()
	if err != nil {
		return nil, err
	}

	// ShellCheck reads its options from this variable, and an -e there would suppress a finding
	// with no record. The caller's value is put back afterwards.
	restore, err := withoutEnvironment("SHELLCHECK_OPTS")
	if err != nil {
		return nil, err
	}
	defer restore()

	labels, err := projectRunnerLabels()
	if err != nil {
		return nil, err
	}
	config, cleanup, err := labelsConfig(labels)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	rulesCreated, shellcheckRule := false, false
	linter, err := upstream.NewLinter(io.Discard, &upstream.LinterOptions{
		ConfigFile: config,
		Color:      upstream.ColorOptionKindNever,
		Shellcheck: shellcheck,
		OnRulesCreated: func(rules []upstream.Rule) []upstream.Rule {
			rulesCreated, shellcheckRule = true, hasShellcheckRule(rules)

			return rules
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot start actionlint: %w", err)
	}

	errs, err := linter.LintFile(filepath.FromSlash(path), nil)
	if err != nil {
		return nil, fmt.Errorf("actionlint could not check %s: %w", path, err)
	}
	if rulesCreated && !shellcheckRule {
		// actionlint drops its ShellCheck rule without a word when it cannot start the
		// executable, and a lane that then reports no shell findings would read as clean. (A
		// file that does not parse never gets its rules created, and is reported as a syntax
		// finding.)
		return nil, fmt.Errorf("actionlint did not enable its ShellCheck rule although %s was verified; no shell would have been checked", shellcheck)
	}

	return Convert(path, errs)
}

// hasShellcheckRule says whether actionlint created its ShellCheck rule among these.
func hasShellcheckRule(rules []upstream.Rule) bool {
	for _, rule := range rules {
		if rule.Name() == shellcheckKind {
			return true
		}
	}

	return false
}

// withoutEnvironment unsets a variable and returns the function that puts it back as it was.
func withoutEnvironment(name string) (func(), error) {
	value, wasSet := os.LookupEnv(name)
	if !wasSet {
		return func() {}, nil
	}
	if err := os.Unsetenv(name); err != nil {
		return nil, fmt.Errorf("cannot unset %s: %w", name, err)
	}

	return func() {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintf(os.Stderr, "actions-qa: cannot restore %s: %v\n", name, err)
		}
	}, nil
}

// pinnedShellcheck finds the ShellCheck the manifest pins and verifies it.
func pinnedShellcheck() (string, error) {
	manifest, err := toolchain.Load()
	if err != nil {
		return "", err
	}
	dirs, err := toolchain.Directories()
	if err != nil {
		return "", err
	}
	binary, err := toolchain.Resolve(manifest, "shellcheck", toolchain.Platform(), dirs)
	if err != nil {
		return "", fmt.Errorf("cannot use the pinned ShellCheck: %w", err)
	}

	return binary, nil
}

// projectConfigNames are the files actionlint reads as the project's configuration, in the order
// it looks for them: the first that exists is the configuration.
var projectConfigNames = []string{".github/actionlint.yaml", ".github/actionlint.yml"}

// projectRunnerLabels returns the self-hosted runner labels the project's actionlint
// configuration declares, or none when it has no configuration. A configuration that cannot be
// read or parsed is an error, never an empty declaration.
func projectRunnerLabels() ([]string, error) {
	for _, name := range projectConfigNames {
		data, err := os.ReadFile(filepath.FromSlash(name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the project's actionlint configuration %s: %w", name, err)
		}
		cfg, err := upstream.ParseConfig(data)
		if err != nil {
			return nil, fmt.Errorf("cannot parse the project's actionlint configuration %s: %w", name, err)
		}

		return cfg.SelfHostedRunner.Labels, nil
	}

	return nil, nil
}

// labelsConfig writes a configuration that declares the given self-hosted runner labels and
// ignores nothing, because the linter reads the project's own configuration unless it is handed
// one.
func labelsConfig(labels []string) (path string, cleanup func(), err error) {
	var content struct {
		SelfHostedRunner struct {
			Labels []string `json:"labels"`
		} `json:"self-hosted-runner"`
	}
	content.SelfHostedRunner.Labels = append([]string{}, labels...)
	// JSON is YAML, and encoding/json quotes every label, so no label can be read as anything else.
	data, err := json.Marshal(content)
	if err != nil {
		return "", nil, fmt.Errorf("cannot write the actionlint configuration: %w", err)
	}

	dir, err := os.MkdirTemp("", "actions-qa-actionlint-")
	if err != nil {
		return "", nil, fmt.Errorf("cannot create the actionlint configuration: %w", err)
	}
	remove := func() {
		// Nothing depends on the removal succeeding: the directory holds one generated file.
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			fmt.Fprintf(os.Stderr, "actions-qa: cannot remove %s: %v\n", dir, removeErr)
		}
	}

	path = filepath.Join(dir, "actionlint.yaml")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		remove()

		return "", nil, fmt.Errorf("cannot create the actionlint configuration: %w", err)
	}

	return path, remove, nil
}

// Convert turns actionlint's errors for the file at path into findings. A kind the lane has no
// identifier for is an error: relabelling or dropping it would hide a defence the upstream
// added, so a bump that introduces one fails until it has a page and a fixture.
func Convert(path string, errs []*upstream.Error) ([]finding.Finding, error) {
	findings := make([]finding.Finding, 0, len(errs))

	for _, e := range errs {
		if e.Kind == shellcheckKind {
			f, reported, err := convertShellcheck(path, e)
			if err != nil {
				return nil, err
			}
			if reported {
				findings = append(findings, f)
			}

			continue
		}
		if !known(e.Kind) {
			return nil, fmt.Errorf("actionlint reported a finding of kind %q, which this toolchain has no identifier for", e.Kind)
		}

		f := finding.Finding{
			ID:         ID(e.Kind),
			Tool:       Tool,
			UpstreamID: e.Kind,
			Path:       path,
			Line:       max(e.Line, 1),
			Col:        max(e.Column, 1),
			Severity:   finding.SeverityError,
			Message:    e.Message,
			DocPath:    registry.DocPath(ID(e.Kind)),
		}
		if err := f.Validate(); err != nil {
			return nil, fmt.Errorf("actionlint produced a finding that cannot be reported: %w", err)
		}
		findings = append(findings, f)
	}

	finding.Sort(findings)

	return findings, nil
}

const shellcheckKind = "shellcheck"

// shellcheckReport is how actionlint words a ShellCheck finding: the code, the level, and the
// position within the script, then ShellCheck's own text.
var shellcheckReport = regexp.MustCompile(`(?s)^shellcheck reported issue in this script: (SC[0-9]{4}):(error|warning|info|style):([0-9]+):([0-9]+): (.*)$`)

// convertShellcheck turns a finding of ShellCheck's, reported through actionlint, into one of
// this toolchain's. The second result is false for a level below the default threshold
// (warning), which is not reported. A message it cannot read is an error: dropping it would
// hide a finding.
func convertShellcheck(path string, e *upstream.Error) (finding.Finding, bool, error) {
	match := shellcheckReport.FindStringSubmatch(e.Message)
	if match == nil {
		return finding.Finding{}, false, fmt.Errorf("cannot read the shellcheck finding actionlint reported at %s:%d: %q", path, e.Line, e.Message)
	}
	code, level, line, column, text := match[1], match[2], match[3], match[4], match[5]

	var severity finding.Severity
	switch level {
	case "error":
		severity = finding.SeverityError
	case "warning":
		severity = finding.SeverityWarning
	default:
		return finding.Finding{}, false, nil
	}

	id := finding.ToolPrefix + ".shellcheck." + code
	f := finding.Finding{
		ID:         id,
		Tool:       shellcheckTool(),
		UpstreamID: code,
		Path:       path,
		Line:       max(e.Line, 1),
		Col:        max(e.Column, 1),
		Severity:   severity,
		Message:    fmt.Sprintf("%s (script line %s, column %s)", strings.TrimRight(text, " "), line, column),
		DocPath:    registry.DocPath(id),
	}
	if err := f.Validate(); err != nil {
		return finding.Finding{}, false, fmt.Errorf("shellcheck produced a finding that cannot be reported: %w", err)
	}

	return f, true, nil
}
