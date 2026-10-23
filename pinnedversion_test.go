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
