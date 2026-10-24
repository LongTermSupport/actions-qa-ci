// Package zizmor is the lane that runs zizmor, the pinned executable, over one workflow, action
// or Dependabot file and reports what it finds as this toolchain's findings.
package zizmor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// Version is the zizmor release the lane runs: the one the manifest pins, which is the only place
// the version is written.
func Version() string {
	return toolchain.Pinned("zizmor")
}

// Tool names the backend on every finding it produces.
func Tool() string {
	return "zizmor " + Version()
}

const (
	// personaRegular is the persona the lane runs at: findings with few false positives. The
	// pedantic and auditor audits are what a rule's own harness shows, and the project record's
	// `enable` is how a project would turn one on.
	personaRegular = "regular"
	personaAuditor = "auditor"
)

// limits bound a zizmor process: how long it may run, how much it may write to standard output,
// and how much of its standard error is kept.
type limits struct {
	timeout   time.Duration
	maxOutput int64
	maxStderr int
}

var defaultLimits = limits{timeout: 5 * time.Minute, maxOutput: 32 << 20, maxStderr: 64 << 10}

// Lint runs zizmor at its regular persona over the file at a repository-relative path, with the
// working directory as the repository root, and returns its findings in a stable order.
func Lint(path string) ([]finding.Finding, error) {
	return lintAs(path, personaRegular)
}

// lintAs is Lint at a persona.
func lintAs(path, persona string) ([]finding.Finding, error) {
	binary, err := pinnedZizmor()
	if err != nil {
		return nil, err
	}

	return lint(binary, path, persona, defaultLimits)
}

// pinnedZizmor finds the zizmor the manifest pins and verifies it. The one on PATH is never used.
func pinnedZizmor() (string, error) {
	manifest, err := toolchain.Load()
	if err != nil {
		return "", err
	}
	dirs, err := toolchain.Directories()
	if err != nil {
		return "", err
	}
	binary, err := toolchain.Resolve(manifest, "zizmor", toolchain.Platform(), dirs)
	if err != nil {
		return "", fmt.Errorf("cannot use the pinned zizmor: %w", err)
	}

	return binary, nil
}

// lint runs the given zizmor executable over one file.
//
// Nothing of the project's or the caller's is applied. zizmor is not given its configuration
// (an ignore there would suppress a finding with no recorded justification), is told to honour
// no ignore comment, runs offline, and is started with an environment of its own (only NO_COLOR=1), so no token, no
// configuration path and no option of the caller's reaches it. A file it cannot parse is an
// error and not a clean run. It decides nothing by its exit code, only by the report it prints.
func lint(binary, path, persona string, bound limits) ([]finding.Finding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bound.timeout)
	defer cancel()

	// The process is signalled when the thread that started it ends, which is why the thread is
	// held for as long as the process lives.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cmd := exec.CommandContext(ctx, binary,
		"--format=json-v1", "--no-config", "--no-ignores", "--offline", "--strict-collection",
		"--no-exit-codes", "--persona="+persona, "--no-progress", "--color=never", "--", path)
	cmd.Env = []string{"NO_COLOR=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second

	output := &cappedWriter{limit: bound.maxOutput, overflow: cancel}
	diagnostics := &cappedWriter{limit: int64(bound.maxStderr)}
	cmd.Stdout, cmd.Stderr = output, diagnostics

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start zizmor: %w", err)
	}
	waitErr := cmd.Wait()
	// Nothing zizmor started outlives it, whatever way it ended. The group is gone already when it
	// left nothing behind.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return nil, fmt.Errorf("cannot end what zizmor started: %w", err)
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, &check.NoVerdictError{Lane: "zizmor", Path: path, Reason: fmt.Sprintf("zizmor did not finish within the time limit of %s", bound.timeout)}
	case output.exceeded:
		return nil, &check.NoVerdictError{Lane: "zizmor", Path: path, Reason: fmt.Sprintf("zizmor wrote more than %d bytes to standard output", bound.maxOutput)}
	}

	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		if status, isWait := exit.Sys().(syscall.WaitStatus); (isWait && status.Signaled()) || exit.ExitCode() == 128+int(syscall.SIGSEGV) {
			return nil, &check.NoVerdictError{Lane: "zizmor", Path: path, Reason: fmt.Sprintf("zizmor ended abnormally (%v): %s", exit, excerpt(diagnostics.buffer.Bytes()))}
		}

		return nil, fmt.Errorf("zizmor failed (%v): %s", exit, excerpt(diagnostics.buffer.Bytes()))
	}
	if waitErr != nil {
		return nil, fmt.Errorf("zizmor could not be waited for: %w", waitErr)
	}

	return Convert(path, output.buffer.Bytes())
}

// excerpt is what a process wrote to standard error, on one line and with nothing a terminal
// would act on.
func excerpt(written []byte) string {
	return finding.Escape(strings.Join(strings.Fields(string(written)), " "))
}

// cappedWriter keeps what is written to it up to a limit, and calls overflow once when more is
// written. What came after the limit is dropped.
type cappedWriter struct {
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
	overflow context.CancelFunc
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	room := w.limit - int64(w.buffer.Len())
	if int64(len(p)) > room {
		w.exceeded = true
		if w.overflow != nil {
			w.overflow()
		}
		w.buffer.Write(p[:max(room, 0)])

		return len(p), nil
	}

	return w.buffer.Write(p)
}

// report is the part of zizmor's json-v1 output the lane reads. Fields the lane does not use are
// not named, so a release that adds one changes nothing here.
type report struct {
	Ident          string `json:"ident"`
	Desc           string `json:"desc"`
	Determinations struct {
		Severity string `json:"severity"`
	} `json:"determinations"`
	Locations []reportLocation `json:"locations"`
}

type reportLocation struct {
	Symbolic struct {
		Key struct {
			Local *struct {
				Path string `json:"verbatim_path"`
			} `json:"Local"`
		} `json:"key"`
		Annotation string `json:"annotation"`
		Kind       string `json:"kind"`
	} `json:"symbolic"`
	Concrete struct {
		Location struct {
			Start point `json:"start_point"`
			End   point `json:"end_point"`
		} `json:"location"`
	} `json:"concrete"`
}

type point struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

// Convert turns zizmor's json-v1 report for the file at path into findings. An audit the lane has
// no identifier for is an error: relabelling or dropping it would hide a defence the upstream
// added, so a bump that introduces one fails until it has a page and a fixture. So is a finding
// that cannot be placed in the file, and anything but a list. A finding zizmor marks ignored is
// reported like any other: the lane runs with ignores off, and the project record is where a
// finding is suppressed.
func Convert(path string, written []byte) ([]finding.Finding, error) {
	decoder := json.NewDecoder(bytes.NewReader(written))

	var reports []report
	if err := decoder.Decode(&reports); err != nil {
		return nil, fmt.Errorf("zizmor's report is not a list of findings: %s", finding.Escape(err.Error()))
	}
	if reports == nil {
		return nil, errors.New("zizmor's report is not a list of findings: it is null")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("zizmor wrote more than its report")
	}

	findings := make([]finding.Finding, 0, len(reports))
	for _, r := range reports {
		f, err := convert(path, r)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	finding.Sort(findings)

	return findings, nil
}

func convert(path string, r report) (finding.Finding, error) {
	if _, known := audit(r.Ident); !known {
		return finding.Finding{}, fmt.Errorf("zizmor reported a finding of the audit %q, which this toolchain has no identifier for", finding.Escape(r.Ident))
	}
	if strings.TrimSpace(r.Desc) == "" {
		return finding.Finding{}, fmt.Errorf("zizmor reported a %s finding with no description", r.Ident)
	}
	severity, err := severityOf(r.Determinations.Severity)
	if err != nil {
		return finding.Finding{}, fmt.Errorf("a %s finding: %w", r.Ident, err)
	}

	// A finding can have several primary locations (an action and the input it was given); it is
	// reported at the first, which is where zizmor lists it first.
	var primary *reportLocation
	for i := range r.Locations {
		if r.Locations[i].Symbolic.Kind == "Primary" {
			primary = &r.Locations[i]

			break
		}
	}
	if primary == nil {
		return finding.Finding{}, fmt.Errorf("zizmor reported a %s finding with no primary location", r.Ident)
	}
	if local := primary.Symbolic.Key.Local; local == nil || local.Path != path {
		return finding.Finding{}, fmt.Errorf("zizmor reported a %s finding that is not in %s", r.Ident, path)
	}

	start, end := primary.Concrete.Location.Start, primary.Concrete.Location.End
	endLine := end.Row + 1
	if end.Column == 0 && end.Row > start.Row {
		// The range ends at the start of a line, so the last line it holds is the one before.
		endLine = end.Row
	}

	message := r.Desc
	if annotation := strings.TrimSpace(primary.Symbolic.Annotation); annotation != "" {
		message += ": " + annotation
	}

	f := finding.Finding{
		ID:         ID(r.Ident),
		Tool:       Tool(),
		UpstreamID: r.Ident,
		Path:       path,
		Line:       start.Row + 1,
		Col:        start.Column + 1,
		EndLine:    endLine,
		Severity:   severity,
		Message:    message,
		DocPath:    registry.DocPath(ID(r.Ident)),
	}
	if err := f.Validate(); err != nil {
		return finding.Finding{}, fmt.Errorf("zizmor produced a finding that cannot be reported: %w", err)
	}

	return f, nil
}

// severityOf reads zizmor's severity as this toolchain's: a high one is an error, a medium one a
// warning and the rest notes. Every finding blocks all the same; the level is for the reader.
func severityOf(level string) (finding.Severity, error) {
	switch level {
	case "High":
		return finding.SeverityError, nil
	case "Medium":
		return finding.SeverityWarning, nil
	case "Low", "Informational":
		return finding.SeverityNote, nil
	}

	return "", fmt.Errorf("zizmor gave the severity %q, which this toolchain does not know", finding.Escape(level))
}
