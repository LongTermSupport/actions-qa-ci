package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func checksumsText() string {
	return strings.Repeat("a", 64) + "  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n" +
		strings.Repeat("b", 64) + "  actions-qa-ci_1.2.3_linux_arm64.tar.gz\n"
}

// scriptRun is the arguments that fill the script from a checksums file, and where it goes.
type scriptRun struct {
	args []string
	out  string
}

func installScript(t *testing.T, checksums string) scriptRun {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(path, []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out", "install.sh")

	return scriptRun{args: []string{"install-script", "--version", "1.2.3", "--checksums", path, "--out", out}, out: out}
}

func TestInstallScriptWritesTheFilledScriptForTheReleaseAndNamesIt(t *testing.T) {
	previous := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(previous) })
	run := installScript(t, checksumsText())

	got := invoke(run.args...)

	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, run.out) {
		t.Fatalf("got %+v", got)
	}
	script, err := os.ReadFile(run.out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "VERSION='1.2.3'") || !strings.Contains(string(script), strings.Repeat("a", 64)) {
		t.Error("the script is not filled for the release")
	}
	info, err := os.Stat(run.out)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("the script: %v, %v; it is run with sh and should be executable", info, err)
	}
}

func TestInstallScriptNeedsEveryOptionAndRefusesAnUnknownOne(t *testing.T) {
	run := installScript(t, checksumsText())
	options := run.args[1:]

	for i := 0; i < len(options); i += 2 {
		without := append(append([]string{"install-script"}, options[:i]...), options[i+2:]...)
		got := invoke(without...)

		if got.code != 2 || !strings.Contains(got.stderr, options[i]+" is needed") {
			t.Errorf("without %s: got %+v, want exit 2 naming it", options[i], got)
		}
	}
	if got := invoke(append(append([]string{}, run.args...), "extra")...); got.code != 2 {
		t.Errorf("a stray argument: got %+v, want exit 2", got)
	}
}

func TestInstallScriptFromChecksumsThatCannotVerifyWritesNothing(t *testing.T) {
	run := installScript(t, strings.Repeat("a", 64)+"  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n")

	got := invoke(run.args...)

	if got.code != 3 || !strings.Contains(got.stderr, "linux_arm64") {
		t.Errorf("got %+v, want exit 3 naming the missing platform", got)
	}
	if _, err := os.Stat(run.out); !os.IsNotExist(err) {
		t.Errorf("a script was written: %v", err)
	}
}

func TestInstallScriptDoesNotReplaceAScriptAlreadyThere(t *testing.T) {
	run := installScript(t, checksumsText())

	first := invoke(run.args...)
	second := invoke(run.args...)

	if first.code != 0 || second.code != 3 || !strings.Contains(second.stderr, "exists") {
		t.Errorf("first %+v, second %+v", first, second)
	}
}

func TestInstallScriptReadsOnlyARegularChecksumsFileOfReasonableSize(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "install.sh")
	big := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 2<<20)), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, checksums := range map[string]string{"a directory": dir, "a missing file": filepath.Join(dir, "none"), "a large file": big} {
		got := invoke("install-script", "--version", "1.2.3", "--checksums", checksums, "--out", out)

		if got.code != 3 {
			t.Errorf("%s: got %+v, want exit 3", name, got)
		}
	}
}
