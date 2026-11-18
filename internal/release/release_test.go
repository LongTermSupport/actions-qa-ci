package release_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/release"
)

const unreleasedOnly = `# Changelog

## Unreleased

### Added

- The first thing.

### Changed

- Something else.
`

const released = `# Changelog

## Unreleased

### Fixed

- A defect corrected.

## 0.2.0 — 2026-11-01

### Added

- A feature.

## 0.1.0 — 2026-10-01

### Added

- The first thing.
`

func TestNext(t *testing.T) {
	cases := []struct {
		name, changelog, want string
	}{
		{"no release yet is 0.1.0", unreleasedOnly, "0.1.0"},
		{"a fix before 1.0 bumps the patch", released, "0.2.1"},
		{"an addition before 1.0 bumps the minor", strings.Replace(released, "### Fixed", "### Added", 1), "0.3.0"},
		{"a removal before 1.0 bumps the minor", strings.Replace(released, "### Fixed", "### Removed", 1), "0.3.0"},
		{"a change before 1.0 bumps the patch", strings.Replace(released, "### Fixed", "### Changed", 1), "0.2.1"},
		{"a removal after 1.0 bumps the major", strings.Replace(strings.Replace(released, "### Fixed", "### Removed", 1), "0.2.0 —", "1.4.2 —", 1), "2.0.0"},
		{"an addition after 1.0 bumps the minor", strings.Replace(strings.Replace(released, "### Fixed", "### Added", 1), "0.2.0 —", "1.4.2 —", 1), "1.5.0"},
		{"a fix after 1.0 bumps the patch", strings.Replace(released, "0.2.0 —", "1.4.2 —", 1), "1.4.3"},
		{"an empty Unreleased releases nothing", "# Changelog\n\n## Unreleased\n\n### Added\n\n## 0.1.0 — 2026-10-01\n\n- x\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := release.Next([]byte(c.changelog))
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got != c.want {
				t.Fatalf("Next = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNextRefusesAMalformedChangelog(t *testing.T) {
	cases := map[string]string{
		"no Unreleased section":              "# Changelog\n\n## 0.1.0 — 2026-10-01\n\n- x\n",
		"an entry under no heading":          "# Changelog\n\n## Unreleased\n\n- loose\n",
		"an unknown heading":                 "# Changelog\n\n## Unreleased\n\n### Misc\n\n- x\n",
		"a version heading with no date":     "# Changelog\n\n## Unreleased\n\n### Added\n\n- x\n\n## 0.1.0\n",
		"a version heading not newest first": "# Changelog\n\n## Unreleased\n\n### Added\n\n- x\n\n## 0.1.0 — 2026-10-01\n\n- y\n\n## 0.2.0 — 2026-11-01\n\n- z\n",
	}
	for name, changelog := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := release.Next([]byte(changelog))
			if err == nil {
				t.Fatalf("Next = %q with no error, want a refusal", got)
			}
		})
	}
}

func TestCut(t *testing.T) {
	got, err := release.Cut([]byte(released), "0.2.1", "2026-11-18")
	if err != nil {
		t.Fatalf("Cut: %v", err)
	}
	want := strings.Replace(released, "## Unreleased\n", "## Unreleased\n\n## 0.2.1 — 2026-11-18\n", 1)
	if string(got) != want {
		t.Fatalf("Cut =\n%s\nwant\n%s", got, want)
	}
	next, err := release.Next(got)
	if err != nil {
		t.Fatalf("Next after Cut: %v", err)
	}
	if next != "" {
		t.Fatalf("Next after Cut = %q, want nothing to release", next)
	}
}

func TestCutRefuses(t *testing.T) {
	cases := []struct{ name, changelog, version, date string }{
		{"a version that is not next", released, "0.3.0", "2026-11-18"},
		{"a version with a v", released, "v0.2.1", "2026-11-18"},
		{"a date that is not a date", released, "0.2.1", "18/11/2026"},
		{"nothing to release", "# Changelog\n\n## Unreleased\n\n## 0.1.0 — 2026-10-01\n\n- x\n", "0.1.1", "2026-11-18"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := release.Cut([]byte(c.changelog), c.version, c.date)
			if err == nil {
				t.Fatalf("Cut returned %d bytes with no error, want a refusal", len(got))
			}
		})
	}
}

func TestPending(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"the newest version has no tag", []string{"v0.1.0"}, "0.2.0"},
		{"every version is tagged", []string{"v0.1.0", "v0.2.0"}, ""},
		{"tags that are not versions are ignored", []string{"v0.1.0", "v0.2.0", "latest", ""}, ""},
		{"no tags at all", nil, "0.2.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := release.Pending([]byte(released), c.tags)
			if err != nil {
				t.Fatalf("Pending: %v", err)
			}
			if got != c.want {
				t.Fatalf("Pending = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPendingRefusesAnOlderUntaggedVersion(t *testing.T) {
	got, err := release.Pending([]byte(released), []string{"v0.2.0"})
	if err == nil {
		t.Fatalf("Pending = %q with no error; 0.1.0 has no tag under a tagged 0.2.0, which must be reported", got)
	}
}

func TestNotes(t *testing.T) {
	got, err := release.Notes([]byte(released), "0.2.0")
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if want := "### Added\n\n- A feature.\n"; got != want {
		t.Fatalf("Notes = %q, want %q", got, want)
	}
	if got, err := release.Notes([]byte(released), "0.9.0"); err == nil {
		t.Fatalf("Notes of a missing version = %q with no error, want a refusal", got)
	}
}
