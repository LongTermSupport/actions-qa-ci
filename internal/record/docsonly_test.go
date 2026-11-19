package record_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const inputReason = "The README is read by the usage test, so a change to it can break the build"

const exemptReason = "The public-safety scan has to see every file a docs-only push publishes"

func TestARecordDeclaresDocsOnlyInputsAndExemptWorkflowsWithReasons(t *testing.T) {
	data := "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n      reason: " + inputReason +
		"\n  exempt:\n    - workflow: .github/workflows/scan.yml\n      reason: " + exemptReason + "\n"
	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	inputs := rec.DocsOnly.Inputs
	if len(inputs) != 1 || inputs[0].Path != "README.md" || inputs[0].Reason != inputReason || inputs[0].Line != 4 {
		t.Errorf("inputs = %+v, want README.md with its reason at line 4", inputs)
	}
	exempt := rec.DocsOnly.Exempt
	if len(exempt) != 1 || exempt[0].Workflow != ".github/workflows/scan.yml" || exempt[0].Reason != exemptReason || exempt[0].Line != 7 {
		t.Errorf("exempt = %+v, want scan.yml with its reason at line 7", exempt)
	}
	if !rec.DocsOnly.Exempts(".github/workflows/scan.yml") || rec.DocsOnly.Exempts(".github/workflows/ci.yml") {
		t.Error("Exempts must hold for the named workflow and no other")
	}
}

func TestAnInputIsNoLongerADocsOnlyPathAndAGlobInputTakesOutWhatItMatches(t *testing.T) {
	data := "version: 1\ndocsOnly:\n  paths: [guide/usage.md]\n  inputs:\n    - path: README.md\n      reason: " + inputReason +
		"\n    - path: 'docs/images/*'\n      reason: The images under docs are embedded into the binary by the build\n"
	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.DocsOnly.EffectivePaths()
	want := []string{"notes/design.md", "docs/guide/install.md", "guide/usage.md"}
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q: README.md and the image are inputs", got, want)
	}
}

func TestADocsOnlyInputOrExemptEntryThatIsMalformedIsRefused(t *testing.T) {
	reason := "\n      reason: " + inputReason + "\n"
	cases := map[string]struct{ data, mention string }{
		"no inputs":                    {"version: 1\ndocsOnly:\n  inputs: []\n", "inputs is empty"},
		"no exempt":                    {"version: 1\ndocsOnly:\n  exempt: []\n", "exempt is empty"},
		"a bare input":                 {"version: 1\ndocsOnly:\n  inputs:\n    -\n", "input 1 (line 4) is empty"},
		"a bare exempt":                {"version: 1\ndocsOnly:\n  exempt:\n    -\n", "exempt entry 1 (line 4) is empty"},
		"an input with no path":        {"version: 1\ndocsOnly:\n  inputs:\n    - reason: " + inputReason + "\n", "path is missing"},
		"an absolute input":            {"version: 1\ndocsOnly:\n  inputs:\n    - path: /README.md" + reason, "absolute"},
		"a negated input":              {"version: 1\ndocsOnly:\n  inputs:\n    - path: '!README.md'" + reason, "negation"},
		"an input twice":               {"version: 1\ndocsOnly:\n  inputs:\n    - path: README.md" + reason + "    - path: README.md" + reason, "listed twice"},
		"an input that is a docs path": {"version: 1\ndocsOnly:\n  paths: [guide/usage.md]\n  inputs:\n    - path: 'guide/*'" + reason, "guide/usage.md"},
		"inputs that take every path":  {"version: 1\ndocsOnly:\n  inputs:\n    - path: '**'" + reason, "exempt"},
		"an unknown input key":         {"version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n      because: " + inputReason + "\n", "because"},
		"an exempt with no workflow":   {"version: 1\ndocsOnly:\n  exempt:\n    - reason: " + exemptReason + "\n", "workflow is missing"},
		"an exempt outside workflows":  {"version: 1\ndocsOnly:\n  exempt:\n    - workflow: ci.yml" + reason, "not a workflow"},
		"an exempt in a subdirectory":  {"version: 1\ndocsOnly:\n  exempt:\n    - workflow: .github/workflows/sub/ci.yml" + reason, "not a workflow"},
		"an exempt that is no YAML":    {"version: 1\ndocsOnly:\n  exempt:\n    - workflow: .github/workflows/ci.txt" + reason, "not a workflow"},
		"an exempt glob":               {"version: 1\ndocsOnly:\n  exempt:\n    - workflow: '.github/workflows/*.yml'" + reason, "glob"},
		"an exempt twice": {"version: 1\ndocsOnly:\n  exempt:\n    - workflow: .github/workflows/ci.yml" + reason +
			"    - workflow: .github/workflows/ci.yml" + reason, "listed twice"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), everythingIsKnown)
		var invalid *record.InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error %v, want an invalid record", name, err)

			continue
		}
		if !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error %q must mention %q", name, err.Error(), c.mention)
		}
	}
}

func TestADocsOnlyEntryWithoutARealReasonHasAFault(t *testing.T) {
	cases := map[string]struct {
		reason string
		faults int
	}{
		"a real reason":    {inputReason, 0},
		"no reason":        {"", 1},
		"blank":            {"   ", 1},
		"a generic one":    {"legacy", 1},
		"too short":        {"read by a test", 1},
		"points elsewhere": {"The same reason as the entry written above this one, see above", 1},
	}

	for name, c := range cases {
		input := record.DocsOnlyInput{Path: "README.md", Reason: c.reason}
		if got := input.Faults(); len(got) != c.faults || (c.faults == 1 && got[0].Field != "reason") {
			t.Errorf("%s: input faults = %+v, want %d on reason", name, got, c.faults)
		}
		exempt := record.DocsOnlyExempt{Workflow: ".github/workflows/scan.yml", Reason: c.reason}
		if got := exempt.Faults(); len(got) != c.faults || (c.faults == 1 && got[0].Field != "reason") {
			t.Errorf("%s: exempt faults = %+v, want %d on reason", name, got, c.faults)
		}
	}
}

func TestAnEntryWithNoReasonIsReadSoItsFaultCanBeReported(t *testing.T) {
	data := "version: 1\ndocsOnly:\n  inputs:\n    - path: README.md\n  exempt:\n    - workflow: .github/workflows/scan.yml\n"
	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatalf("a missing reason is a finding, not a refusal: %v", err)
	}
	if len(rec.DocsOnly.Inputs[0].Faults()) != 1 || len(rec.DocsOnly.Exempt[0].Faults()) != 1 {
		t.Errorf("faults %+v and %+v, want one each", rec.DocsOnly.Inputs[0].Faults(), rec.DocsOnly.Exempt[0].Faults())
	}
}
