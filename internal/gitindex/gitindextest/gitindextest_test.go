package gitindextest_test

import (
	"slices"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
)

func TestWhatIsWrittenIsWhatGitindexReadsBack(t *testing.T) {
	root := t.TempDir()
	entries := append(gitindextest.Files("b/z.yml", ".github/workflows/ci.yml", "a b/ü.yml"), gitindextest.Link("link.yml"))

	gitindextest.Write(t, root, entries...)

	got, err := gitindex.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range got {
		paths = append(paths, e.Path)
	}
	if !slices.Equal(paths, []string{".github/workflows/ci.yml", "a b/ü.yml", "b/z.yml", "link.yml"}) {
		t.Errorf("paths = %q", paths)
	}
	if !got[0].IsRegular() || got[3].IsRegular() {
		t.Errorf("a regular file and a link were not told apart: %+v", got)
	}
}
