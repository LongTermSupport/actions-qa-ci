package actionsqaci_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// A pinned tool moves by rewriting the manifest alone, so the version a tool is pinned at is
// written in the manifest and nowhere else that a program or a reader takes it from. Where else
// it is spelled, the daily update produces a change that fails its own gate until a person edits
// that place, and the update never proposes anything.
//
// The files that may spell it: the manifest itself, the changelog (a record of what a release
// held at the time) and tests (a fixture is its own manifest, and a testdata file belongs to one).
func TestAPinnedVersionIsWrittenOnlyInTheManifest(t *testing.T) {
	manifest, err := toolchain.Load()
	if err != nil {
		t.Fatal(err)
	}

	var spellings []*regexp.Regexp
	for name, tool := range manifest.Tools {
		bare := strings.TrimPrefix(tool.Version, "v")
		spellings = append(spellings, regexp.MustCompile(`(^|[^0-9.])v?`+regexp.QuoteMeta(bare)+`([^0-9]|$)`))
		t.Logf("%s is pinned at %s", name, tool.Version)
	}

	err = filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			// Only these directories of the repository root are left out: the repository's own
			// metadata, scratch and fetched binaries. A directory of the same name deeper in the
			// tree is source like any other.
			switch filepath.ToSlash(path) {
			case ".git", "untracked", "bin":
				return filepath.SkipDir
			}

			return nil
		}
		if !readsAPinnedVersion(path) {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, spelling := range spellings {
			if spelling.Match(content) {
				t.Errorf("%s spells a pinned version (%s); take it from the manifest", path, spelling.FindString(string(content)))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The changelog's Unreleased "Added" entries describe what the program does, not which release of
// a bundled tool it carries: a tool move is its own "Changed" entry, so an "Added" line that
// spelled the version would go wrong at the first move.
func TestTheChangelogsUnreleasedAddedEntriesSpellNoPinnedVersion(t *testing.T) {
	manifest, err := toolchain.Load()
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for name, tool := range manifest.Tools {
		versions[name] = tool.Version
	}
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if match := regexp.MustCompile(`(?m)^\s*github\.com/rhysd/actionlint (v\S+)`).FindSubmatch(gomod); match != nil {
		versions["actionlint"] = string(match[1])
	} else {
		t.Fatal("go.mod does not require actionlint, so its version cannot be checked")
	}

	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	added := unreleasedAdded(string(changelog))
	if added == "" {
		t.Fatal("the changelog has no Unreleased Added section to check")
	}
	for name, version := range versions {
		spelling := regexp.MustCompile(`(^|[^0-9.])v?` + regexp.QuoteMeta(strings.TrimPrefix(version, "v")) + `([^0-9]|$)`)
		if spelling.MatchString(added) {
			t.Errorf("the Unreleased Added entries spell %s's version (%s); say what it does and leave the release to a Changed entry", name, version)
		}
	}
}

// unreleasedAdded is the text of the "### Added" part of the "## Unreleased" section.
func unreleasedAdded(changelog string) string {
	var out []string
	inUnreleased, inAdded := false, false
	for _, line := range strings.Split(changelog, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			inUnreleased, inAdded = line == "## Unreleased", false
		case strings.HasPrefix(line, "### "):
			inAdded = inUnreleased && line == "### Added"
		case inAdded:
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// readsAPinnedVersion says whether a file is one a program or a reader takes a version from:
// production Go, the README and the documentation pages, the workflows and the shell templates.
func readsAPinnedVersion(path string) bool {
	path = filepath.ToSlash(path)
	switch {
	case strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/testdata/"):
		return false
	case strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sh"):
		return true
	case path == "README.md" || path == "CLAUDE.md":
		return true
	case strings.HasPrefix(path, "docs/") && strings.HasSuffix(path, ".md"):
		return true
	case strings.HasPrefix(path, ".github/") && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")):
		return true
	}

	return false
}
