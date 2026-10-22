package firstparty_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/agentsummary"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

func summaryRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg, err := registry.New(
		registry.Rule{ID: "actionsqaci.templateDrift", Summary: "A workflow matches its template"},
		registry.Rule{ID: "actionsqaci.lintCoverage", Summary: "Every tracked workflow-shaped file is analysed"},
	)
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

// currentRegion is the region `rules --write-agent-summary` would write for the registry, with
// the record's narrowings if it has any.
func currentRegion(t *testing.T, reg *registry.Registry, rec *record.Record) string {
	t.Helper()

	entries := listing.Active(reg)
	listing.Narrow(reg, entries, rec)
	region, err := agentsummary.Region(entries)
	if err != nil {
		t.Fatal(err)
	}

	return region
}

func summaryCheck(reg *registry.Registry) func(string) ([]finding.Finding, error) {
	return firstparty.CheckAgentSummaryCurrent(everythingIsKnown, func() *registry.Registry { return reg })
}

func TestAnAgentFileWhoseRegionIsCurrentIsClean(t *testing.T) {
	reg := summaryRegistry(t)
	inTree(t, map[string]string{
		"CLAUDE.md": "# Project\n\n" + currentRegion(t, reg, nil) + "\n\nMore.\n",
		"AGENTS.md": currentRegion(t, reg, nil) + "\n",
	})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none", got, err)
	}
}

func TestAProjectThatDoesNotUseTheSummaryIsLeftAlone(t *testing.T) {
	reg := summaryRegistry(t)
	inTree(t, map[string]string{"CLAUDE.md": "# Project\n\nNo region here.\n", "other.md": agentsummary.Begin + "\n"})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; a file with no region has nothing to be stale", got, err)
	}
}

func TestAStaleRegionIsAFindingAtItsBeginMarkerNamingTheFix(t *testing.T) {
	reg := summaryRegistry(t)
	stale := strings.Replace(currentRegion(t, reg, nil), "A workflow matches its template", "An old summary", 1)
	inTree(t, map[string]string{
		"CLAUDE.md": "# Project\n\n" + currentRegion(t, reg, nil) + "\n",
		"AGENTS.md": "# Agents\n\ntext\n\n" + stale + "\n",
	})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 1 {
		t.Fatalf("findings %+v, error %v; want one, for AGENTS.md only", got, err)
	}
	f := got[0]
	if f.ID != firstparty.AgentSummaryCurrent || f.Path != "AGENTS.md" || f.Line != 5 || f.Severity != finding.SeverityError {
		t.Errorf("finding %+v; want an error at the begin marker, line 5 of AGENTS.md", f)
	}
	if !strings.Contains(f.Message, "rules --write-agent-summary AGENTS.md") {
		t.Errorf("message %q should name the command that brings it up to date", f.Message)
	}
	if err := f.Validate(); err != nil {
		t.Error(err)
	}
}

func TestARegionThatLacksARuleTheBuildHasIsStale(t *testing.T) {
	reg := summaryRegistry(t)
	smaller, err := registry.New(registry.Rule{ID: "actionsqaci.templateDrift", Summary: "A workflow matches its template"})
	if err != nil {
		t.Fatal(err)
	}
	inTree(t, map[string]string{"CLAUDE.md": currentRegion(t, smaller, nil) + "\n"})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != "CLAUDE.md" {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestTheRecordsNarrowingsAreInTheRegionItShouldHave(t *testing.T) {
	reg := summaryRegistry(t)
	narrowing := "version: 1\nexceptions:\n  - id: actionsqaci.templateDrift\n    hazard: \"The template check would report copies that the project keeps differing on purpose\"\n" +
		"    scope: \"Everywhere, because this project ships no template and keeps no copies of one\"\n    decision: \"Owner: https://example.com/decision\"\n"
	rec, err := record.Parse([]byte(narrowing), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}
	inTree(t, map[string]string{
		recordPath:  narrowing,
		"CLAUDE.md": currentRegion(t, reg, nil) + "\n",
		"AGENTS.md": currentRegion(t, reg, rec) + "\n",
	})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != "CLAUDE.md" {
		t.Errorf("findings %+v, error %v; the region written without the narrowing hides it, so it is stale", got, err)
	}
}

func TestMarkersThatDoNotFormOneRegionAreAFindingAtTheFile(t *testing.T) {
	reg := summaryRegistry(t)
	inTree(t, map[string]string{"CLAUDE.md": "# Project\n" + agentsummary.Begin + "\nnever closed\n"})

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != "CLAUDE.md" || !strings.Contains(got[0].Message, "exactly one") {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestAFileThatIsALinkOrAbsentIsNotCompared(t *testing.T) {
	reg := summaryRegistry(t)
	inTree(t, map[string]string{"AGENTS.md": strings.Replace(currentRegion(t, reg, nil), "template", "stale", 1) + "\n"})
	if err := os.Symlink("AGENTS.md", "CLAUDE.md"); err != nil {
		t.Fatal(err)
	}

	got, err := summaryCheck(reg)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != "AGENTS.md" {
		t.Errorf("findings %+v, error %v; the file behind the link is judged once, as AGENTS.md", got, err)
	}
}

func TestAnAgentFileThatCannotBeReadIsNoVerdict(t *testing.T) {
	reg := summaryRegistry(t)
	inTree(t, map[string]string{"AGENTS.md": strings.Repeat("x", 3<<20)})

	_, err := summaryCheck(reg)(recordPath)

	if !errors.Is(err, check.ErrNotAnalysable) {
		t.Errorf("error = %v; a file too big to compare is not a clean result", err)
	}
}

func TestTheCheckNeedsTheRegistryAndAValidRecord(t *testing.T) {
	inTree(t, map[string]string{"CLAUDE.md": "x\n"})
	if _, err := firstparty.CheckAgentSummaryCurrent(everythingIsKnown, func() *registry.Registry { return nil })(recordPath); err == nil {
		t.Error("no registry: want an error, not a clean result")
	}

	inTree(t, map[string]string{recordPath: "version: 1\nbogus: true\n"})
	if _, err := summaryCheck(summaryRegistry(t))(recordPath); !errors.Is(err, record.ErrInvalid) {
		t.Errorf("an invalid record: error = %v", err)
	}
}

func TestAgentSummaryCurrentIsARegisteredRuleWithASummary(t *testing.T) {
	rule := firstparty.AgentSummaryRule(everythingIsKnown, func() *registry.Registry { return nil })

	if rule.ID != "actionsqaci.agentSummaryCurrent" || rule.Summary == "" || rule.Check == nil {
		t.Errorf("rule %+v", rule)
	}
}
