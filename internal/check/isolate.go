package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// ChildFault is the exit code of a lane process whose lane failed. Its standard error says why.
// Any other way for the process to end without findings is no verdict.
const ChildFault = 3

// Limits bound a lane process: how long it may run, how much it may write to standard output
// and how much of its standard error is kept.
type Limits struct {
	Timeout   time.Duration
	MaxOutput int64
	MaxStderr int
}

// Command builds the process that runs the named lane on one file.
type Command func(ctx context.Context, lane, path string) *exec.Cmd

// NoVerdictError is a lane process that ended without findings and without saying why: it
// crashed, was killed by a signal, ran past its time limit or wrote more than it may. The
// analysis of that file has no verdict, and it is never reported as clean.
type NoVerdictError struct {
	Lane   string
	Path   string
	Reason string
}

func (e *NoVerdictError) Error() string {
	return finding.Escape("the lane process ended without a verdict: " + e.Reason)
}

// Isolated is the lane run in a child process instead of in this one, so that a backend that
// crashes the process (a runtime fatal error, a segmentation fault) or never finishes cannot
// take the analysis with it or be mistaken for a clean result.
func Isolated(lane Lane, command Command, limits Limits) Lane {
	isolated := lane
	isolated.Run = func(path string) ([]finding.Finding, error) {
		return runChild(lane.Name, path, command, limits)
	}

	return isolated
}

func runChild(name, path string, command Command, limits Limits) ([]finding.Finding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limits.Timeout)
	defer cancel()

	// The lane process is signalled when the thread that started it ends, which is why the thread
	// is held for as long as the process lives. A program that is killed leaves no lane process
	// behind to run without its time limit. The signal is SIGTERM, not SIGKILL, so that Serve can
	// end the tools the lane started along with itself.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cmd := command(ctx, name, path)
	// The lane process may start tools of its own; they die with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second

	output := &limitedWriter{limit: limits.MaxOutput, overflow: cancel}
	diagnostics := &limitedWriter{limit: int64(limits.MaxStderr)}
	cmd.Stdout, cmd.Stderr = output, diagnostics

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start the lane process: %w", err)
	}
	waitErr := cmd.Wait()
	// Nothing the lane process started outlives it, whatever way it ended. The group is gone
	// already when the process left nothing behind.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return nil, fmt.Errorf("cannot end what the lane process started: %w", err)
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, &NoVerdictError{name, path, fmt.Sprintf("it did not finish within the time limit of %s", limits.Timeout)}
	case output.exceeded:
		return nil, &NoVerdictError{name, path, fmt.Sprintf("it wrote more than %d bytes to standard output", limits.MaxOutput)}
	}

	var exit *exec.ExitError
	switch {
	case errors.As(waitErr, &exit) && exit.ExitCode() == ChildFault:
		return nil, fmt.Errorf("the lane process failed: %s", excerpt(diagnostics.buffer.Bytes()))
	case errors.As(waitErr, &exit):
		return nil, &NoVerdictError{name, path, fmt.Sprintf("%v: %s", exit, excerpt(diagnostics.buffer.Bytes()))}
	case waitErr != nil:
		return nil, fmt.Errorf("the lane process could not be waited for: %w", waitErr)
	}

	return decode(output.buffer.Bytes())
}

// excerpt is what a process wrote to standard error, on one line and with nothing a terminal
// would act on.
func excerpt(written []byte) string {
	return finding.Escape(strings.Join(strings.Fields(string(written)), " "))
}

// decode reads the findings a lane process wrote. Anything else is a defect of the toolchain.
func decode(written []byte) ([]finding.Finding, error) {
	decoder := json.NewDecoder(bytes.NewReader(written))
	decoder.DisallowUnknownFields()

	var found []finding.Finding
	if err := decoder.Decode(&found); err != nil {
		return nil, fmt.Errorf("the lane process wrote output that is not findings: %s", finding.Escape(err.Error()))
	}
	if found == nil {
		return nil, errors.New("the lane process wrote no list of findings")
	}
	if decoder.More() {
		return nil, errors.New("the lane process wrote more than its findings")
	}
	for _, f := range found {
		if err := f.Validate(); err != nil {
			return nil, fmt.Errorf("the lane process wrote a finding that cannot be reported: %s", finding.Escape(err.Error()))
		}
	}

	return found, nil
}

// Serve is what the lane process does: run the lane on one file and write the findings as JSON
// to stdout, or, when the lane fails, say why on stderr and return ChildFault.
func Serve(lane Lane, path string, stdout, stderr io.Writer) int {
	done := make(chan struct{})
	defer close(done)
	endWithParent(done)

	found, err := lane.Run(path)
	if err != nil {
		fmt.Fprintln(stderr, err)

		return ChildFault
	}
	if found == nil {
		found = []finding.Finding{}
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(found); err != nil {
		fmt.Fprintf(stderr, "cannot write the findings: %v\n", err)

		return ChildFault
	}

	return 0
}

// endWithParent ends this process and the tools it started when it is told to stop, which is
// what the program that isolated it does by dying. The tools are in the process group this
// process leads, which is the group of its own id; a process that leads no group (one a person
// started by hand) has no such group to end and only exits.
func endWithParent(done <-chan struct{}) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)

	go func() {
		defer signal.Stop(stop)

		select {
		case <-done:
		case <-stop:
			if err := syscall.Kill(-os.Getpid(), syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				fmt.Fprintf(os.Stderr, "cannot end what this process started: %v\n", err)
			}
			os.Exit(terminated)
		}
	}()
}

// terminated is the exit code of a process that was told to stop: 128 and the signal.
const terminated = 128 + int(syscall.SIGTERM)

// limitedWriter keeps what is written to it up to a limit, and calls overflow once when more is
// written. What came after the limit is dropped.
type limitedWriter struct {
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
	overflow context.CancelFunc
}

func (w *limitedWriter) Write(p []byte) (int, error) {
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
