package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)

	return code, stdout.String(), stderr.String()
}

func TestVersionPrintsTheConformanceDeclarationAsJSON(t *testing.T) {
	code, stdout, stderr := run(t, "version", "--json")

	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitOK, stderr)
	}

	var printed actionsqaci.Declaration
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("version --json is not the declaration: %v\n%s", err, stdout)
	}

	embedded, err := actionsqaci.LoadDeclaration()
	if err != nil {
		t.Fatal(err)
	}
	if printed.Method != embedded.Method || printed.Toolchain != embedded.Toolchain {
		t.Errorf("printed %q/%q, want the embedded %q/%q", printed.Method, printed.Toolchain, embedded.Method, embedded.Toolchain)
	}
	if len(printed.KnownGaps) != len(embedded.KnownGaps) {
		t.Errorf("printed %d known gaps, want the embedded %d", len(printed.KnownGaps), len(embedded.KnownGaps))
	}
}

func TestVersionInTextNamesTheSpecificationsItIsAuditedAgainst(t *testing.T) {
	code, stdout, stderr := run(t, "version")

	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitOK, stderr)
	}
	for _, want := range []string{"actions-qa", "method 1.1.0", "toolchain 0.2.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("version output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestNoArgumentsIsAUsageErrorThatAnalysedNothing(t *testing.T) {
	code, stdout, stderr := run(t)

	if code != cli.ExitUsage {
		t.Errorf("exit = %d, want %d", code, cli.ExitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing: a usage error is not a verdict", stdout)
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr should say how to use the program:\n%s", stderr)
	}
}

func TestAnUnknownCommandIsAUsageErrorNamingIt(t *testing.T) {
	code, stdout, stderr := run(t, "frobnicate")

	if code != cli.ExitUsage {
		t.Errorf("exit = %d, want %d", code, cli.ExitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "frobnicate") {
		t.Errorf("stderr should name the unknown command:\n%s", stderr)
	}
}

func TestAnUnknownFlagOnAKnownCommandIsAUsageError(t *testing.T) {
	code, stdout, stderr := run(t, "version", "--xml")

	if code != cli.ExitUsage {
		t.Errorf("exit = %d, want %d", code, cli.ExitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "--xml") {
		t.Errorf("stderr should name the unknown flag:\n%s", stderr)
	}
}

func TestHelpIsNotAnError(t *testing.T) {
	for _, flag := range []string{"help", "--help", "-h"} {
		code, stdout, stderr := run(t, flag)

		if code != cli.ExitOK {
			t.Errorf("%s: exit = %d, want %d; stderr: %s", flag, code, cli.ExitOK, stderr)
		}
		if !strings.Contains(stdout, "usage:") {
			t.Errorf("%s: stdout should say how to use the program:\n%s", flag, stdout)
		}
	}
}

func TestTheExitCodesAreTheDocumentedOnes(t *testing.T) {
	// design 7.2: 0 clean, 1 findings, 2 usage or record error, 3 toolchain fault, 4 no verdict.
	got := []int{cli.ExitOK, cli.ExitFindings, cli.ExitUsage, cli.ExitToolchainFault, cli.ExitNoVerdict}
	want := []int{0, 1, 2, 3, 4}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("exit code %d is %d", want[i], got[i])
		}
	}
}
