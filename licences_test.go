package actionsqaci_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/release"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// The release bundles every tool the manifest pins with its licence, and the bundle refuses a
// tool whose licence is missing; licences/ holds them, so a tool added to the manifest without
// its licence fails here rather than in the release.
func TestEveryPinnedToolHasItsLicence(t *testing.T) {
	manifest, err := toolchain.Load()
	if err != nil {
		t.Fatalf("cannot load the manifest: %v", err)
	}
	if len(manifest.Tools) == 0 {
		t.Fatal("the manifest pins no tool")
	}
	for name := range manifest.Tools {
		content, err := os.ReadFile(filepath.Join("licences", name+".txt"))
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}
		if !strings.Contains(strings.ToUpper(string(content)), "LICENSE") {
			t.Errorf("licences/%s.txt does not read as a licence", name)
		}
	}
}

// The release workflow reads CHANGELOG.md only on main, after a merge; a heading it cannot read
// fails here, on the pull request, instead.
func TestTheChangelogIsReadableByTheRelease(t *testing.T) {
	content, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("cannot read CHANGELOG.md: %v", err)
	}
	next, err := release.Next(content)
	if err != nil {
		t.Fatalf("the release cannot read CHANGELOG.md: %v", err)
	}
	pending, err := release.Pending(content, nil)
	if err != nil {
		t.Fatalf("the release cannot read CHANGELOG.md: %v", err)
	}
	if next == "" && pending == "" {
		t.Log("CHANGELOG.md has nothing unreleased and no version awaiting a tag")
	}
}

// The program's own licence is LICENSE, which the release copies into the bundle; a copy kept
// under licences/ would be a second source that drifts.
func TestTheProjectLicenceIsNotDuplicated(t *testing.T) {
	_, err := os.Stat(filepath.Join("licences", "actions-qa-ci.txt"))
	if !os.IsNotExist(err) {
		t.Fatalf("licences/actions-qa-ci.txt exists (stat: %v); the release copies LICENSE instead", err)
	}
}
