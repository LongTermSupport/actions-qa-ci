package installscript_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/bundle"
	"github.com/LongTermSupport/actions-qa-ci/internal/installscript"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const version = "1.2.3"

func digest(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}

// hostPlatform is the platform of the machine the tests run on, in the manifest's form.
func hostPlatform(t *testing.T) string {
	t.Helper()

	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skipf("the install script supports linux amd64 and arm64, not %s %s", runtime.GOOS, runtime.GOARCH)
	}

	return "linux_" + runtime.GOARCH
}

// release is a built bundle for the host and a filled-in script for it.
type release struct {
	bundle    bundle.Bundle
	script    string
	checksums string
}

func newRelease(t *testing.T) release {
	t.Helper()

	return newReleaseFor(t, hostPlatform(t))
}

// newReleaseFor is a built bundle for a platform and the script filled in for it, so the script
// can be run as if on that platform with a uname that says so.
func newReleaseFor(t *testing.T, platform string) release {
	t.Helper()

	dir := t.TempDir()
	write := func(path, content string) string {
		t.Helper()

		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}

		return full
	}

	program := write("build/actions-qa", "the program")
	write("tools/shellcheck", "a tool")
	write("licences/actions-qa-ci.txt", "project licence")
	write("licences/shellcheck.txt", "tool licence")

	built, err := bundle.Build(bundle.Options{
		Manifest: toolchain.Manifest{Tools: map[string]toolchain.Tool{
			"shellcheck": {Version: "v1", Assets: map[string]toolchain.Asset{platform: {BinarySHA256: digest([]byte("a tool"))}}},
		}},
		Platform: platform,
		Version:  version,
		Program:  program,
		ToolDirs: []string{filepath.Join(dir, "tools")},
		Licences: filepath.Join(dir, "licences"),
	})
	if err != nil {
		t.Fatal(err)
	}

	other := "linux_amd64"
	if platform == other {
		other = "linux_arm64"
	}
	checksums := bundle.Checksums([]bundle.Bundle{
		built,
		{Name: "actions-qa-ci_" + version + "_" + other + ".tar.gz", SHA256: strings.Repeat("0", 64)},
	})
	filled, err := installscript.Fill(version, checksums)
	if err != nil {
		t.Fatal(err)
	}

	return release{bundle: built, script: write("install.sh", string(filled)), checksums: checksums}
}

type outcome struct {
	code           int
	stdout, stderr string
}

// run runs the script with sh and an environment of its own: a home, and only the variables
// given.
func run(t *testing.T, script, home string, env []string, args ...string) outcome {
	t.Helper()

	command := exec.Command("sh", append([]string{script}, args...)...)
	command.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr

	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func writeBundle(t *testing.T, r release) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), r.bundle.Name)
	if err := os.WriteFile(path, r.bundle.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestFillPutsTheVersionAndBothChecksumsIntoTheScript(t *testing.T) {
	checksums := strings.Repeat("a", 64) + "  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n" +
		strings.Repeat("b", 64) + "  actions-qa-ci_1.2.3_linux_arm64.tar.gz\n"

	filled, err := installscript.Fill("1.2.3", checksums)
	if err != nil {
		t.Fatal(err)
	}

	text := string(filled)
	for _, want := range []string{"VERSION='1.2.3'", "SHA256_LINUX_AMD64='" + strings.Repeat("a", 64) + "'", "SHA256_LINUX_ARM64='" + strings.Repeat("b", 64) + "'"} {
		if !strings.Contains(text, want) {
			t.Errorf("the script does not hold %s", want)
		}
	}
	for _, placeholder := range []string{"@VERSION@", "@SHA256_LINUX_AMD64@", "@SHA256_LINUX_ARM64@"} {
		if strings.Contains(text, placeholder) {
			t.Errorf("the placeholder %s is left in the script", placeholder)
		}
	}
}

func TestFillRefusesWhatWouldMakeAScriptThatCannotVerify(t *testing.T) {
	amd := strings.Repeat("a", 64) + "  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n"
	arm := strings.Repeat("b", 64) + "  actions-qa-ci_1.2.3_linux_arm64.tar.gz\n"
	cases := map[string]struct{ version, checksums string }{
		"no arm64 line":           {"1.2.3", amd},
		"no amd64 line":           {"1.2.3", arm},
		"a short checksum":        {"1.2.3", "abc  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n" + arm},
		"an upper-case checksum":  {"1.2.3", strings.Repeat("A", 64) + "  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n" + arm},
		"a checksum with a quote": {"1.2.3", strings.Repeat("a", 63) + "'  actions-qa-ci_1.2.3_linux_amd64.tar.gz\n" + arm},
		"another version":         {"1.2.4", amd + arm},
		"a v in the version":      {"v1.2.3", amd + arm},
		"a version with a quote":  {"1.2.3'; rm -rf /; '", amd + arm},
		"a line of two files":     {"1.2.3", amd + amd + arm},
		"no checksums":            {"1.2.3", ""},
	}

	for name, c := range cases {
		if filled, err := installscript.Fill(c.version, c.checksums); err == nil {
			t.Errorf("%s: filled a script: %q", name, filled)
		}
	}
}

func TestTheScriptInstallsAVerifiedBundleUnderTheHomeAndSaysWhere(t *testing.T) {
	r := newRelease(t)
	home := t.TempDir()

	got := run(t, r.script, home, []string{"ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)})

	dest := filepath.Join(home, ".local/share/actions-qa-ci", version)
	if got.code != 0 {
		t.Fatalf("%+v", got)
	}
	program, err := os.ReadFile(filepath.Join(dest, "bin/actions-qa"))
	if err != nil || string(program) != "the program" {
		t.Errorf("the program: %q, %v", program, err)
	}
	if info, err := os.Stat(filepath.Join(dest, "bin/actions-qa")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the program is not executable: %v, %v", info, err)
	}
	if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("the install directory: %v, %v; it must be readable by everyone who may run the program, not the 0700 of the staging directory", info, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "LICENSES/actions-qa-ci.txt")); err != nil {
		t.Errorf("the licence was not installed: %v", err)
	}
	if !strings.Contains(got.stdout, filepath.Join(dest, "bin")) {
		t.Errorf("stdout %q does not say where the program is", got.stdout)
	}
}

func TestTheInstallLocationFollowsTheEnvironmentAndThePrefix(t *testing.T) {
	r := newRelease(t)
	bundlePath := writeBundle(t, r)

	cases := map[string]struct {
		env  func(string) []string
		args []string
		dest func(string) string
	}{
		"ACTIONS_QA_CI_HOME": {
			env: func(root string) []string {
				return []string{"ACTIONS_QA_CI_HOME=" + root + "/own", "XDG_DATA_HOME=" + root + "/xdg"}
			},
			dest: func(root string) string { return root + "/own/" + version },
		},
		"XDG_DATA_HOME": {
			env:  func(root string) []string { return []string{"XDG_DATA_HOME=" + root + "/xdg"} },
			dest: func(root string) string { return root + "/xdg/actions-qa-ci/" + version },
		},
		"--prefix": {
			env:  func(root string) []string { return []string{"ACTIONS_QA_CI_HOME=" + root + "/own"} },
			args: nil,
			dest: func(root string) string { return root + "/prefix" },
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			args := c.args
			if name == "--prefix" {
				args = []string{"--prefix", root + "/prefix"}
			}

			got := run(t, r.script, t.TempDir(), append(c.env(root), "ACTIONS_QA_CI_BUNDLE="+bundlePath), args...)

			if got.code != 0 {
				t.Fatalf("%+v", got)
			}
			if _, err := os.Stat(filepath.Join(c.dest(root), "bin/actions-qa")); err != nil {
				t.Errorf("not installed where expected: %v", err)
			}
		})
	}
}

func TestABundleThatDoesNotMatchTheChecksumInstallsNothingAndFails(t *testing.T) {
	r := newRelease(t)
	home := t.TempDir()
	tampered := append(append([]byte{}, r.bundle.Data...), 0)
	path := filepath.Join(t.TempDir(), r.bundle.Name)
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	got := run(t, r.script, home, []string{"ACTIONS_QA_CI_BUNDLE=" + path})

	if got.code == 0 || !strings.Contains(got.stderr, "checksum") {
		t.Errorf("%+v; a bundle that differs from its checksum must fail and say so", got)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("something was installed or left behind in the home: %v, %v", entries, err)
	}
}

func TestAnExistingInstallIsNotReplaced(t *testing.T) {
	r := newRelease(t)
	home := t.TempDir()
	env := []string{"ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}

	first := run(t, r.script, home, env)
	second := run(t, r.script, home, env)

	if first.code != 0 || second.code == 0 || !strings.Contains(second.stderr, "already") {
		t.Errorf("first %+v, second %+v", first, second)
	}
}

func TestTheChecksumIsTakenWithWhicheverToolTheHostHas(t *testing.T) {
	r := newRelease(t)
	bundlePath := writeBundle(t, r)

	for _, tool := range []string{"sha256sum", "shasum", "openssl"} {
		t.Run(tool, func(t *testing.T) {
			path := restrictedPath(t, tool)
			home := t.TempDir()
			command := exec.Command(shell(t), r.script)
			command.Env = []string{"HOME=" + home, "PATH=" + path, "ACTIONS_QA_CI_BUNDLE=" + bundlePath}
			var stderr bytes.Buffer
			command.Stderr = &stderr

			if err := command.Run(); err != nil {
				t.Fatalf("with only %s: %v\n%s", tool, err, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(home, ".local/share/actions-qa-ci", version, "bin/actions-qa")); err != nil {
				t.Errorf("not installed with only %s: %v", tool, err)
			}
		})
	}
}

func TestWithNoChecksumToolTheScriptFailsAndInstallsNothing(t *testing.T) {
	r := newRelease(t)
	home := t.TempDir()
	command := exec.Command(shell(t), r.script)
	command.Env = []string{"HOME=" + home, "PATH=" + restrictedPath(t, ""), "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}
	var stderr bytes.Buffer
	command.Stderr = &stderr

	err := command.Run()

	if err == nil || !strings.Contains(stderr.String(), "sha256sum") {
		t.Errorf("error %v, stderr %q; a host that cannot verify must not install", err, stderr.String())
	}
	if entries, readErr := os.ReadDir(home); readErr != nil || len(entries) != 0 {
		t.Errorf("something was installed: %v, %v", entries, readErr)
	}
}

func TestAPlatformTheScriptDoesNotSupportIsNamedAndRefused(t *testing.T) {
	r := newRelease(t)
	for _, c := range []struct{ system, machine string }{{"Darwin", "arm64"}, {"Linux", "riscv64"}, {"FreeBSD", "amd64"}} {
		home := t.TempDir()
		shim := t.TempDir()
		fake := "#!/bin/sh\ncase \"$1\" in -s) echo " + c.system + " ;; -m) echo " + c.machine + " ;; *) echo " + c.system + " ;; esac\n"
		if err := os.WriteFile(filepath.Join(shim, "uname"), []byte(fake), 0o755); err != nil {
			t.Fatal(err)
		}

		got := run(t, r.script, home, []string{"PATH=" + shim + ":" + os.Getenv("PATH"), "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)})

		if got.code == 0 || !strings.Contains(got.stderr, c.system) {
			t.Errorf("%s %s: %+v; the refusal must name the platform", c.system, c.machine, got)
		}
	}
}

func TestTheScriptDownloadsFromTheGivenAddressWithCurlOrWget(t *testing.T) {
	r := newRelease(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/"+r.bundle.Name {
			http.NotFound(w, req)

			return
		}
		if _, err := w.Write(r.bundle.Data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)

	for _, tool := range []string{"curl", "wget"} {
		t.Run(tool, func(t *testing.T) {
			path := restrictedPath(t, "sha256sum", tool)
			home := t.TempDir()
			command := exec.Command(shell(t), r.script)
			command.Env = []string{"HOME=" + home, "PATH=" + path, "ACTIONS_QA_CI_DOWNLOAD_URL=" + server.URL}
			var stderr bytes.Buffer
			command.Stderr = &stderr

			if err := command.Run(); err != nil {
				t.Fatalf("with only %s: %v\n%s", tool, err, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(home, ".local/share/actions-qa-ci", version, "bin/actions-qa")); err != nil {
				t.Errorf("not installed with only %s: %v", tool, err)
			}
		})
	}
}

func TestADownloadThatFailsInstallsNothing(t *testing.T) {
	r := newRelease(t)
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	home := t.TempDir()

	got := run(t, r.script, home, []string{"ACTIONS_QA_CI_DOWNLOAD_URL=" + server.URL})

	if got.code == 0 {
		t.Errorf("%+v", got)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("something was installed or left behind: %v, %v", entries, err)
	}
}

func TestATemplateThatWasNeverFilledRefusesToRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, installscript.Template(), 0o755); err != nil {
		t.Fatal(err)
	}

	got := run(t, path, t.TempDir(), nil)

	if got.code == 0 || !strings.Contains(got.stderr, "template") {
		t.Errorf("%+v; the template of a release must not run", got)
	}
}

func TestTheScriptPassesShellCheckAsPosixSh(t *testing.T) {
	tools := os.Getenv(toolchain.ToolsDirectoryVariable)
	if tools == "" {
		t.Skip("the pinned ShellCheck is not available: " + toolchain.ToolsDirectoryVariable + " is not set")
	}
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, installscript.Template(), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(filepath.Join(tools, "shellcheck"), "--shell=sh", "--severity=style", path)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output

	if err := command.Run(); err != nil {
		t.Errorf("shellcheck: %v\n%s", err, output.String())
	}
}

// shell is the interpreter to run the script with when PATH is restricted.
func shell(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// restrictedPath is a directory of links to the commands the script needs and only the named
// tools of the checksum and download kinds, so the script is run as on a host that has just those.
func restrictedPath(t *testing.T, only ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"uname", "mktemp", "tar", "gzip", "mkdir", "mv", "rm", "cp", "cat", "cut", "tr", "awk", "dirname", "basename", "id", "test", "printf", "chmod", "ls", "head", "sed", "grep", "date"} {
		if target, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range only {
		if name == "" {
			continue
		}
		target, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s is not installed here", name)
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}
