package zizmor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const reportedPath = ".github/workflows/w.yml"

// location is one place of a finding as zizmor's json-v1 describes it.
func location(path, kind, annotation string, startRow, startColumn, endRow, endColumn int) map[string]any {
	return map[string]any{
		"symbolic": map[string]any{
			"key":        map[string]any{"Local": map[string]any{"verbatim_path": path}},
			"annotation": annotation,
			"kind":       kind,
		},
		"concrete": map[string]any{"location": map[string]any{
			"start_point": map[string]any{"row": startRow, "column": startColumn},
			"end_point":   map[string]any{"row": endRow, "column": endColumn},
		}},
	}
}

// entry is one finding of the json-v1 report.
func entry(ident, desc, severity string, ignored bool, locations ...map[string]any) map[string]any {
	return map[string]any{
		"ident":          ident,
		"desc":           desc,
		"determinations": map[string]any{"confidence": "High", "severity": severity, "persona": "Regular"},
		"locations":      locations,
		"ignored":        ignored,
		"fixes":          []any{},
	}
}

func encode(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()

	out, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func TestAFindingBecomesOneOfThisToolchainsWithTheIdentifierAndPositionOfItsPrimaryLocation(t *testing.T) {
	report := encode(t, entry("artipacked", "credential persistence through GitHub Actions artifacts", "Medium", false,
		location(reportedPath, "Related", "this job", 3, 2, 8, 0),
		location(reportedPath, "Primary", "does not set persist-credentials: false", 6, 8, 6, 33)))

	got, err := Convert(reportedPath, report)

	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings: %+v", len(got), got)
	}
	want := finding.Finding{
		ID:         "actionsqaci.zizmor.artipacked",
		Tool:       Tool(),
		UpstreamID: "artipacked",
		Path:       reportedPath,
		Line:       7,
		Col:        9,
		EndLine:    7,
		Severity:   finding.SeverityWarning,
		Message:    "credential persistence through GitHub Actions artifacts: does not set persist-credentials: false",
		DocPath:    "docs/rules/actionsqaci.zizmor.artipacked.md",
	}
	if got[0] != want {
		t.Errorf("got  %+v\nwant %+v", got[0], want)
	}
}

func TestTheSeverityIsZizmorsOwnReadAsHighErrorMediumWarningAndTheRestNote(t *testing.T) {
	for severity, want := range map[string]finding.Severity{
		"High":          finding.SeverityError,
		"Medium":        finding.SeverityWarning,
		"Low":           finding.SeverityNote,
		"Informational": finding.SeverityNote,
	} {
		report := encode(t, entry("artipacked", "d", severity, false, location(reportedPath, "Primary", "a", 0, 0, 0, 1)))

		got, err := Convert(reportedPath, report)

		if err != nil || len(got) != 1 || got[0].Severity != want {
			t.Errorf("%s: got %+v, %v; want severity %s", severity, got, err, want)
		}
	}
}

func TestAFindingThatEndsAtTheStartOfALineEndsOnTheLineBefore(t *testing.T) {
	report := encode(t, entry("excessive-permissions", "d", "Medium", false, location(reportedPath, "Primary", "a", 3, 2, 8, 0)))

	got, err := Convert(reportedPath, report)

	if err != nil || len(got) != 1 || got[0].Line != 4 || got[0].EndLine != 8 {
		t.Errorf("got %+v, %v; want lines 4 to 8", got, err)
	}
}

func TestAFindingWithSeveralPrimaryLocationsIsReportedAtTheFirst(t *testing.T) {
	// use-trusted-publishing names both the action and the token input it was given.
	report := encode(t, entry("use-trusted-publishing", "d", "High", false,
		location(reportedPath, "Primary", "the action", 5, 8, 5, 20),
		location(reportedPath, "Related", "this job", 1, 0, 9, 0),
		location(reportedPath, "Primary", "the token", 7, 10, 7, 30)))

	got, err := Convert(reportedPath, report)

	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got[0].Line != 6 || got[0].Col != 9 || !strings.HasSuffix(got[0].Message, ": the action") {
		t.Errorf("got %+v; want the finding at the first primary location", got[0])
	}
}

func TestAFindingZizmorMarksIgnoredIsReportedLikeAnyOther(t *testing.T) {
	// Told to honour no ignore comment, zizmor still reports the finding a comment names, marked
	// ignored. The mark is not ours to act on: only the project record suppresses a finding.
	report := encode(t, entry("artipacked", "d", "High", true, location(reportedPath, "Primary", "a", 0, 0, 0, 1)))

	got, err := Convert(reportedPath, report)

	if err != nil || len(got) != 1 || got[0].ID != "actionsqaci.zizmor.artipacked" || got[0].Excepted {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestNoFindingsIsAnEmptyListAndNotAnError(t *testing.T) {
	got, err := Convert(reportedPath, []byte("[]"))

	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestWhatCannotBeReadAsAFindingIsAnErrorAndNeverCleanOrDropped(t *testing.T) {
	primary := location(reportedPath, "Primary", "a", 0, 0, 0, 1)
	cases := map[string][]byte{
		"an audit this toolchain has no identifier for": encode(t, entry("brand-new-audit", "d", "High", false, primary)),
		"a finding with no primary location":            encode(t, entry("artipacked", "d", "High", false, location(reportedPath, "Related", "a", 0, 0, 0, 1))),
		"a finding in another file":                     encode(t, entry("artipacked", "d", "High", false, location(".github/workflows/other.yml", "Primary", "a", 0, 0, 0, 1))),
		"a severity zizmor never had":                   encode(t, entry("artipacked", "d", "Catastrophic", false, primary)),
		"a finding with no description":                 encode(t, entry("artipacked", "", "High", false, primary)),
		"an end before the start":                       encode(t, entry("artipacked", "d", "High", false, location(reportedPath, "Primary", "a", 5, 0, 2, 0))),
		"a report that is not JSON":                     []byte("not json"),
		"a report that is not a list":                   []byte(`{"ident":"artipacked"}`),
		"a report that is null":                         []byte("null"),
		"a report with something after the list":        []byte("[] []"),
		"no report at all":                              nil,
	}

	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Convert(reportedPath, report)

			if err == nil {
				t.Errorf("got %+v and no error", got)
			}
			if got != nil {
				t.Errorf("an error came with findings %+v", got)
			}
		})
	}
}

func TestAnUnknownAuditIsRefusedByName(t *testing.T) {
	report := encode(t, entry("brand-new-audit", "d", "High", false, location(reportedPath, "Primary", "a", 0, 0, 0, 1)))

	_, err := Convert(reportedPath, report)

	if err == nil || !strings.Contains(err.Error(), "brand-new-audit") {
		t.Errorf("got %v; want the audit named", err)
	}
}

func TestEveryAuditGivesAnIdentifierOfTheSchemeAndAPage(t *testing.T) {
	for _, audit := range Audits() {
		id := ID(audit.Name)

		kind, err := finding.ClassifyIdentifier(id)

		if err != nil || kind != finding.KindZizmor {
			t.Errorf("%s: %v, %v", audit.Name, kind, err)
		}
		if strings.TrimSpace(audit.Summary) == "" {
			t.Errorf("%s has no summary", audit.Name)
		}
	}
}

func TestEachAuditIsListedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, audit := range Audits() {
		if seen[audit.Name] {
			t.Errorf("%s is listed twice", audit.Name)
		}
		seen[audit.Name] = true
	}
	if len(seen) == 0 {
		t.Error("no audit is listed")
	}
}
