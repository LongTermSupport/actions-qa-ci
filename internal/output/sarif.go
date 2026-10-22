package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const (
	sarifSchema   = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion  = "2.1.0"
	driverName    = "actions-qa-ci"
	driverURI     = "https://github.com/LongTermSupport/actions-qa-ci"
	sourceRootID  = "%SRCROOT%"
	suppressionBy = "external"
)

// Extension is a bundled tool whose rules the findings come from, named in the log so a reader
// knows what ran and at which version.
type Extension struct {
	Name    string
	Version string
}

// SARIFOptions is what the SARIF log says that the findings do not: the bundled tools, what a
// rule requires, and where a reader finds its page.
type SARIFOptions struct {
	Extensions []Extension
	// Describe gives the one-line statement of a rule; ok is false for an identifier it does
	// not know, which then has no description.
	Describe func(id string) (summary string, ok bool)
	// HelpURI is where the page of a rule can be read, or empty for none.
	HelpURI func(id string) string
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID               string        `json:"id"`
	ShortDescription *sarifMessage `json:"shortDescription,omitempty"`
	HelpURI          string        `json:"helpUri,omitempty"`
}

type sarifResult struct {
	RuleID       string             `json:"ruleId"`
	RuleIndex    int                `json:"ruleIndex"`
	Level        string             `json:"level"`
	Message      sarifMessage       `json:"message"`
	Locations    []sarifLocation    `json:"locations"`
	Properties   map[string]string  `json:"properties"`
	Suppressions []sarifSuppression `json:"suppressions,omitempty"`
}

type sarifLocation struct {
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
}

type sarifSuppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification"`
}

type sarifExtension struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRun struct {
	Tool struct {
		Driver     sarifDriver      `json:"driver"`
		Extensions []sarifExtension `json:"extensions"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifDocument struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

// WriteSARIF writes the findings as one SARIF 2.1.0 log with a single run: GitHub rejects an
// upload with several runs of one tool and category. Every finding is kept; an excepted one
// carries an external suppression, so code scanning shows it as suppressed and does not drop
// it. It validates every finding and renders the whole log before writing any of it.
func WriteSARIF(w io.Writer, findings []finding.Finding, options SARIFOptions) error {
	for _, f := range findings {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("refusing to write an invalid finding: %w", err)
		}
	}

	sorted := slices.Clone(findings)
	finding.Sort(sorted)

	var ids []string
	for _, f := range sorted {
		ids = append(ids, f.ID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	run := sarifRun{Results: []sarifResult{}}
	run.Tool.Driver = sarifDriver{Name: driverName, InformationURI: driverURI, Rules: []sarifRule{}}
	run.Tool.Extensions = []sarifExtension{}
	for _, extension := range options.Extensions {
		run.Tool.Extensions = append(run.Tool.Extensions, sarifExtension(extension))
	}
	for _, id := range ids {
		rule := sarifRule{ID: id}
		if options.Describe != nil {
			if summary, ok := options.Describe(id); ok {
				rule.ShortDescription = &sarifMessage{Text: summary}
			}
		}
		if options.HelpURI != nil {
			rule.HelpURI = options.HelpURI(id)
		}
		run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, rule)
	}

	for _, f := range sorted {
		run.Results = append(run.Results, result(f, slices.Index(ids, f.ID)))
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sarifDocument{Schema: sarifSchema, Version: sarifVersion, Runs: []sarifRun{run}}); err != nil {
		return err
	}

	if _, err := w.Write([]byte(escapeUnprintable(encoded.String()))); err != nil {
		return fmt.Errorf("writing SARIF output: %w", err)
	}

	return nil
}

func result(f finding.Finding, ruleIndex int) sarifResult {
	var location sarifLocation
	location.PhysicalLocation.ArtifactLocation.URI = (&url.URL{Path: f.Path}).String()
	location.PhysicalLocation.ArtifactLocation.URIBaseID = sourceRootID
	location.PhysicalLocation.Region.StartLine = f.Line
	location.PhysicalLocation.Region.StartColumn = column(f)
	location.PhysicalLocation.Region.EndLine = max(f.EndLine, f.Line)

	r := sarifResult{
		RuleID:     f.ID,
		RuleIndex:  ruleIndex,
		Level:      sarifLevel(f.Severity),
		Message:    sarifMessage{Text: f.Message},
		Locations:  []sarifLocation{location},
		Properties: map[string]string{"tool": f.Tool, "upstreamId": f.UpstreamID},
	}
	if f.Excepted {
		r.Suppressions = []sarifSuppression{{Kind: suppressionBy, Justification: "accepted by the exception " + f.ExceptionKey + " of the project record"}}
	}

	return r
}

func sarifLevel(severity finding.Severity) string {
	switch severity {
	case finding.SeverityWarning:
		return "warning"
	case finding.SeverityNote:
		return "note"
	default:
		return "error"
	}
}
