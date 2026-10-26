package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/exceptions"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// checkCommand runs the lanes over the files the project has, or over the files it is given,
// and prints what they found. The record is judged first: a record that is wrong means nothing
// is analysed. A run that cannot cover every file it should has no verdict and says so.
func checkCommand(env Env, args []string, stdout, stderr io.Writer) int {
	arguments, problem := parseCheckArguments(args)
	if problem != "" {
		return usageError(stderr, problem)
	}

	format, problem := checkFormat(env, arguments)
	if problem != "" {
		return usageError(stderr, problem)
	}

	available := env.Lanes
	if arguments.online {
		online, code := onlineLane(env, stderr)
		if code != ExitOK {
			return code
		}
		available = append(slices.Clone(env.Lanes), online)
	} else if slices.Contains(arguments.lanes, onlineLaneName) {
		return refusal(stderr, "the lane "+onlineLaneName+" compares with the real branch protection and needs --online")
	}

	lanes, problem := chooseLanes(available, arguments.lanes)
	if problem != "" {
		return refusal(stderr, problem)
	}

	if _, err := os.Stat(".git"); err != nil {
		return refusal(stderr, fmt.Sprintf("the working directory must be the repository root, which holds .git: %v", err))
	}

	rec, code := validateRecord(env, stderr)
	if code != ExitOK {
		return code
	}

	included, code := includedFiles(rec, stderr)
	if code != ExitOK {
		return code
	}
	lanes = check.Widen(lanes, included)
	lanes, err := check.Enabling(lanes, rec.Enabled())
	if err != nil {
		return fault(stderr, err)
	}

	files, code := filesToCheck(lanes, arguments.files, stderr)
	if code != ExitOK {
		return code
	}
	if len(arguments.files) == 0 {
		files = append(files, included.Files()...)
	}

	run := check.RunProject
	if len(arguments.files) > 0 {
		run = check.Run
	}
	found, err := run(isolate(env.Isolation, lanes), files)
	var ended *check.NoVerdictError
	if errors.Is(err, record.ErrInvalid) {
		return refuseInvalid(stderr, err)
	}
	if errors.As(err, &ended) || errors.Is(err, check.ErrNotAnalysable) {
		return noVerdict(stderr, err)
	}
	if err != nil {
		return fault(stderr, err)
	}
	for _, f := range found {
		if _, known := env.Registry.Lookup(f.ID); !known {
			return fault(stderr, fmt.Errorf("a lane reported a finding for %s, which this build has no rule for", f.ID))
		}
		if err := f.Validate(); err != nil {
			return fault(stderr, fmt.Errorf("a lane reported a finding that cannot be reported: %w", err))
		}
	}

	if rec != nil {
		applied, err := exceptions.Apply(rec, found, readRepositoryFile)
		if err != nil {
			return fault(stderr, err)
		}
		found = applied.Findings
		if fullRun(arguments) {
			found = append(found, exceptions.Stale(rec, judgeable(rec, applied.Matched, arguments.online))...)
		}
	}

	if arguments.sarif != "" {
		if err := writeSARIF(env, arguments.sarif, found, stderr); err != nil {
			return fault(stderr, err)
		}
	}
	if err := output.Write(stdout, format, found); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}
	if fullRun(arguments) && !arguments.online {
		if err := noteOffline(rec, stderr); err != nil {
			return fault(stderr, fmt.Errorf("cannot write output: %w", err))
		}
	}
	if len(arguments.files) == 0 {
		if err := noteUnanalysed(check.Widen(env.Lanes, included), files, stderr); err != nil {
			return fault(stderr, fmt.Errorf("cannot write output: %w", err))
		}
	}
	if err := noteNotRun(lanes, files, stderr); err != nil {
		return fault(stderr, fmt.Errorf("cannot write output: %w", err))
	}
	if hasUnexcepted(found) {
		return ExitFindings
	}

	return ExitOK
}

// noteNotRun names, on the diagnostic stream, each check of a chosen lane's backend that this
// build does not run and why, so a clean result is not read as covering it. A lane that analysed
// none of the files has nothing to say.
func noteNotRun(lanes []check.Lane, files []string, stderr io.Writer) error {
	for _, lane := range lanes {
		if !slices.ContainsFunc(files, lane.Selects) {
			continue
		}
		for _, skipped := range lane.NotRun {
			if _, err := fmt.Fprintf(stderr, "actions-qa: the %s lane does not run %s: %s\n", finding.Escape(lane.Name), finding.Escape(skipped.ID), finding.Escape(skipped.Reason)); err != nil {
				return err
			}
		}
	}

	return nil
}

type checkArguments struct {
	lanes  []string
	format string
	sarif  string
	online bool
	files  []string
}

// parseCheckArguments reads `[-t lane[,lane…]]… [--format F] [--sarif file] [--] [path…]`.
func parseCheckArguments(args []string) (checkArguments, string) {
	var parsed checkArguments

	for len(args) > 0 {
		switch {
		case args[0] == "--":
			parsed.files = append(parsed.files, args[1:]...)

			return parsed, ""
		case args[0] == "-t":
			if len(args) < 2 {
				return checkArguments{}, "-t needs a lane name"
			}
			for _, name := range strings.Split(args[1], ",") {
				if strings.TrimSpace(name) == "" {
					return checkArguments{}, fmt.Sprintf("-t %q names a lane that is empty", args[1])
				}
				parsed.lanes = append(parsed.lanes, name)
			}
			args = args[2:]
		case args[0] == "--format":
			if len(args) < 2 {
				return checkArguments{}, "--format needs a value"
			}
			parsed.format = args[1]
			args = args[2:]
		case args[0] == "--online":
			parsed.online = true
			args = args[1:]
		case args[0] == "--sarif":
			if len(args) < 2 || args[1] == "" || strings.HasPrefix(args[1], "-") {
				return checkArguments{}, "--sarif needs the file to write (a name that starts with - is written ./-name)"
			}
			if parsed.sarif != "" {
				return checkArguments{}, "--sarif is given once"
			}
			parsed.sarif = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "-"):
			return checkArguments{}, fmt.Sprintf("unknown flag %q for check", args[0])
		default:
			parsed.files = append(parsed.files, args[0])
			args = args[1:]
		}
	}

	return parsed, ""
}

// checkFormat is the format asked for, or else the one the environment names, or else agent.
func checkFormat(env Env, arguments checkArguments) (output.Format, string) {
	switch {
	case arguments.format != "":
		format, err := output.ParseFormat(arguments.format)
		if err != nil {
			return "", err.Error()
		}

		return format, ""
	case env.DefaultFormat != "":
		format, err := output.ParseFormat(env.DefaultFormat)
		if err != nil {
			return "", "the default output format is wrong: " + err.Error()
		}

		return format, ""
	}

	return output.Agent, ""
}

// chooseLanes is the lanes asked for by name, in the order the build lists them, or all of
// them when none is named. The second result says why a name cannot be used.
func chooseLanes(all []check.Lane, wanted []string) ([]check.Lane, string) {
	if len(wanted) == 0 {
		return all, ""
	}

	names := make([]string, 0, len(all))
	for _, lane := range all {
		names = append(names, lane.Name)
	}
	for _, name := range wanted {
		if !slices.Contains(names, name) {
			return nil, fmt.Sprintf("there is no lane %q; the lanes are %s", name, strings.Join(names, ", "))
		}
	}

	chosen := make([]check.Lane, 0, len(wanted))
	for _, lane := range all {
		if slices.Contains(wanted, lane.Name) {
			chosen = append(chosen, lane)
		}
	}

	return chosen, ""
}

// fullRun says whether the run is every lane over every file the project has, the only kind
// against which an exception can be called stale.
func fullRun(arguments checkArguments) bool {
	return len(arguments.lanes) == 0 && len(arguments.files) == 0
}

// readRepositoryFile reads a file of the repository by its repository-relative path.
func readRepositoryFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.FromSlash(path))
}

// validateRecord loads the project record if there is one, so that a mistake in it stops the
// run before anything is analysed. It returns the record, or nil when the project has none.
func validateRecord(env Env, stderr io.Writer) (*record.Record, int) {
	// The record has one name. A copy under another extension would be read by nothing, and a
	// project with one would look like a project with no record.
	if _, err := os.Lstat(".github/actions-qa-ci.yaml"); err == nil {
		return nil, refusal(stderr, fmt.Sprintf("found .github/actions-qa-ci.yaml; the project record is %s", record.Path))
	}

	info, err := os.Lstat(record.Path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, ExitOK
	case errors.Is(err, syscall.ENOTDIR):
		// .github is not a directory: the same fault Discover reports for it.
		return nil, noVerdict(stderr, fmt.Errorf("cannot look for the project record: %w", err))
	case err != nil:
		// A project record that cannot be looked at is a file that cannot be read, like every
		// other one the default set holds: there is no verdict.
		return nil, noVerdict(stderr, fmt.Errorf("cannot look for the project record: %w", err))
	case !info.Mode().IsRegular():
		return nil, noVerdict(stderr, fmt.Errorf("cannot analyse %s: it is not a regular file", record.Path))
	}

	path, problem, err := repositoryPath(record.Path)
	if err != nil {
		return nil, fault(stderr, err)
	}
	if problem != "" {
		return nil, noVerdict(stderr, errors.New(problem))
	}

	known := func(id string) bool {
		_, ok := env.Registry.Lookup(id)

		return ok
	}
	rec, err := record.Load(path, known)
	if errors.Is(err, record.ErrInvalid) {
		return nil, refuseInvalid(stderr, err)
	}
	if err != nil {
		return nil, fault(stderr, err)
	}
	if reasons := unofferedDefences(env.Lanes, rec); len(reasons) > 0 {
		return nil, refuseInvalid(stderr, &record.InvalidError{Path: path, Reasons: reasons})
	}

	return rec, ExitOK
}

// unofferedDefences says, for each defence the record asks the lanes to run, why no lane can: it
// is one that already runs, one this build does not run, or not one a lane runs at all.
func unofferedDefences(lanes []check.Lane, rec *record.Record) []string {
	var reasons []string
	for i, enabled := range rec.Calibrations.Enable {
		if !check.Offers(lanes, enabled.ID) {
			reasons = append(reasons, fmt.Sprintf("enable entry %d (line %d): %s cannot be enabled: no lane of this build holds it back, because it already runs, this build does not run it, or it belongs to no lane", i+1, enabled.Line, enabled.ID))
		}
	}

	return reasons
}

// filesToCheck is the files named on the command line, each of which some chosen lane must
// analyse, or else the default set. The code is ExitOK when there are files to give the lanes.
func filesToCheck(lanes []check.Lane, named []string, stderr io.Writer) ([]string, int) {
	if len(named) == 0 {
		files, err := check.Discover(os.DirFS("."))
		if err != nil {
			return nil, noVerdict(stderr, err)
		}

		return files, ExitOK
	}

	files := make([]string, 0, len(named))
	for _, typed := range named {
		path, problem, err := repositoryPath(typed)
		if err != nil {
			return nil, fault(stderr, err)
		}
		if problem != "" {
			return nil, refusal(stderr, problem)
		}
		if len(check.Selecting(lanes, path)) == 0 {
			return nil, refusal(stderr, fmt.Sprintf("no lane analyses %s", typed))
		}
		files = append(files, path)
	}

	return files, ExitOK
}

// noteUnanalysed names, on the diagnostic stream, the files of the default set that no lane of
// this build lints, so a clean result is not read as covering them. A lane that only guards
// against suppression does not lint; a tool's configuration file is not linted either, it is
// forbidden, and the lane that forbids it covers it.
func noteUnanalysed(lanes []check.Lane, files []string, stderr io.Writer) error {
	var skipped []string
	for _, file := range files {
		if check.IsNativeSuppressionConfig(file) {
			continue
		}
		if !slices.ContainsFunc(check.Selecting(lanes, file), func(lane check.Lane) bool { return !lane.Guard }) {
			skipped = append(skipped, file)
		}
	}
	if len(skipped) == 0 {
		return nil
	}

	for i, file := range skipped {
		skipped[i] = finding.DisplayPath(file)
	}
	_, err := fmt.Fprintf(stderr, "actions-qa: no lane in this build analyses %s\n", strings.Join(skipped, ", "))

	return err
}

// noVerdict reports an analysis that could not cover what it should have.
func noVerdict(stderr io.Writer, cause error) int {
	if _, err := fmt.Fprintf(stderr, "actions-qa: no verdict: %s\n", finding.Escape(cause.Error())); err != nil {
		return ExitToolchainFault
	}

	return ExitNoVerdict
}
