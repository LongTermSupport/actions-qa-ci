package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseChangelog = "# Changelog\n\n## Unreleased\n\n### Added\n\n- A feature.\n\n## 0.1.0 — 2026-10-01\n\n### Added\n\n- The first thing.\n"

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}

	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}

	return string(content)
}

func TestReleaseNextPrintsTheVersion(t *testing.T) {
	changelog := writeFile(t, filepath.Join(t.TempDir(), "CHANGELOG.md"), releaseChangelog)

	got := invoke("release", "next", "--changelog", changelog)
	if got.code != 0 || got.stdout != "0.2.0\n" {
		t.Fatalf("got %+v, want exit 0 and 0.2.0", got)
	}
}

func TestReleaseNextPrintsNothingWhenNothingIsUnreleased(t *testing.T) {
	changelog := writeFile(t, filepath.Join(t.TempDir(), "CHANGELOG.md"), "# Changelog\n\n## Unreleased\n\n## 0.1.0 — 2026-10-01\n\n- x\n")

	got := invoke("release", "next", "--changelog", changelog)
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("got %+v, want exit 0 and no output", got)
	}
}

func TestReleaseCutRewritesTheChangelogAndWritesVersion(t *testing.T) {
	dir := t.TempDir()
	changelog := writeFile(t, filepath.Join(dir, "CHANGELOG.md"), releaseChangelog)
	version := filepath.Join(dir, "VERSION")

	got := invoke("release", "cut", "--changelog", changelog, "--date", "2026-11-18", "--version-file", version)
	if got.code != 0 || got.stdout != "0.2.0\n" {
		t.Fatalf("got %+v, want exit 0 and 0.2.0", got)
	}
	if text := readFile(t, changelog); !strings.Contains(text, "## Unreleased\n\n## 0.2.0 — 2026-11-18\n\n### Added\n\n- A feature.\n") {
		t.Fatalf("the changelog was not cut:\n%s", text)
	}
	if text := readFile(t, version); text != "0.2.0\n" {
		t.Fatalf("VERSION = %q, want 0.2.0", text)
	}
}

func TestReleaseCutRefusesWhenNothingIsUnreleased(t *testing.T) {
	dir := t.TempDir()
	original := "# Changelog\n\n## Unreleased\n\n## 0.1.0 — 2026-10-01\n\n- x\n"
	changelog := writeFile(t, filepath.Join(dir, "CHANGELOG.md"), original)
	version := filepath.Join(dir, "VERSION")

	got := invoke("release", "cut", "--changelog", changelog, "--date", "2026-11-18", "--version-file", version)
	if got.code != 3 || !strings.Contains(got.stderr, "nothing to release") {
		t.Fatalf("got %+v, want exit 3 saying there is nothing to release", got)
	}
	if text := readFile(t, changelog); text != original {
		t.Fatalf("a refused cut changed the changelog:\n%s", text)
	}
	if _, err := os.Stat(version); !os.IsNotExist(err) {
		t.Fatalf("a refused cut wrote VERSION (stat: %v)", err)
	}
}

func TestReleasePendingReadsTheTags(t *testing.T) {
	dir := t.TempDir()
	cut := strings.Replace(releaseChangelog, "## Unreleased\n", "## Unreleased\n\n## 0.2.0 — 2026-11-18\n", 1)
	changelog := writeFile(t, filepath.Join(dir, "CHANGELOG.md"), cut)

	untagged := writeFile(t, filepath.Join(dir, "tags-untagged"), "v0.1.0\n")
	got := invoke("release", "pending", "--changelog", changelog, "--tags", untagged)
	if got.code != 0 || got.stdout != "0.2.0\n" {
		t.Fatalf("got %+v, want exit 0 and 0.2.0", got)
	}

	tagged := writeFile(t, filepath.Join(dir, "tags-tagged"), "v0.1.0\nv0.2.0\n")
	got = invoke("release", "pending", "--changelog", changelog, "--tags", tagged)
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("got %+v, want exit 0 and no output", got)
	}
}

func TestReleaseNotesPrintsTheSection(t *testing.T) {
	changelog := writeFile(t, filepath.Join(t.TempDir(), "CHANGELOG.md"), releaseChangelog)

	got := invoke("release", "notes", "--changelog", changelog, "--version", "0.1.0")
	if got.code != 0 || got.stdout != "### Added\n\n- The first thing.\n" {
		t.Fatalf("got %+v, want the 0.1.0 section", got)
	}
	got = invoke("release", "notes", "--changelog", changelog, "--version", "0.9.0")
	if got.code != 3 || !strings.Contains(got.stderr, "0.9.0") {
		t.Fatalf("got %+v, want exit 3 naming 0.9.0", got)
	}
}

func TestReleaseUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"release"},
		{"release", "publish"},
		{"release", "next"},
		{"release", "cut", "--changelog", "CHANGELOG.md"},
		{"release", "pending", "--changelog", "CHANGELOG.md"},
		{"release", "notes", "--changelog", "CHANGELOG.md"},
		{"release", "next", "--changelog", "CHANGELOG.md", "extra"},
	} {
		got := invoke(args...)
		if got.code != 2 || !strings.Contains(got.stderr, "release") {
			t.Errorf("%v: got %+v, want exit 2 and the release usage", args, got)
		}
	}
}

func TestFetchRefusesAPlatformTheManifestDoesNotPin(t *testing.T) {
	got := invoke("fetch", "--platform", "plan9_mips", t.TempDir())
	if got.code != 3 || !strings.Contains(got.stderr, "plan9_mips") {
		t.Fatalf("got %+v, want exit 3 naming the platform", got)
	}
}
