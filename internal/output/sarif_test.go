package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/output"
)

type sarifLog struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name           string `json:"name"`
				InformationURI string `json:"informationUri"`
				Rules          []struct {
					ID               string `json:"id"`
					HelpURI          string `json:"helpUri"`
					ShortDescription struct {
						Text string `json:"text"`
					} `json:"shortDescription"`
				} `json:"rules"`
			} `json:"driver"`
			Extensions []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"extensions"`
		} `json:"tool"`
		Results []struct {
			RuleID    string `json:"ruleId"`
			RuleIndex int    `json:"ruleIndex"`
			Level     string `json:"level"`
			Message   struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI       string `json:"uri"`
						URIBaseID string `json:"uriBaseId"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine   int `json:"startLine"`
						StartColumn int `json:"startColumn"`
						EndLine     int `json:"endLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
			Properties   map[string]string `json:"properties"`
			Suppressions []struct {
				Kind          string `json:"kind"`
				Justification string `json:"justification"`
			} `json:"suppressions"`
		} `json:"results"`
	} `json:"runs"`
}

// written is a SARIF document as parsed and as written.
type written struct {
	log  sarifLog
	text string
}

var sarifOptions = output.SARIFOptions{
	Extensions: []output.Extension{{Name: "actionlint", Version: "1.7.12"}, {Name: "shellcheck", Version: "0.11.0"}},
	Describe: func(id string) (string, bool) {
		return "summary of " + id, true
	},
	HelpURI: func(id string) string { return "https://example.test/docs/" + id + ".md" },
}

func sarif(t *testing.T, findings []finding.Finding) written {
	t.Helper()

	var buffer bytes.Buffer
	if err := output.WriteSARIF(&buffer, findings, sarifOptions); err != nil {
		t.Fatal(err)
	}

	var parsed sarifLog
	if err := json.Unmarshal(buffer.Bytes(), &parsed); err != nil {
		t.Fatalf("%v\n%s", err, buffer.String())
	}

	return written{log: parsed, text: buffer.String()}
}

func TestSARIFIsOneRunOfOneDriverWithTheBundledToolsAsExtensions(t *testing.T) {
	got := sarif(t, []finding.Finding{at(ciPath, 3, 5, templateDrift, "drifted")})

	if got.log.Version != "2.1.0" || !strings.Contains(got.log.Schema, "sarif") || len(got.log.Runs) != 1 {
		t.Fatalf("version %q, schema %q, runs %d; want SARIF 2.1.0 with one run", got.log.Version, got.log.Schema, len(got.log.Runs))
	}
	tool := got.log.Runs[0].Tool
	if tool.Driver.Name != "actions-qa-ci" || !strings.HasPrefix(tool.Driver.InformationURI, "https://") {
		t.Errorf("driver %+v", tool.Driver)
	}
	if len(tool.Extensions) != 2 || tool.Extensions[0].Name != "actionlint" || tool.Extensions[0].Version != "1.7.12" || tool.Extensions[1].Name != "shellcheck" {
		t.Errorf("extensions %+v", tool.Extensions)
	}
}

func TestAResultCarriesTheIdentifierLevelPositionAndTheUpstreamIdentifier(t *testing.T) {
	f := at("dir/ci.yml", 3, 0, "actionsqaci.actionlint.credentials", "bad")
	f.Severity = finding.SeverityWarning
	f.Tool = "actionlint 1.7.12"
	f.UpstreamID = "syntax-check"

	got := sarif(t, []finding.Finding{f})

	if len(got.log.Runs[0].Results) != 1 {
		t.Fatalf("results %+v", got.log.Runs[0].Results)
	}
	r := got.log.Runs[0].Results[0]
	region := r.Locations[0].PhysicalLocation.Region
	artifact := r.Locations[0].PhysicalLocation.ArtifactLocation
	if r.RuleID != "actionsqaci.actionlint.credentials" || r.Level != "warning" || r.Message.Text != "bad" {
		t.Errorf("result %+v", r)
	}
	if artifact.URI != "dir/ci.yml" || artifact.URIBaseID != "%SRCROOT%" {
		t.Errorf("artifact %+v; want the repository-relative path under %%SRCROOT%%", artifact)
	}
	if region.StartLine != 3 || region.StartColumn != 1 || region.EndLine != 3 {
		t.Errorf("region %+v; a column that is not known is the start of the line", region)
	}
	if r.Properties["upstreamId"] != "syntax-check" || r.Properties["tool"] != "actionlint 1.7.12" {
		t.Errorf("properties %+v", r.Properties)
	}
}

func TestEachSeverityIsTheSARIFLevelCodeScanningKnows(t *testing.T) {
	for severity, want := range map[finding.Severity]string{finding.SeverityError: "error", finding.SeverityWarning: "warning", finding.SeverityNote: "note"} {
		f := at(ciPath, 1, 1, templateDrift, "m")
		f.Severity = severity

		got := sarif(t, []finding.Finding{f})

		if level := got.log.Runs[0].Results[0].Level; level != want {
			t.Errorf("%s is level %q, want %q", severity, level, want)
		}
	}
}

func TestTheRulesAreThoseTheResultsUseEachWithItsPageAndTheResultPointsAtItsRule(t *testing.T) {
	findings := []finding.Finding{
		at("b.yml", 1, 1, templateDrift, "one"),
		at("a.yml", 1, 1, lintCoverage, "two"),
		at("c.yml", 1, 1, templateDrift, "three"),
	}

	got := sarif(t, findings)

	rules := got.log.Runs[0].Tool.Driver.Rules
	if len(rules) != 2 || rules[0].ID != lintCoverage || rules[1].ID != templateDrift {
		t.Fatalf("rules %+v; want each used identifier once, in order", rules)
	}
	if rules[1].HelpURI != "https://example.test/docs/"+templateDrift+".md" || rules[1].ShortDescription.Text != "summary of "+templateDrift {
		t.Errorf("rule %+v", rules[1])
	}
	for _, r := range got.log.Runs[0].Results {
		if rules[r.RuleIndex].ID != r.RuleID {
			t.Errorf("result %s points at rule %s", r.RuleID, rules[r.RuleIndex].ID)
		}
	}
}

func TestAnExceptedFindingIsKeptAsAnExternalSuppressionNotDropped(t *testing.T) {
	accepted := at(ciPath, 1, 1, templateDrift, "accepted")
	accepted.Excepted, accepted.ExceptionKey = true, "actionsqaci.templateDrift@.github/workflows/ci.yml"
	active := at(ciPath, 2, 1, templateDrift, "active")

	got := sarif(t, []finding.Finding{accepted, active})

	results := got.log.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("results %+v; an excepted finding must stay so code scanning shows it as suppressed", results)
	}
	if len(results[0].Suppressions) != 1 || results[0].Suppressions[0].Kind != "external" || !strings.Contains(results[0].Suppressions[0].Justification, accepted.ExceptionKey) {
		t.Errorf("suppressions %+v", results[0].Suppressions)
	}
	if len(results[1].Suppressions) != 0 || strings.Count(got.text, `"suppressions"`) != 1 {
		t.Errorf("the active finding must carry no suppressions key:\n%s", got.text)
	}
}

func TestACleanRunIsAnEmptyListNotNull(t *testing.T) {
	got := sarif(t, nil)

	if !strings.Contains(got.text, `"results": []`) {
		t.Errorf("results must be an empty list, which code scanning reads as no alerts:\n%s", got.text)
	}
}

func TestAPathIsAURIReference(t *testing.T) {
	cases := map[string]string{
		"a b#c?.yml": "a%20b%23c%3F.yml",
		"x:y/ci.yml": "./x:y/ci.yml",
		"100%.yml":   "100%25.yml",
	}

	for path, want := range cases {
		got := sarif(t, []finding.Finding{at(path, 1, 1, templateDrift, "m")})

		if uri := got.log.Runs[0].Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI; uri != want {
			t.Errorf("path %q is URI %q, want %q", path, uri, want)
		}
	}
}

func TestSARIFEscapesWhatHidesOrReordersTextAndKeepsTheValue(t *testing.T) {
	f := at(ciPath, 3, 5, templateDrift, hostile())

	got := sarif(t, []finding.Finding{f})

	for _, r := range got.text {
		if r != '\n' && r != ' ' && finding.IsControlOrSeparator(r) {
			t.Errorf("printed U+%04X", r)
		}
	}
	if got.log.Runs[0].Results[0].Message.Text != hostile() {
		t.Errorf("the message did not survive the round trip")
	}
}

func TestSARIFRefusesAnInvalidFindingAndWritesNothing(t *testing.T) {
	bad := at(ciPath, 0, 1, templateDrift, "m")

	var buffer bytes.Buffer
	err := output.WriteSARIF(&buffer, []finding.Finding{bad}, sarifOptions)

	if err == nil || buffer.Len() != 0 {
		t.Errorf("error %v, wrote %q", err, buffer.String())
	}
}

func TestSARIFIsTheSameWhateverOrderTheFindingsCameIn(t *testing.T) {
	a := at("a.yml", 1, 1, templateDrift, "a")
	b := at("b.yml", 2, 1, lintCoverage, "b")

	first := sarif(t, []finding.Finding{a, b})
	second := sarif(t, []finding.Finding{b, a})

	if first.text != second.text {
		t.Errorf("the output depends on order:\n%s\n%s", first.text, second.text)
	}
}
