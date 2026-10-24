package catalogue_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/zizmor"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

func TestTheShippedRulesFormAValidRegistry(t *testing.T) {
	if _, err := catalogue.Registry(); err != nil {
		t.Fatalf("the rules this build ships do not form a registry: %v", err)
	}
}

// The self-audit: every shipped rule has a complete page, and every page is for a shipped
// rule. A rule added without its page, or a page left behind, fails here.
func TestEveryShippedRuleHasACompletePageAndEveryPageHasARule(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	for _, problem := range pages.Audit(reg, os.DirFS("../..")) {
		t.Error(problem)
	}
}

func TestTheShippedRegistryResolvesEveryShellcheckCodeThroughTheFamily(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	rule, known := reg.Lookup("actionsqaci.shellcheck.SC2115")
	if !known || rule.Check == nil || rule.Summary == "" {
		t.Errorf("SC2115 does not resolve to a runnable rule: %+v, %v", rule, known)
	}
	if len(reg.Families()) != 1 {
		t.Errorf("families = %+v, want the ShellCheck family alone", reg.Families())
	}
}

func TestTheShippedRegistryHasARunnableRuleForEveryZizmorAudit(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	for _, audit := range zizmor.Audits() {
		rule, known := reg.Lookup(zizmor.ID(audit.Name))
		if !known || rule.Check == nil || rule.Upstream != zizmor.Tool() || !strings.HasPrefix(rule.Summary, audit.Summary) {
			t.Errorf("%s does not resolve to a runnable rule of zizmor: %+v, %v", audit.Name, rule, known)
		}
		// A rule is listed as a standing defence, so one this build does not run says so there.
		if audit.Held == "" && !audit.WiderPersonaOnly && rule.Summary != audit.Summary {
			t.Errorf("%s: summary %q, want %q", audit.Name, rule.Summary, audit.Summary)
		}
		// Likewise an audit `check` does not report is not listed as one it enforces.
		if audit.WiderPersonaOnly && !strings.Contains(rule.Summary, "reported by the rule only") {
			t.Errorf("%s: summary %q does not say check leaves the audit out", audit.Name, rule.Summary)
		}
		if audit.Held != "" && !strings.Contains(rule.Summary, audit.Held) {
			t.Errorf("%s: summary %q does not say why the audit is held: %q", audit.Name, rule.Summary, audit.Held)
		}
	}
}

func TestTheZizmorLaneNamesEveryAuditThisBuildHoldsAndNoOther(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	var lane *check.Lane
	for _, candidate := range catalogue.Lanes(reg) {
		if candidate.Name == "zizmor" {
			lane = &candidate
		}
	}
	if lane == nil {
		t.Fatal("there is no zizmor lane")
	}

	want := map[string]string{}
	for _, audit := range zizmor.Audits() {
		if audit.Held != "" {
			want[zizmor.ID(audit.Name)] = audit.Held
		}
	}
	got := map[string]string{}
	for _, notRun := range lane.NotRun {
		got[notRun.ID] = notRun.Reason
	}
	if !maps.Equal(got, want) {
		t.Errorf("the lane says it does not run %v, the audits held are %v", got, want)
	}
	for _, other := range catalogue.Lanes(reg) {
		if other.Name != "zizmor" && len(other.NotRun) != 0 {
			t.Errorf("the %s lane says it does not run %v; nothing of it is held", other.Name, other.NotRun)
		}
	}
}

func TestEveryReportActionlintDropsIsAbsorbedByARegisteredOwner(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	for _, duplicate := range actionlint.Duplicates() {
		owner, known := reg.Lookup(duplicate.Owner)
		if !known {
			t.Errorf("%s drops %s, and its owner %s is not a rule of this build", duplicate.Kind, duplicate.Report, duplicate.Owner)

			continue
		}
		if !slices.Contains(owner.Absorbs, duplicate.Report) {
			t.Errorf("%s absorbs %v, and does not say it stands for %q", owner.ID, owner.Absorbs, duplicate.Report)
		}
	}
}

func TestTheBundledToolsAreNamedWithTheirReleases(t *testing.T) {
	versions := map[string]string{}
	for _, extension := range catalogue.Extensions() {
		versions[extension.Name] = extension.Version
	}

	if versions["zizmor"] == "" || versions["zizmor"] != toolchain.Pinned("zizmor") {
		t.Errorf("extensions = %+v; zizmor must be named at the release the manifest pins", versions)
	}
	if versions["shellcheck"] != toolchain.Pinned("shellcheck") || versions["actionlint"] == "" {
		t.Errorf("extensions = %+v", versions)
	}
}

func TestTheJustificationCheckIsShippedAndKnowsEveryShippedIdentifier(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	rule, known := reg.Lookup("actionsqaci.unjustifiedException")
	if !known || rule.Check == nil {
		t.Fatalf("rule = %+v, known = %v", rule, known)
	}

	root := t.TempDir()
	record := "version: 1\nexceptions:\n" +
		"  - id: actionsqaci.shellcheck.SC2115\n    hazard: legacy\n    scope: legacy\n    decision: x\n" +
		"  - id: actionsqaci.actionlint.glob\n    hazard: legacy\n    scope: legacy\n    decision: x\n" +
		"  - id: actionsqaci.actionlint.id\n    hazard: legacy\n    scope: legacy\n    decision: x\n"
	if err := os.WriteFile(filepath.Join(root, "record.yml"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	found, err := rule.Check("record.yml")
	if err != nil {
		t.Fatalf("an exception naming a shipped identifier was refused as unknown: %v", err)
	}
	if len(found) != 9 {
		t.Errorf("found %d findings, want 3 for each of the 3 exceptions: %+v", len(found), found)
	}

	if err := os.WriteFile(filepath.Join(root, "record.yml"), []byte("version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if found, err := rule.Check("record.yml"); err == nil {
		t.Errorf("an exception naming no shipped identifier was accepted: %+v", found)
	}
}

func TestTheShippedLanesSelectExactlyWhatTheyAnalyse(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}
	lanes := catalogue.Lanes(reg)
	selects := map[string]func(string) bool{}
	for _, lane := range lanes {
		selects[lane.Name] = lane.Selects
	}

	cases := []struct {
		lane, path string
		want       bool
	}{
		{"actionlint", ".github/workflows/ci.yml", true},
		{"actionlint", ".github/workflows/ci.yaml", true},
		{"actionlint", ".github/workflows/sub/ci.yml", false},
		{"actionlint", ".github/workflows/ciyml", false},
		{"actionlint", ".github/workflows/ci.yml.bak", false},
		{"actionlint", "other/.github/workflows/ci.yml", false},
		{"actionlint", "action.yml", false},
		{"actionlint", ".github/actions-qa-ci.yml", false},
		{"firstParty", ".github/actions-qa-ci.yml", true},
		{"firstParty", ".github/workflows/ci.yml", false},
		{"firstParty", "sub/.github/actions-qa-ci.yml", false},
		{"templateDrift", ".github/actions-qa-ci.yml", true},
		{"templateDrift", ".github/workflows/ci.yml", false},
		{"templateDrift", "sub/.github/actions-qa-ci.yml", false},
		{"requiredChecks", ".github/actions-qa-ci.yml", true},
		{"requiredChecks", ".github/workflows/ci.yml", false},
		{"requiredChecks", "sub/.github/actions-qa-ci.yml", false},
		{"lintCoverage", ".github/actions-qa-ci.yml", true},
		{"lintCoverage", ".github/workflows/ci.yml", false},
		{"lintCoverage", "sub/.github/actions-qa-ci.yml", false},
		{"agentSummaryCurrent", ".github/actions-qa-ci.yml", true},
		{"agentSummaryCurrent", "CLAUDE.md", false},
		{"agentSummaryCurrent", "sub/.github/actions-qa-ci.yml", false},
		{"inlineSuppression", ".github/workflows/ci.yml", true},
		{"inlineSuppression", "action.yml", true},
		{"inlineSuppression", ".github/actions/x/action.yaml", true},
		{"inlineSuppression", "other/action.yml", false},
		{"inlineSuppression", ".github/dependabot.yml", false},
		{"inlineSuppression", ".github/actions-qa-ci.yml", false},
		{"inlineSuppression", "zizmor.yml", false},
		{"nativeSuppressionConfig", "zizmor.yml", true},
		{"nativeSuppressionConfig", ".github/actionlint.yaml", true},
		{"nativeSuppressionConfig", ".pinact.yaml", true},
		{"nativeSuppressionConfig", ".github/workflows/ci.yml", false},
		{"nativeSuppressionConfig", "sub/zizmor.yml", false},
		{"zizmor", ".github/workflows/ci.yml", true},
		{"zizmor", ".github/workflows/ci.yaml", true},
		{"zizmor", ".github/workflows/sub/ci.yml", false},
		{"zizmor", ".github/workflows/ci.yml.bak", false},
		{"zizmor", "action.yml", true},
		{"zizmor", ".github/actions/x/action.yaml", true},
		{"zizmor", "other/action.yml", false},
		{"zizmor", ".github/dependabot.yml", true},
		{"zizmor", ".github/dependabot.yaml", true},
		{"zizmor", "dependabot.yml", false},
		{"zizmor", "sub/.github/dependabot.yml", false},
		{"zizmor", ".github/actions-qa-ci.yml", false},
		{"zizmor", "zizmor.yml", false},
	}
	for _, c := range cases {
		if got := selects[c.lane](c.path); got != c.want {
			t.Errorf("lane %s selects %s = %v, want %v", c.lane, c.path, got, c.want)
		}
	}
	if len(lanes) != 9 {
		t.Errorf("lanes = %d; a lane added here needs cases above", len(lanes))
	}
}
