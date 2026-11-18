package firstparty_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex/gitindextest"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const (
	inputReason  = "The README is read by the usage test, so a change to it can break the build"
	exemptReason = "The public-safety scan has to see every file a docs-only push publishes"

	// readmeIsAnInput leaves documentation out and takes the README back in, because a test
	// reads it.
	readmeIsAnInput = "on:\n  push:\n    branches: [main]\n    paths: ['%%', '!%%.md', '!docs/%%', 'README.md']\n" + oneJob

	// scanEveryPush has no paths filter: it must see every push.
	scanEveryPush = "on:\n  push:\n    branches: [main]\n" + oneJob

	inputRecord  = "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n      reason: " + inputReason + "\n"
	exemptRecord = "version: 1\ndocsOnly:\n  exempt:\n    - workflow: .github/workflows/ci.yml\n      reason: " + exemptReason + "\n"
)

// docsOnlyRecordCheck writes the files, makes them all tracked, and runs the docsOnlyPush check
// on the project record.
func docsOnlyRecordCheck(t *testing.T, files map[string]string) []finding.Finding {
	t.Helper()

	expanded := map[string]string{}
	var tracked []string
	for name, text := range files {
		expanded[name] = globs(text)
		tracked = append(tracked, name)
	}
	inTree(t, expanded)
	gitindextest.Write(t, ".", gitindextest.Files(tracked...)...)

	got, err := firstparty.CheckDocsOnlyPush(everything)(record.Path)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	for _, f := range got {
		if f.ID != firstparty.DocsOnlyPush || f.Path != record.Path {
			t.Errorf("finding %+v is not a %s finding at the record", f, firstparty.DocsOnlyPush)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("finding %+v is not valid: %v", f, err)
		}
	}

	return got
}

func TestDocsOnlyInputsDiscriminate(t *testing.T) {
	red := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: readmeIsAnInput})
	if len(red) != 1 || !strings.Contains(red[0].Message, "README.md") || !strings.Contains(red[0].Message, "docsOnly.inputs") {
		t.Errorf("a filter taking README.md back in, with no input declared: %v, want one finding naming README.md and docsOnly.inputs", red)
	}

	green := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: readmeIsAnInput, record.Path: inputRecord})
	if len(green) != 0 {
		t.Errorf("with README.md declared an input: %v, want none", green)
	}

	stillDocs := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: redDocsOnly, record.Path: inputRecord})
	if len(stillDocs) != 1 || strings.Contains(stillDocs[0].Message, "README.md") || !strings.Contains(stillDocs[0].Message, "docs/guide/install.md") {
		t.Errorf("an input takes out only what it names: %v, want one finding naming the other docs", stillDocs)
	}
}

func TestDocsOnlyExemptDiscriminates(t *testing.T) {
	red := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: scanEveryPush})
	if len(red) != 1 || !strings.Contains(red[0].Message, "docsOnly.exempt") {
		t.Errorf("a workflow with no filter, not exempt: %v, want one finding naming docsOnly.exempt", red)
	}

	green := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: scanEveryPush, record.Path: exemptRecord})
	if len(green) != 0 {
		t.Errorf("with the workflow exempt: %v, want none", green)
	}

	other := strings.ReplaceAll(exemptRecord, "ci.yml", "scan.yml")
	if got := runRule(t, firstparty.CheckDocsOnlyPush(everything), map[string]string{workflow: scanEveryPush, record.Path: other}); len(got) != 1 {
		t.Errorf("with another workflow exempt: %v, want the finding", got)
	}
}

func TestDocsOnlyEntriesThatNameSomethingRealAndNeededAreNoFinding(t *testing.T) {
	both := "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n      reason: " + inputReason +
		"\n  exempt:\n    - workflow: .github/workflows/scan.yml\n      reason: " + exemptReason + "\n"
	got := docsOnlyRecordCheck(t, map[string]string{
		record.Path:                  both,
		"README.md":                  "# x\n",
		workflow:                     readmeIsAnInput,
		".github/workflows/scan.yml": scanEveryPush,
	})
	if len(got) != 0 {
		t.Errorf("findings %v, want none", got)
	}
}

func TestADocsOnlyInputThatMatchesNoTrackedFileIsAFinding(t *testing.T) {
	stale := "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n      reason: " + inputReason +
		"\n    - path: 'guide/*.md'\n      reason: The guide pages are rendered into the binary by the build step\n"
	got := docsOnlyRecordCheck(t, map[string]string{record.Path: stale, "README.md": "# x\n"})
	if len(got) != 1 || got[0].Line != 6 || !strings.Contains(got[0].Message, "guide/*.md") || !strings.Contains(got[0].Message, "matches no tracked file") {
		t.Errorf("findings %v, want one at line 6 naming the input that matches nothing", got)
	}
}

func TestADocsOnlyExemptWorkflowThatDoesNotExistIsAFinding(t *testing.T) {
	got := docsOnlyRecordCheck(t, map[string]string{record.Path: exemptRecord})
	if len(got) != 1 || got[0].Line != 4 || !strings.Contains(got[0].Message, ".github/workflows/ci.yml") || !strings.Contains(got[0].Message, "no tracked workflow") {
		t.Errorf("findings %v, want one at line 4 naming the missing workflow", got)
	}
}

func TestADocsOnlyExemptWorkflowTheRuleWouldNotReportIsAFinding(t *testing.T) {
	filtered := "on:\n  push:\n    paths-ignore: ['%%.md', 'docs/%%']\n" + oneJob
	got := docsOnlyRecordCheck(t, map[string]string{record.Path: exemptRecord, workflow: filtered})
	if len(got) != 1 || got[0].Line != 4 || !strings.Contains(got[0].Message, "excuses nothing") {
		t.Errorf("findings %v, want one at line 4 saying the exemption excuses nothing", got)
	}
}

func TestADocsOnlyEntryWithoutAReasonIsAnUnjustifiedExceptionFinding(t *testing.T) {
	noReasons := "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n  exempt:\n    - workflow: .github/workflows/ci.yml\n      reason: legacy\n"
	path := inRepository(t, noReasons)

	got, err := firstparty.CheckRecord(everythingIsKnown)(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("findings %v, want two", got)
	}
	for i, want := range []struct {
		line    int
		mention string
	}{{4, "reason of docsOnly input 1 (README.md) is missing"}, {6, "reason of docsOnly exempt entry 1 (.github/workflows/ci.yml)"}} {
		f := got[i]
		if f.ID != firstparty.UnjustifiedException || f.Line != want.line || !strings.Contains(f.Message, want.mention) {
			t.Errorf("finding %d = %+v, want %s at line %d mentioning %q", i, f, firstparty.UnjustifiedException, want.line, want.mention)
		}
	}

	justified := inRepository(t, inputRecord)
	if got, err := firstparty.CheckRecord(everythingIsKnown)(justified); err != nil || len(got) != 0 {
		t.Errorf("with a real reason: findings %v, error %v; want none", got, err)
	}
}
