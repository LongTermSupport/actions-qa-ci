package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

type listedRule struct {
	ID         string   `json:"id"`
	NarrowedBy []string `json:"narrowedBy"`
}

func TestRulesSaysWhichDefencesTheRecordDisablesEverywhere(t *testing.T) {
	repository(t, map[string]string{record.Path: excepting(
		entry(templateDrift, workflowPath, ""),
		entry(templateInj, "", ""),
		entry("actionsqaci.shellcheck.SC2086", "", ""),
	)})
	env := familyEnv(t, twoRules()...)

	text := runWith(t, env, "rules")

	if text.code != cli.ExitOK {
		t.Fatalf("exit %d, stderr %q", text.code, text.stderr)
	}
	for _, want := range []string{
		templateInj + "  No untrusted input in run:\n    narrowed: actionsqaci.zizmor.template-injection is disabled everywhere by " + record.Path + ":8\n",
		codesName + "  Shell in run: steps is free of ShellCheck warnings\n    narrowed: actionsqaci.shellcheck.SC2086 is disabled everywhere by " + record.Path + ":12\n",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("rules lacks %q:\n%s", want, text.stdout)
		}
	}
	if strings.Contains(text.stdout, "narrowed: "+templateDrift) || strings.Count(text.stdout, "narrowed:") != 2 {
		t.Errorf("an exception with a path narrows nothing everywhere, and is not listed:\n%s", text.stdout)
	}

	asJSON := runWith(t, env, "rules", "--json")
	var listed []listedRule
	if err := json.Unmarshal([]byte(asJSON.stdout), &listed); err != nil {
		t.Fatal(err)
	}
	narrowed := map[string]int{}
	for _, rule := range listed {
		narrowed[rule.ID] = len(rule.NarrowedBy)
	}
	if narrowed[templateInj] != 1 || narrowed[codesName] != 1 || narrowed[templateDrift] != 0 {
		t.Errorf("narrowedBy by rule = %v", narrowed)
	}
}

func TestRulesListsNothingNarrowedWithoutARecordAndKeepsItsJSONShape(t *testing.T) {
	repository(t, map[string]string{"README.md": "# x\n"})

	got := runWith(t, familyEnv(t, twoRules()...), "rules", "--json")

	if got.code != cli.ExitOK || strings.Contains(got.stdout, "narrowedBy") {
		t.Errorf("exit %d, stdout:\n%s", got.code, got.stdout)
	}
}

func TestRulesRefusesAnInvalidRecordInsteadOfListingAsIfItWereNotThere(t *testing.T) {
	repository(t, map[string]string{record.Path: "version: 1\nsurprise: 1\n"})

	got := runWith(t, familyEnv(t, twoRules()...), "rules")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "not valid") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRulesRunBelowTheRepositoryRootSaysItDidNotLookForTheRecord(t *testing.T) {
	repository(t, map[string]string{record.Path: excepting(entry(templateInj, "", ""))})
	if err := os.Chdir(".github"); err != nil {
		t.Fatal(err)
	}

	got := runWith(t, familyEnv(t, twoRules()...), "rules")

	if got.code != cli.ExitOK || strings.Contains(got.stdout, "narrowed") || !strings.Contains(got.stderr, "repository root") {
		t.Errorf("exit %d, stdout %q, stderr %q; silence here would read as nothing being narrowed", got.code, got.stdout, got.stderr)
	}
}

func TestRulesRunOutsideAnyRepositoryHasNothingToSay(t *testing.T) {
	t.Chdir(t.TempDir())

	got := runWith(t, familyEnv(t, twoRules()...), "rules")

	if got.code != cli.ExitOK || got.stderr != "" {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}
