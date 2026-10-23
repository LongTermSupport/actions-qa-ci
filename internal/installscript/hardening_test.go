package installscript_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bundle"
	"github.com/LongTermSupport/actions-qa-ci/internal/installscript"
)

// shim writes an executable script named name into a new directory and returns the directory.
func shim(t *testing.T, name, body string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return dir
}

// filledFor is a script filled in for the checksum of the given archive content, for the host.
func filledFor(t *testing.T, platform string, archive []byte) string {
	t.Helper()

	name := "actions-qa-ci_" + version + "_" + platform + ".tar.gz"
	other := "linux_arm64"
	if platform == other {
		other = "linux_amd64"
	}
	checksums := bundle.Checksums([]bundle.Bundle{
		{Name: name, SHA256: digest(archive)},
		{Name: "actions-qa-ci_" + version + "_" + other + ".tar.gz", SHA256: strings.Repeat("0", 64)},
	})
	script, err := installscript.Fill(version, checksums)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, script, 0o755); err != nil {
		t.Fatal(err)
	}

	return path
}

func writeFile(t *testing.T, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

// tarball makes a gzip-compressed tar of the named files.
func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

func entries(t *testing.T, dir string) []string {
	t.Helper()

	listing, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range listing {
		names = append(names, item.Name())
	}

	return names
}

func TestAHashToolThatPrintsNothingIsNotAMatch(t *testing.T) {
	r := newRelease(t)
	home := t.TempDir()
	path := restrictedPath(t) + ":" + shim(t, "sha256sum", "exit 0")
	command := exec.Command(shell(t), r.script)
	command.Env = []string{"HOME=" + home, "PATH=" + path, "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}
	var stderr bytes.Buffer
	command.Stderr = &stderr

	err := command.Run()

	if err == nil || !strings.Contains(stderr.String(), "checksum") {
		t.Errorf("error %v, stderr %q; an empty answer from the checksum tool must not pass", err, stderr.String())
	}
	if names := entries(t, home); len(names) != 0 {
		t.Errorf("something was installed: %v", names)
	}
}

func TestAnInstallThatIsAlreadyThereIsNotDownloadedAgain(t *testing.T) {
	r := newRelease(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		if _, err := w.Write(r.bundle.Data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	home := t.TempDir()
	env := []string{"ACTIONS_QA_CI_DOWNLOAD_URL=" + server.URL}

	first := run(t, r.script, home, env)
	afterFirst := requests.Load()
	second := run(t, r.script, home, env)

	if first.code != 0 || second.code == 0 || requests.Load() != afterFirst {
		t.Errorf("first %+v, second %+v, requests %d then %d; a second run must stop before it downloads", first, second, afterFirst, requests.Load())
	}
}

func TestAnArchiveThatCannotBeUnpackedLeavesNothingBehind(t *testing.T) {
	garbage := []byte("not a tar file at all")
	script := filledFor(t, hostPlatform(t), garbage)
	home, temp := t.TempDir(), t.TempDir()

	got := run(t, script, home, []string{"TMPDIR=" + temp, "ACTIONS_QA_CI_BUNDLE=" + writeFile(t, garbage)})

	if got.code == 0 || !strings.Contains(got.stderr, "unpack") {
		t.Errorf("%+v; an archive that does not unpack must fail and say so", got)
	}
	if names := entries(t, temp); len(names) != 0 {
		t.Errorf("temporary files left behind: %v", names)
	}
	if names := entries(t, filepath.Join(home, ".local/share/actions-qa-ci")); len(names) != 0 {
		t.Errorf("a staging directory or install left behind: %v", names)
	}
}

func TestABundleWithoutTheProgramIsNotInstalled(t *testing.T) {
	archive := tarball(t, map[string]string{"LICENSES/actions-qa-ci.txt": "a licence"})
	script := filledFor(t, hostPlatform(t), archive)
	home, temp := t.TempDir(), t.TempDir()

	got := run(t, script, home, []string{"TMPDIR=" + temp, "ACTIONS_QA_CI_BUNDLE=" + writeFile(t, archive)})

	if got.code == 0 || !strings.Contains(got.stderr, "bin/actions-qa") {
		t.Errorf("%+v; a verified bundle with no program must not be installed", got)
	}
	if names := entries(t, filepath.Join(home, ".local/share/actions-qa-ci")); len(names) != 0 {
		t.Errorf("something was left in the install location: %v", names)
	}
	if names := entries(t, temp); len(names) != 0 {
		t.Errorf("temporary files left behind: %v", names)
	}
}

func TestATerminationSignalMidInstallLeavesNothingBehind(t *testing.T) {
	r := newRelease(t)
	home, temp := t.TempDir(), t.TempDir()
	running := filepath.Join(t.TempDir(), "tar-is-running")
	slow := shim(t, "tar", ": > "+running+"\nsleep 30")
	command := exec.Command(shell(t), r.script)
	command.Env = []string{"HOME=" + home, "PATH=" + slow + ":" + os.Getenv("PATH"), "TMPDIR=" + temp, "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	// The script is inside the unpack, and so has made both of its temporary directories, when
	// the unpack has started. A signal sent before that is held by the shell until the command it
	// is waiting for ends, which is not the case this test is about.
	parent := filepath.Join(home, ".local/share/actions-qa-ci")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(running); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(running); err != nil {
		t.Fatalf("the unpack never started: %v", err)
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("the script finished successfully although it was terminated")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the script did not stop when terminated")
	}
	if names := entries(t, temp); len(names) != 0 {
		t.Errorf("temporary files left behind: %v", names)
	}
	if names := entries(t, parent); len(names) != 0 {
		t.Errorf("a staging directory left behind: %v", names)
	}
}

// A signal that reaches the whole process group at the moment a temporary directory has been made
// must not leave the directory behind, however the script names it: the script has to know the
// name before the directory exists, or it cannot remove what it was never told about. The shims
// make the directory with the real tool and then signal the group, so the script is stopped
// exactly between the directory being made and the script learning of it.
func TestASignalTheMomentATemporaryDirectoryIsMadeLeavesNothingBehind(t *testing.T) {
	realMkdir, err := exec.LookPath("mkdir")
	if err != nil {
		t.Fatal(err)
	}
	realMktemp, err := exec.LookPath("mktemp")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"the working directory": `*"/actions-qa-ci-install."*`,
		"the staging directory": `*"/.actions-qa-ci-install."*`,
	}
	for name, pattern := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t)
			home, temp := t.TempDir(), t.TempDir()
			dir := shim(t, "mkdir", `"`+realMkdir+`" "$@" || exit
for last; do :; done
case "$last" in `+pattern+`) kill -TERM 0 ;; esac`)
			if err := os.WriteFile(filepath.Join(dir, "mktemp"), []byte(`#!/bin/sh
out=$("`+realMktemp+`" "$@") || exit
case "$out" in `+pattern+`) kill -TERM 0 ;; esac
printf '%s\n' "$out"
`), 0o755); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(shell(t), r.script)
			command.Env = []string{"HOME=" + home, "PATH=" + dir + ":" + os.Getenv("PATH"), "TMPDIR=" + temp, "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()

			select {
			case err := <-done:
				if err == nil {
					t.Error("the script finished successfully although the group was signalled")
				}
			case <-time.After(30 * time.Second):
				t.Fatal("the script did not stop when its group was signalled")
			}
			if names := entries(t, temp); len(names) != 0 {
				t.Errorf("temporary files left behind: %v", names)
			}
			// The install location is made after the working directory, so a signal at the first
			// finds it absent.
			parent := filepath.Join(home, ".local/share/actions-qa-ci")
			if _, err := os.Stat(parent); err == nil {
				if names := entries(t, parent); len(names) != 0 {
					t.Errorf("a staging directory or install left behind: %v", names)
				}
			}
		})
	}
}

func TestADirectoryNameAlreadyTakenIsRefusedAndNeverRemoved(t *testing.T) {
	realMkdir, err := exec.LookPath("mkdir")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"the working directory": `*"/actions-qa-ci-install."*`,
		"the staging directory": `*"/.actions-qa-ci-install."*`,
	}
	for name, pattern := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t)
			home, temp, record := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "taken")
			dir := shim(t, "mkdir", `for last; do :; done
case "$last" in `+pattern+`) "`+realMkdir+`" -p "$last" && : > "$last/sentinel" && printf '%s\n' "$last" > "`+record+`" ;; esac
exec "`+realMkdir+`" "$@"`)
			command := exec.Command(shell(t), r.script)
			command.Env = []string{"HOME=" + home, "PATH=" + dir + ":" + os.Getenv("PATH"), "TMPDIR=" + temp, "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}

			output, err := command.CombinedOutput()
			if err == nil {
				t.Errorf("the script finished successfully although its directory name was taken:\n%s", output)
			}
			if !strings.Contains(string(output), "cannot make the") {
				t.Errorf("the script did not say it could not make the directory:\n%s", output)
			}
			taken, err := os.ReadFile(record)
			if err != nil {
				t.Fatalf("the shim never met the directory name: %v", err)
			}
			sentinel := filepath.Join(strings.TrimSpace(string(taken)), "sentinel")
			if _, err := os.Stat(sentinel); err != nil {
				t.Errorf("the script removed a directory it did not make: %v", err)
			}
		})
	}
}

func TestEveryNameOfAMachineMapsToItsPlatformsBundle(t *testing.T) {
	cases := map[string]string{"x86_64": "linux_amd64", "amd64": "linux_amd64", "aarch64": "linux_arm64", "arm64": "linux_arm64"}

	for machine, platform := range cases {
		t.Run(machine, func(t *testing.T) {
			r := newReleaseFor(t, platform)
			home := t.TempDir()
			path := shim(t, "uname", "case \"$1\" in -s) echo Linux ;; -m) echo "+machine+" ;; esac") + ":" + os.Getenv("PATH")

			got := run(t, r.script, home, []string{"PATH=" + path, "ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)})

			if got.code != 0 {
				t.Fatalf("%+v", got)
			}
			if _, err := os.Stat(filepath.Join(home, ".local/share/actions-qa-ci", version, "bin/actions-qa")); err != nil {
				t.Errorf("not installed for %s: %v", machine, err)
			}
		})
	}
}

func TestTheScriptTakesItsArgumentsAsGiven(t *testing.T) {
	r := newRelease(t)
	bundleEnv := []string{"ACTIONS_QA_CI_BUNDLE=" + writeBundle(t, r)}

	t.Run("--prefix=DIR", func(t *testing.T) {
		prefix := filepath.Join(t.TempDir(), "into")
		got := run(t, r.script, t.TempDir(), bundleEnv, "--prefix="+prefix)

		if got.code != 0 {
			t.Fatalf("%+v", got)
		}
		if _, err := os.Stat(filepath.Join(prefix, "bin/actions-qa")); err != nil {
			t.Errorf("not installed under the prefix: %v", err)
		}
	})
	t.Run("an empty prefix", func(t *testing.T) {
		home := t.TempDir()
		for _, args := range [][]string{{"--prefix", ""}, {"--prefix="}, {"--prefix"}} {
			got := run(t, r.script, home, bundleEnv, args...)

			if got.code == 0 || !strings.Contains(got.stderr, "--prefix") {
				t.Errorf("%v: %+v; an empty prefix must not fall back to the default silently", args, got)
			}
		}
		if names := entries(t, home); len(names) != 0 {
			t.Errorf("something was installed: %v", names)
		}
	})
	t.Run("a prefix that starts with a dash", func(t *testing.T) {
		work := t.TempDir()
		command := exec.Command("sh", r.script, "--prefix", "-into")
		command.Dir = work
		command.Env = append([]string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}, bundleEnv...)
		var stderr bytes.Buffer
		command.Stderr = &stderr

		if err := command.Run(); err != nil {
			t.Fatalf("%v\n%s", err, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(work, "-into/bin/actions-qa")); err != nil {
			t.Errorf("a relative prefix is relative to where the script was run: %v", err)
		}
	})
	t.Run("an unknown argument", func(t *testing.T) {
		got := run(t, r.script, t.TempDir(), bundleEnv, "--frobnicate")

		if got.code == 0 || !strings.Contains(got.stderr, "--frobnicate") {
			t.Errorf("%+v", got)
		}
	})
	t.Run("--help", func(t *testing.T) {
		home := t.TempDir()
		got := run(t, r.script, home, nil, "--help")

		if got.code != 0 || !strings.Contains(got.stdout, "usage") || len(entries(t, home)) != 0 {
			t.Errorf("%+v", got)
		}
	})
	t.Run("a bundle that is not a file", func(t *testing.T) {
		got := run(t, r.script, t.TempDir(), []string{"ACTIONS_QA_CI_BUNDLE=" + t.TempDir()})

		if got.code == 0 || !strings.Contains(got.stderr, "ACTIONS_QA_CI_BUNDLE") {
			t.Errorf("%+v", got)
		}
	})
}
