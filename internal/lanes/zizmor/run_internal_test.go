package zizmor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
)

var testLimits = limits{timeout: 10 * time.Second, maxOutput: 1 << 20, maxStderr: 4 << 10}

// standIn writes an executable shell script that stands for zizmor, and returns its path and
// the path of the file the script may write its record to.
func standIn(t *testing.T, body string) (binary, record string) {
	t.Helper()

	dir := t.TempDir()
	record = filepath.Join(dir, "record")
	binary = filepath.Join(dir, "zizmor")
	script := "#!/bin/sh\nrecord='" + record + "'\n" + body + "\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(binary, 0o700); err != nil {
		t.Fatal(err)
	}

	return binary, record
}

// standInBinary is a stand-in that keeps no record.
func standInBinary(t *testing.T, body string) string {
	t.Helper()

	binary, record := standIn(t, body)
	if record == "" {
		t.Fatal("a stand-in has no record path")
	}

	return binary
}

func TestZizmorIsAskedForJSONOfOneFileWithNothingOfTheProjectsOwnApplied(t *testing.T) {
	binary, record := standIn(t, `printf '%s\n' "$@" > "$record"; echo '[]'`)

	found, err := lint(binary, reportedPath, "regular", testLimits)

	if err != nil || len(found) != 0 {
		t.Fatalf("got %+v, %v", found, err)
	}
	written, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(written)), "\n")
	for _, want := range []string{"--format=json-v1", "--no-config", "--no-ignores", "--offline", "--strict-collection", "--no-exit-codes", "--persona=regular", "--no-progress", "--color=never"} {
		if !slices.Contains(args, want) {
			t.Errorf("zizmor was not given %s; its arguments were %q", want, args)
		}
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != reportedPath {
		t.Errorf("the file must come last, after --; the arguments were %q", args)
	}
}

func TestZizmorRunsWithNothingOfTheCallersEnvironment(t *testing.T) {
	t.Setenv("GH_TOKEN", "a-token")
	t.Setenv("GITHUB_TOKEN", "a-token")
	t.Setenv("ZIZMOR_CONFIG", "/nowhere")
	t.Setenv("ZIZMOR_GITHUB_TOKEN", "a-token")
	t.Setenv("ZIZMOR_OFFLINE", "0")
	binary, record := standIn(t, `env > "$record"; echo '[]'`)

	if _, err := lint(binary, reportedPath, "regular", testLimits); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "ZIZMOR_", "HOME="} {
		if strings.Contains(string(written), name) {
			t.Errorf("zizmor ran with %s in its environment:\n%s", name, written)
		}
	}
}

func TestAnExitCodeOfZizmorsOwnIsAFaultWithItsWordsAndNeverACleanRun(t *testing.T) {
	for _, code := range []string{"1", "2", "3", "101"} {
		binary := standInBinary(t, `echo "error: cannot read the file" >&2; exit `+code)

		found, err := lint(binary, reportedPath, "regular", testLimits)

		var ended *check.NoVerdictError
		if err == nil || errors.As(err, &ended) || !strings.Contains(err.Error(), "cannot read the file") || found != nil {
			t.Errorf("exit %s: got %+v, %v", code, found, err)
		}
	}
}

func TestAZizmorKilledByASignalOrEndedLikeOneHasNoVerdict(t *testing.T) {
	cases := map[string]string{
		"killed":          `kill -9 $$`,
		"a segmentation":  `kill -SEGV $$`,
		"exit 139":        `exit 139`,
		"terminated":      `kill -TERM $$`,
		"aborted":         `kill -ABRT $$`,
		"a bus error":     `kill -BUS $$`,
		"killed (stderr)": `echo "something" >&2; kill -9 $$`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			binary := standInBinary(t, body)

			found, err := lint(binary, reportedPath, "regular", testLimits)

			var ended *check.NoVerdictError
			if !errors.As(err, &ended) || found != nil {
				t.Errorf("got %+v, %v; want no verdict", found, err)
			}
		})
	}
}

func TestAZizmorThatDoesNotFinishInTimeHasNoVerdictAndWhatItStartedIsEnded(t *testing.T) {
	binary, record := standIn(t, `sleep 30 & echo $! > "$record"; wait`)
	short := testLimits
	short.timeout = 300 * time.Millisecond

	start := time.Now()
	found, err := lint(binary, reportedPath, "regular", short)

	var ended *check.NoVerdictError
	if !errors.As(err, &ended) || found != nil || !strings.Contains(ended.Reason, "time limit") {
		t.Fatalf("got %+v, %v; want no verdict for the time limit", found, err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("it took %s to give up", time.Since(start))
	}
	pid, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/proc/" + strings.TrimSpace(string(pid))); err == nil {
		t.Errorf("the process %s that zizmor started is still running", strings.TrimSpace(string(pid)))
	}
}

func TestAZizmorThatWritesMoreThanItMayHasNoVerdict(t *testing.T) {
	binary := standInBinary(t, `yes '[]' | head -c 3000000; exit 0`)
	small := testLimits
	small.maxOutput = 1 << 20

	found, err := lint(binary, reportedPath, "regular", small)

	var ended *check.NoVerdictError
	if !errors.As(err, &ended) || found != nil || !strings.Contains(ended.Reason, "more than") {
		t.Errorf("got %+v, %v; want no verdict for the size", found, err)
	}
}

func TestWhatZizmorPrintsThatIsNotAReportIsAFault(t *testing.T) {
	for name, body := range map[string]string{
		"nothing":   `exit 0`,
		"text":      `echo 'all fine'`,
		"an object": `echo '{}'`,
	} {
		t.Run(name, func(t *testing.T) {
			binary := standInBinary(t, body)

			found, err := lint(binary, reportedPath, "regular", testLimits)

			var ended *check.NoVerdictError
			if err == nil || errors.As(err, &ended) || found != nil {
				t.Errorf("got %+v, %v; want a fault", found, err)
			}
		})
	}
}

func TestAMissingZizmorIsAFault(t *testing.T) {
	found, err := lint(filepath.Join(t.TempDir(), "zizmor"), reportedPath, "regular", testLimits)

	if err == nil || found != nil {
		t.Errorf("got %+v, %v", found, err)
	}
}
