package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const (
	codesName = "actionsqaci.shellcheck.SC<nnnn>"
	codesPage = "# actionsqaci.shellcheck\n\nthe family page\n"
)

var codesMember = regexp.MustCompile(`^actionsqaci\.shellcheck\.SC[0-9]{4}$`)

// familyEnv has a registry with the codes family, which reports one valid finding for the
// member it was asked about, and the family page.
func familyEnv(t *testing.T, rules ...registry.Rule) cli.Env {
	t.Helper()

	reg, err := registry.NewWithFamilies([]registry.Family{{
		Name:     codesName,
		Page:     "actionsqaci.shellcheck",
		Summary:  "Shell in run: steps is free of ShellCheck warnings",
		Upstream: "shellcheck v0.11.0",
		Member:   codesMember.MatchString,
		Check: func(id string) func(string) ([]finding.Finding, error) {
			return func(path string) ([]finding.Finding, error) {
				return []finding.Finding{{
					ID: id, Tool: "shellcheck v0.11.0", UpstreamID: strings.TrimPrefix(id, "actionsqaci.shellcheck."),
					Path: path, Line: 1, Col: 1, Severity: finding.SeverityWarning, Message: "m", DocPath: registry.DocPath(id),
				}}, nil
			}
		},
	}}, rules...)
	if err != nil {
		t.Fatal(err)
	}

	return cli.Env{
		Registry: reg,
		Pages: fstest.MapFS{
			"docs/rules/actionsqaci.shellcheck.md": {Data: []byte(codesPage)},
		},
	}
}

func TestRulesListsAFamilyOnceUnderItsPlaceholderName(t *testing.T) {
	got := runWith(t, familyEnv(t, twoRules()...), "rules")

	want := codesName + "  Shell in run: steps is free of ShellCheck warnings\n" +
		templateDrift + "  A workflow matches its template\n" +
		templateInj + "  No untrusted input in run:\n"
	if got.code != cli.ExitOK || got.stdout != want {
		t.Errorf("exit %d, output:\n%q\nwant:\n%q", got.code, got.stdout, want)
	}
}

func TestRulesAsJSONCarriesTheFamilyWithItsSharedPage(t *testing.T) {
	got := runWith(t, familyEnv(t), "rules", "--json")

	var listed []map[string]string
	if err := json.Unmarshal([]byte(got.stdout), &listed); err != nil {
		t.Fatalf("not a JSON list: %v\n%s", err, got.stdout)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d entries, want 1: %v", len(listed), listed)
	}
	if listed[0]["id"] != codesName || listed[0]["upstream"] != "shellcheck v0.11.0" || listed[0]["docPath"] != "docs/rules/actionsqaci.shellcheck.md" {
		t.Errorf("entry = %v", listed[0])
	}
}

func TestRuleDocForAFamilyMemberPrintsTheFamilyPage(t *testing.T) {
	got := runWith(t, familyEnv(t), "rule-doc", "actionsqaci.shellcheck.SC2086")

	if got.code != cli.ExitOK || got.stdout != codesPage {
		t.Errorf("exit %d, output %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleDocRefusesWhatIsNotAMemberOfAnyFamily(t *testing.T) {
	for _, id := range []string{"actionsqaci.shellcheck.SC208", "actionsqaci.shellcheck.SC20866"} {
		got := runWith(t, familyEnv(t), "rule-doc", id)

		if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, id) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", id, got.code, got.stdout, got.stderr)
		}
	}
}

func TestTheHarnessRunsAFamilyMemberWithItsOwnIdentifier(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "w.yml"), []byte("on: push\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	got := runWith(t, familyEnv(t), "rule", "--format", "json", "actionsqaci.shellcheck.SC2164", "w.yml")

	if got.code != cli.ExitFindings {
		t.Fatalf("exit = %d; stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, `"id": "actionsqaci.shellcheck.SC2164"`) {
		t.Errorf("the finding does not carry the member's identifier:\n%s", got.stdout)
	}
}

func TestRuleDocAcceptsTheNameTheFamilyIsListedUnder(t *testing.T) {
	got := runWith(t, familyEnv(t), "rule-doc", codesName)

	if got.code != cli.ExitOK || got.stdout != codesPage {
		t.Errorf("exit %d, output %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestTheHarnessSaysAFamilyNeedsACodeNotTheListedName(t *testing.T) {
	got := runWith(t, familyEnv(t), "rule", codesName, "w.yml")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "family") || !strings.Contains(got.stderr, "code") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleDocForATypedCodeNearTheFamilySuggestsTheFamily(t *testing.T) {
	got := runWith(t, familyEnv(t), "rule-doc", "actionsqaci.shellcheck.SC<nnn>")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, codesName) {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}
