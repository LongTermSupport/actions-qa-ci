package changelog_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/changelog"
)

const withAddedOnly = `# Changelog

## Unreleased

### Added

- The first thing.

## 1.0.0

### Changed

- A change that was released.
`

func record(t *testing.T, content string, moves ...changelog.Move) string {
	t.Helper()

	got, err := changelog.Record([]byte(content), moves)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	return string(got)
}

func TestAMoveAddsAChangedSectionAfterAddedWhenThereIsNone(t *testing.T) {
	got := record(t, withAddedOnly, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	want := `# Changelog

## Unreleased

### Added

- The first thing.

### Changed

- ` + "`shellcheck`" + ` moves from v0.11.0 to v0.12.0.

## 1.0.0

### Changed

- A change that was released.
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAMoveJoinsAnExistingChangedSectionInOrder(t *testing.T) {
	content := strings.Replace(withAddedOnly, "- The first thing.\n", "- The first thing.\n\n### Changed\n\n- `zizmor` moves from v1.0.0 to v1.1.0.\n", 1)

	got := record(t, content, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	shell := strings.Index(got, "`shellcheck` moves")
	zizmor := strings.Index(got, "`zizmor` moves")
	released := strings.Index(got, "## 1.0.0")
	if shell < 0 || zizmor < 0 || shell > zizmor || zizmor > released {
		t.Errorf("the entries are not in tool order inside Unreleased:\n%s", got)
	}
}

func TestChangedIsPlacedBeforeALaterHeadingNotAfterIt(t *testing.T) {
	content := strings.Replace(withAddedOnly, "- The first thing.\n", "- The first thing.\n\n### Fixed\n\n- A fix.\n", 1)

	got := record(t, content, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	added := strings.Index(got, "### Added")
	changed := strings.Index(got, "### Changed")
	fixed := strings.Index(got, "### Fixed")
	if !(added < changed && changed < fixed) {
		t.Errorf("want Added, Changed, Fixed in that order:\n%s", got)
	}
}

func TestTwoMovesOfOneToolBecomeOneEntryFromWhereItStarted(t *testing.T) {
	once := record(t, withAddedOnly, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	twice := record(t, once, changelog.Move{Tool: "shellcheck", From: "v0.12.0", To: "v0.13.0"})

	if strings.Count(twice, "`shellcheck` moves") != 1 || !strings.Contains(twice, "`shellcheck` moves from v0.11.0 to v0.13.0.") {
		t.Errorf("want one entry from v0.11.0 to v0.13.0:\n%s", twice)
	}
}

func TestAToolThatGoesBackToWhereItStartedLeavesNoEntryAndNoEmptyHeading(t *testing.T) {
	once := record(t, withAddedOnly, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	back := record(t, once, changelog.Move{Tool: "shellcheck", From: "v0.12.0", To: "v0.11.0"})

	if back != withAddedOnly {
		t.Errorf("want the changelog as it was:\n%s", back)
	}
}

func TestRecordingTheSameMoveAgainChangesNothing(t *testing.T) {
	move := changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"}
	once := record(t, withAddedOnly, move)

	if twice := record(t, once, move); twice != once {
		t.Errorf("a second recording changed the file:\n%s", twice)
	}
}

func TestSeveralMovesAreWrittenInToolOrderWhateverOrderTheyComeIn(t *testing.T) {
	got := record(t, withAddedOnly,
		changelog.Move{Tool: "zizmor", From: "v1.0.0", To: "v1.1.0"},
		changelog.Move{Tool: "github.com/rhysd/actionlint", From: "v1.7.12", To: "v1.7.13"},
	)

	actionlint := strings.Index(got, "`github.com/rhysd/actionlint`")
	zizmor := strings.Index(got, "`zizmor`")
	if actionlint < 0 || zizmor < 0 || actionlint > zizmor {
		t.Errorf("want tool order:\n%s", got)
	}
}

func TestAReleasedSectionsChangedHeadingIsNeverTouched(t *testing.T) {
	got := record(t, withAddedOnly, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	if !strings.HasSuffix(got, "## 1.0.0\n\n### Changed\n\n- A change that was released.\n") {
		t.Errorf("the released section changed:\n%s", got)
	}
}

func TestNoMovesLeavesTheFileExactlyAsItWas(t *testing.T) {
	if got := record(t, withAddedOnly); got != withAddedOnly {
		t.Errorf("got:\n%s", got)
	}
}

func TestAChangelogWithNoUnreleasedSectionIsRefused(t *testing.T) {
	_, err := changelog.Record([]byte("# Changelog\n\n## 1.0.0\n\n### Added\n\n- x\n"), []changelog.Move{{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"}})

	if err == nil || !strings.Contains(err.Error(), "Unreleased") {
		t.Errorf("want an error naming the Unreleased section, got %v", err)
	}
}

func TestAMoveThatCouldNotBeAnEntryIsRefused(t *testing.T) {
	cases := map[string]changelog.Move{
		"a tool name with a line break":    {Tool: "shellcheck\n- injected", From: "v0.11.0", To: "v0.12.0"},
		"a tool name with a backtick":      {Tool: "shell`check", From: "v0.11.0", To: "v0.12.0"},
		"an empty tool name":               {Tool: "", From: "v0.11.0", To: "v0.12.0"},
		"a version with a line break":      {Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0\n## Evil"},
		"a version that is not a version":  {Tool: "shellcheck", From: "latest", To: "v0.12.0"},
		"an empty version":                 {Tool: "shellcheck", From: "", To: "v0.12.0"},
		"a move from a version to itself":  {Tool: "shellcheck", From: "v0.12.0", To: "v0.12.0"},
		"a version with a markdown escape": {Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0](x)"},
	}

	for name, move := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := changelog.Record([]byte(withAddedOnly), []changelog.Move{move}); err == nil {
				t.Errorf("want %q refused", name)
			}
		})
	}
}

func TestACarriageReturnFileKeepsItsLineEndings(t *testing.T) {
	content := strings.ReplaceAll(withAddedOnly, "\n", "\r\n")

	got := record(t, content, changelog.Move{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"})

	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Errorf("a bare line feed was written into a file of carriage returns:\n%q", got)
	}
	if !strings.Contains(got, "`shellcheck` moves from v0.11.0 to v0.12.0.\r\n") {
		t.Errorf("the entry is missing:\n%q", got)
	}
}
