package check_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const (
	childEnvironment   = "CHECK_TEST_CHILD_MODE"
	pidFileEnvironment = "CHECK_TEST_PID_FILE"
	hereAndNow         = ".github/workflows/ci.yml"
)

func someFinding(path string) finding.Finding {
	return finding.Finding{
		ID: "actionsqaci.actionlint.glob", Tool: "t", Path: path, Line: 2, Col: 3,
		Severity: finding.SeverityError, Message: "m", DocPath: "docs/rules/actionsqaci.actionlint.glob.md",
	}
}

// TestMain turns the test binary into the child process a lane is isolated in, when it is
// started as one.
func TestMain(m *testing.M) {
	if mode := os.Getenv(childEnvironment); mode != "" {
		os.Exit(childMain(mode))
	}
	os.Exit(m.Run())
}

func childMain(mode string) int {
	served := check.Lane{
		Name:    "child",
		Selects: func(string) bool { return true },
		Run: func(path string) ([]finding.Finding, error) {
			switch mode {
			case "findings":
				return []finding.Finding{someFinding(path)}, nil
			case "lane-error":
				return nil, errors.New("shellcheck could not start\nactions-qa: FORGED")
			}

			return nil, nil
		},
	}

	switch mode {
	case "findings", "lane-error", "nothing":
		return check.Serve(served, os.Args[len(os.Args)-1], os.Stdout, os.Stderr)
	case "crash":
		fmt.Fprintln(os.Stderr, "fatal error: stack overflow")
		fmt.Fprintln(os.Stderr, strings.Repeat("goroutine 1 [running]:\n", 3000))
		return 2
	case "killed":
		if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
			return 99
		}
		time.Sleep(time.Minute)
	case "hang":
		time.Sleep(time.Minute)
	case "flood":
		fmt.Fprint(os.Stdout, strings.Repeat("x", 1<<16))
	case "garbage":
		fmt.Fprint(os.Stdout, "this is not json")
	case "null":
		fmt.Fprint(os.Stdout, "null")
	case "findings-then-crash":
		fmt.Fprint(os.Stdout, "[]")
		return 2
	case "record-and-hang":
		if err := os.WriteFile(os.Getenv(pidFileEnvironment), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			return 97
		}
		time.Sleep(time.Minute)
	case "orphan-parent":
		// The program that isolates a lane: its lane process hangs, and the test kills this one.
		if _, err := isolated("record-and-hang", generous).Run(hereAndNow); err != nil {
			fmt.Fprintln(os.Stderr, err)

			return 96
		}
	case "grandchild":
		// A process of its own that keeps the pipes open and outlives this one.
		grandchild := exec.Command("sleep", "60")
		grandchild.Stdout, grandchild.Stderr = os.Stdout, os.Stderr
		if err := grandchild.Start(); err != nil {
			return 98
		}
		if err := os.WriteFile(os.Getenv(pidFileEnvironment), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600); err != nil {
			return 97
		}
		fmt.Fprint(os.Stdout, "[]")
	case "unknown-field":
		fmt.Fprint(os.Stdout, `[{"id":"actionsqaci.actionlint.glob","surprise":1}]`)
	case "invalid-finding":
		fmt.Fprint(os.Stdout, `[{"id":"actionsqaci.actionlint.glob","tool":"t","path":"a.yml","line":0,"col":1,"severity":"error","message":"m"}]`)
	case "exit-seven":
		return 7
	}

	return 0
}

func isolated(mode string, limits check.Limits) check.Lane {
	command := func(ctx context.Context, lane, path string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=NoSuchTestEver", lane, path)
		cmd.Env = append(os.Environ(), childEnvironment+"="+mode)

		return cmd
	}
	inner := check.Lane{
		Name:    "alpha",
		Selects: func(path string) bool { return strings.HasSuffix(path, ".yml") },
		Run: func(string) ([]finding.Finding, error) {
			return nil, errors.New("the lane must not run in this process")
		},
	}

	return check.Isolated(inner, command, limits)
}

var generous = check.Limits{Timeout: time.Minute, MaxOutput: 1 << 20, MaxStderr: 2048}

func TestAnIsolatedLaneReturnsWhatItsChildFound(t *testing.T) {
	lane := isolated("findings", generous)

	got, err := lane.Run(hereAndNow)

	if err != nil || len(got) != 1 || got[0] != someFinding(hereAndNow) {
		t.Errorf("findings = %+v, err = %v", got, err)
	}
	if lane.Name != "alpha" || !lane.Selects("x.yml") || lane.Selects("x.txt") {
		t.Error("the isolated lane keeps the name and the selection of the lane it wraps")
	}
}

func TestAnIsolatedLaneWithNothingToReportReportsNothing(t *testing.T) {
	got, err := isolated("nothing", generous).Run(hereAndNow)

	if err != nil || len(got) != 0 {
		t.Errorf("findings = %+v, err = %v", got, err)
	}
}

func TestWhatTheChildSaysWhenItsLaneFailsIsALaneFaultNotAVerdictOrALine(t *testing.T) {
	_, err := isolated("lane-error", generous).Run(hereAndNow)

	var noVerdict *check.NoVerdictError
	if err == nil || errors.As(err, &noVerdict) || !strings.Contains(err.Error(), "shellcheck could not start") || strings.ContainsAny(err.Error(), "\n") {
		t.Errorf("error = %q; the lane's own failure is a fault, in one line", err)
	}
}

func TestAChildThatDiesWithoutAVerdictLeavesNoVerdict(t *testing.T) {
	cases := map[string]struct {
		mode    string
		limits  check.Limits
		mention string
	}{
		"a runtime fatal error":  {"crash", generous, "exit status 2"},
		"a signal":               {"killed", generous, "signal"},
		"another exit code":      {"exit-seven", generous, "exit status 7"},
		"findings, then a crash": {"findings-then-crash", generous, "exit status 2"},
		"too long a time":        {"hang", check.Limits{Timeout: 300 * time.Millisecond, MaxOutput: 1 << 20, MaxStderr: 2048}, "time limit"},
		"too much output":        {"flood", check.Limits{Timeout: time.Minute, MaxOutput: 1024, MaxStderr: 2048}, "more than"},
	}

	for name, c := range cases {
		started := time.Now()

		got, err := isolated(c.mode, c.limits).Run(hereAndNow)

		var noVerdict *check.NoVerdictError
		if !errors.As(err, &noVerdict) || got != nil {
			t.Errorf("%s: findings %+v, error %v; want a NoVerdictError", name, got, err)

			continue
		}
		if !strings.Contains(err.Error(), c.mention) || noVerdict.Lane != "alpha" || noVerdict.Path != hereAndNow {
			t.Errorf("%s: error %q should mention %q and name the lane and file", name, err.Error(), c.mention)
		}
		if time.Since(started) > 20*time.Second {
			t.Errorf("%s: took %s; the time limit did not stop the child", name, time.Since(started))
		}
	}
}

func TestWhatTheChildWroteToStandardErrorIsKeptShortAndOnOneLine(t *testing.T) {
	_, err := isolated("crash", generous).Run(hereAndNow)

	if err == nil || strings.ContainsAny(err.Error(), "\n\x1b") || len(err.Error()) > 4096 || !strings.Contains(err.Error(), "fatal error: stack overflow") {
		t.Errorf("error (%d bytes) = %.300q", len(err.Error()), err)
	}
}

func TestAChildThatBreaksTheProtocolIsAFaultOfTheToolchain(t *testing.T) {
	for _, mode := range []string{"garbage", "unknown-field", "invalid-finding", "null"} {
		got, err := isolated(mode, generous).Run(hereAndNow)

		var noVerdict *check.NoVerdictError
		if err == nil || got != nil || errors.As(err, &noVerdict) {
			t.Errorf("%s: findings %+v, error %v; output a lane cannot have written is a fault", mode, got, err)
		}
	}
}

// gone says whether no live process has this id: it is not there, or it is a zombie that only
// waits to be reaped.
func gone(t *testing.T, pid int) bool {
	t.Helper()

	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return true
	}
	if err != nil {
		t.Fatal(err)
	}
	// The state is the field after the command name, which is in parentheses.
	afterName := string(stat[strings.LastIndex(string(stat), ")")+2:])

	return strings.HasPrefix(afterName, "Z")
}

func TestAProcessTheLaneProcessLeftBehindDiesWithIt(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Setenv(pidFileEnvironment, pidFile)

	_, runErr := isolated("grandchild", generous).Run(hereAndNow)

	written, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the lane process did not start its own process (run error: %v): %v", runErr, err)
	}
	pid, err := strconv.Atoi(string(written))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !gone(t, pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !gone(t, pid) {
		if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
			t.Error(killErr)
		}
		t.Errorf("process %d, started by the lane process, is still running after the lane ended", pid)
	}
}

func TestALaneProcessDoesNotOutliveTheProgramThatStartedIt(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "lane.pid")
	t.Setenv(pidFileEnvironment, pidFile)

	program := exec.Command(os.Args[0], "-test.run=NoSuchTestEver")
	program.Env = append(os.Environ(), childEnvironment+"=orphan-parent")
	if err := program.Start(); err != nil {
		t.Fatal(err)
	}

	pid := 0
	deadline := time.Now().Add(20 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		written, err := os.ReadFile(pidFile)
		if err == nil {
			if parsed, parseErr := strconv.Atoi(string(written)); parseErr == nil {
				pid = parsed
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == 0 {
		if err := program.Process.Kill(); err != nil {
			t.Error(err)
		}
		t.Fatal("the lane process never started")
	}

	if err := program.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var exit *exec.ExitError
	if err := program.Wait(); !errors.As(err, &exit) {
		t.Fatalf("the program that was killed ended with %v", err)
	}

	deadline = time.Now().Add(10 * time.Second)
	for !gone(t, pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !gone(t, pid) {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Error(err)
		}
		t.Errorf("lane process %d is still running after the program that started it was killed", pid)
	}
}

func TestACommandThatCannotBeStartedIsAFaultOfTheToolchain(t *testing.T) {
	command := func(ctx context.Context, lane, path string) *exec.Cmd {
		return exec.CommandContext(ctx, "/nonexistent/actions-qa-child")
	}
	lane := check.Isolated(check.Lane{Name: "alpha", Selects: func(string) bool { return true }}, command, generous)

	got, err := lane.Run(hereAndNow)

	var noVerdict *check.NoVerdictError
	if err == nil || got != nil || errors.As(err, &noVerdict) {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestRunCarriesTheNoVerdictOfAnIsolatedLaneToItsCaller(t *testing.T) {
	_, err := check.Run([]check.Lane{isolated("crash", generous)}, []string{hereAndNow})

	var noVerdict *check.NoVerdictError
	var laneErr *check.LaneError
	if !errors.As(err, &noVerdict) || !errors.As(err, &laneErr) {
		t.Errorf("error = %v; it should be a lane error that holds a no verdict", err)
	}
}

func TestServeWritesTheFindingsAsJSONAndReportsAFailureOnStandardError(t *testing.T) {
	var stdout, stderr strings.Builder
	good := check.Lane{Name: "x", Selects: func(string) bool { return true }, Run: func(path string) ([]finding.Finding, error) {
		return []finding.Finding{someFinding(path)}, nil
	}}

	if code := check.Serve(good, "a.yml", &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), `"path": "a.yml"`) || stderr.Len() != 0 {
		t.Errorf("good: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	empty := check.Lane{Name: "x", Selects: func(string) bool { return true }, Run: func(string) ([]finding.Finding, error) { return nil, nil }}
	if code := check.Serve(empty, "a.yml", &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) != "[]" {
		t.Errorf("empty: exit %d, stdout %q", code, stdout.String())
	}

	bad := check.Lane{Name: "x", Selects: func(string) bool { return true }, Run: func(string) ([]finding.Finding, error) { return nil, errors.New("boom") }}
	stdout.Reset()
	if code := check.Serve(bad, "a.yml", &stdout, &stderr); code != check.ChildFault || stdout.Len() != 0 || !strings.Contains(stderr.String(), "boom") {
		t.Errorf("bad: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
