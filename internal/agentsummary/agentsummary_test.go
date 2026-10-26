package agentsummary_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/agentsummary"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
)

func entries() []listing.Entry {
	return []listing.Entry{
		{ID: "actionsqaci.lintCoverage", Summary: "Every tracked workflow-shaped file is analysed"},
		{ID: "actionsqaci.shellcheck.SC<nnnn>", Summary: "ShellCheck's findings on shell in run steps", NarrowedBy: []string{"actionsqaci.shellcheck.SC2086 is disabled everywhere by .github/actions-qa-ci.yml:9"}},
	}
}

func TestRegionListsEveryDefenceAndWhatNarrowsIt(t *testing.T) {
	region, err := agentsummary.Region(entries())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(region, agentsummary.Begin+"\n") || !strings.HasSuffix(region, "\n"+agentsummary.End) {
		t.Errorf("region must be delimited by its markers:\n%s", region)
	}
	for _, want := range []string{
		"- `actionsqaci.lintCoverage` — Every tracked workflow-shaped file is analysed\n",
		"- `actionsqaci.shellcheck.SC<nnnn>` — ShellCheck's findings on shell in run steps\n",
		"  - narrowed: actionsqaci.shellcheck.SC2086 is disabled everywhere by .github/actions-qa-ci.yml:9\n",
		"actions-qa rule-doc <identifier>",
	} {
		if !strings.Contains(region, want) {
			t.Errorf("region should contain %q:\n%s", want, region)
		}
	}
}

func TestRegionSaysWhichDefencesTheRecordAsksFor(t *testing.T) {
	region, err := agentsummary.Region([]listing.Entry{
		{ID: "actionsqaci.zizmor.concurrency-limits", Summary: "A workflow states a concurrency limit", EnabledBy: []string{"actionsqaci.zizmor.concurrency-limits is enabled by .github/actions-qa-ci.yml:4"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if want := "  - enabled: actionsqaci.zizmor.concurrency-limits is enabled by .github/actions-qa-ci.yml:4\n"; !strings.Contains(region, want) {
		t.Errorf("region should contain %q:\n%s", want, region)
	}
}

func TestRegionOfNoDefencesSaysSo(t *testing.T) {
	region, err := agentsummary.Region(nil)
	if err != nil || !strings.Contains(region, "No defence is active") {
		t.Errorf("region = %q, err = %v", region, err)
	}
}

func TestRegionRefusesTextThatCouldEndTheRegionOrStartAnotherLine(t *testing.T) {
	for name, entry := range map[string]listing.Entry{
		"a newline in the summary":    {ID: "a.b", Summary: "one\ntwo"},
		"a marker in the summary":     {ID: "a.b", Summary: agentsummary.End},
		"a newline in a narrowing":    {ID: "a.b", Summary: "s", NarrowedBy: []string{"x\ny"}},
		"a newline in an enabling":    {ID: "a.b", Summary: "s", EnabledBy: []string{"x\ny"}},
		"a carriage return":           {ID: "a.b", Summary: "x\ry"},
		"a backtick in an identifier": {ID: "a`b", Summary: "s"},
	} {
		if _, err := agentsummary.Region([]listing.Entry{entry}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func region(t *testing.T) string {
	t.Helper()
	got, err := agentsummary.Region(entries())
	if err != nil {
		t.Fatal(err)
	}

	return got
}

func TestSpliceCreatesTheFileContentFromNothing(t *testing.T) {
	got, err := agentsummary.Splice(nil, region(t))

	if err != nil || string(got) != region(t)+"\n" {
		t.Errorf("got %q, err %v", got, err)
	}
}

func TestSpliceAppendsAfterWhatIsThereAndKeepsItByteForByte(t *testing.T) {
	for name, existing := range map[string]string{
		"with a final newline":    "# Project\n\nText.\n",
		"without a final newline": "# Project\n\nText.",
	} {
		got, err := agentsummary.Splice([]byte(existing), region(t))

		want := strings.TrimRight(existing, "\n") + "\n\n" + region(t) + "\n"
		if err != nil || string(got) != want {
			t.Errorf("%s: got %q, err %v; want %q", name, got, err, want)
		}
		if !strings.HasPrefix(string(got), strings.TrimRight(existing, "\n")) {
			t.Errorf("%s: the existing text must be kept", name)
		}
	}
}

func TestSpliceReplacesOnlyTheRegionAndIsIdempotent(t *testing.T) {
	before := "# Project\n\nIntro.\n\n"
	after := "\n## Other\n\nMore.\n"
	existing := before + agentsummary.Begin + "\n\nold text\n\n" + agentsummary.End + "\n" + after

	got, err := agentsummary.Splice([]byte(existing), region(t))
	if err != nil {
		t.Fatal(err)
	}

	if want := before + region(t) + "\n" + after; string(got) != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	again, err := agentsummary.Splice(got, region(t))
	if err != nil || !bytes.Equal(again, got) {
		t.Errorf("a second splice changed the file: %q, err %v", again, err)
	}
}

func TestSpliceRefusesMarkersThatDoNotFormOneRegion(t *testing.T) {
	cases := map[string]string{
		"a begin alone":            "text\n" + agentsummary.Begin + "\nmore\n",
		"an end alone":             "text\n" + agentsummary.End + "\n",
		"two begins":               agentsummary.Begin + "\n" + agentsummary.Begin + "\n" + agentsummary.End + "\n",
		"two regions":              agentsummary.Begin + "\n" + agentsummary.End + "\n" + agentsummary.Begin + "\n" + agentsummary.End + "\n",
		"the end before the begin": agentsummary.End + "\n" + agentsummary.Begin + "\n",
	}

	for name, existing := range cases {
		if _, err := agentsummary.Splice([]byte(existing), region(t)); err == nil {
			t.Errorf("%s: want an error, a file with a broken region must not be rewritten", name)
		}
	}
}

func TestAMarkerInsideALineIsNotAMarker(t *testing.T) {
	existing := "Write " + agentsummary.Begin + " to start it.\n"

	got, err := agentsummary.Splice([]byte(existing), region(t))

	if err != nil || !strings.HasPrefix(string(got), existing) {
		t.Errorf("got %q, err %v; text that only mentions a marker is not a region", got, err)
	}
}

func crlf(text string) string { return strings.ReplaceAll(text, "\n", "\r\n") }

func TestAFileWithWindowsLineEndingsGetsItsRegionReplacedNotRepeated(t *testing.T) {
	before := crlf("# Project\n\nIntro.\n\n")
	after := crlf("\n## Other\n")
	existing := before + crlf(agentsummary.Begin+"\n\nold text\n\n"+agentsummary.End+"\n") + after

	got, err := agentsummary.Splice([]byte(existing), region(t))
	if err != nil {
		t.Fatal(err)
	}

	if want := before + crlf(region(t)+"\n") + after; string(got) != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	again, err := agentsummary.Splice(got, region(t))
	if err != nil || !bytes.Equal(again, got) {
		t.Errorf("a second splice changed the file: %q, err %v", again, err)
	}
}

func TestARegionAppendedToAFileWithWindowsLineEndingsHasThemToo(t *testing.T) {
	got, err := agentsummary.Splice([]byte(crlf("# Project\n\nText.\n")), region(t))

	if want := crlf("# Project\n\nText.\n\n" + region(t) + "\n"); err != nil || string(got) != want {
		t.Errorf("got %q, err %v; want %q", got, err, want)
	}
}

func TestExtractGivesTheRegionWithPlainLineEndingsWhateverTheFileUses(t *testing.T) {
	file := crlf("# Project\n\n" + region(t) + "\n")

	found, ok, err := agentsummary.Extract([]byte(file))

	if err != nil || !ok || found != region(t) {
		t.Errorf("found %q, %v, %v; want the region as Region renders it", found, ok, err)
	}
}

func TestBeginLineSaysWhereTheRegionStartsCountingFromOne(t *testing.T) {
	file := "# Project\n\ntext\r\n" + region(t) + "\n"

	line, found, err := agentsummary.BeginLine([]byte(file))
	if err != nil || !found || line != 4 {
		t.Errorf("line %d, found %v, err %v", line, found, err)
	}
	if _, found, err := agentsummary.BeginLine([]byte("# Project\n")); found || err != nil {
		t.Errorf("a file with no region: found %v, err %v", found, err)
	}
	if _, _, err := agentsummary.BeginLine([]byte(agentsummary.End + "\n")); err == nil {
		t.Error("markers that do not form a region must be an error")
	}
}

func TestExtractFindsTheRegionAndCurrentComparesIt(t *testing.T) {
	file, err := agentsummary.Splice([]byte("# Project\n"), region(t))
	if err != nil {
		t.Fatal(err)
	}

	found, ok, err := agentsummary.Extract(file)
	if err != nil || !ok || found != region(t) {
		t.Fatalf("found %q, %v, %v", found, ok, err)
	}

	if _, ok, err := agentsummary.Extract([]byte("# Project\n")); ok || err != nil {
		t.Errorf("a file with no region: ok %v, err %v", ok, err)
	}
	if _, _, err := agentsummary.Extract([]byte(agentsummary.Begin + "\n")); err == nil {
		t.Error("a region that is never closed must be an error")
	}
}
