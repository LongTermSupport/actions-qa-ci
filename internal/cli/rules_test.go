package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const (
	templateDrift = "actionsqaci.templateDrift"
	lintCoverage  = "actionsqaci.lintCoverage"
	templateInj   = "actionsqaci.zizmor.template-injection"
	driftPage     = "# actionsqaci.templateDrift\n\nthe page body\n"
)

type result struct {
	code   int
	stdout string
	stderr string
}

func envWith(t *testing.T, rules ...registry.Rule) cli.Env {
	t.Helper()

	reg, err := registry.New(rules...)
	if err != nil {
		t.Fatal(err)
	}

	return cli.Env{
		Registry: reg,
		Pages: fstest.MapFS{
			"docs/rules/" + templateDrift + ".md": {Data: []byte(driftPage)},
		},
	}
}

func runWith(t *testing.T, env cli.Env, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.RunWith(env, args, &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func twoRules() []registry.Rule {
	return []registry.Rule{
		{ID: templateDrift, Summary: "A workflow matches its template"},
		{ID: templateInj, Summary: "No untrusted input in run:", Upstream: "zizmor v1.31.1"},
	}
}

func TestRulesListsEveryActiveRuleInIdentifierOrder(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rules")

	if got.code != cli.ExitOK {
		t.Fatalf("exit = %d; stderr: %s", got.code, got.stderr)
	}
	want := templateDrift + "  A workflow matches its template\n" +
		templateInj + "  No untrusted input in run:\n"
	if got.stdout != want {
		t.Errorf("rules output:\n%q\nwant:\n%q", got.stdout, want)
	}
}

func TestRulesSaysSoWhenNoRuleIsActive(t *testing.T) {
	got := runWith(t, envWith(t), "rules")

	if got.code != cli.ExitOK || got.stdout != "no rules are active\n" {
		t.Errorf("exit %d, output %q", got.code, got.stdout)
	}
}

func TestRulesAsJSONCarriesTheIdentifierSummaryUpstreamAndPage(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rules", "--json")

	if got.code != cli.ExitOK {
		t.Fatalf("exit = %d; stderr: %s", got.code, got.stderr)
	}

	var listed []map[string]string
	if err := json.Unmarshal([]byte(got.stdout), &listed); err != nil {
		t.Fatalf("not a JSON list: %v\n%s", err, got.stdout)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d rules, want 2", len(listed))
	}
	first := listed[0]
	if first["id"] != templateDrift || first["summary"] != "A workflow matches its template" ||
		first["upstream"] != "" || first["docPath"] != "docs/rules/"+templateDrift+".md" {
		t.Errorf("first entry = %v", first)
	}
	if listed[1]["upstream"] != "zizmor v1.31.1" {
		t.Errorf("second entry lost its upstream: %v", listed[1])
	}
}

func TestRulesAsJSONPrintsAnEmptyListNotNull(t *testing.T) {
	got := runWith(t, envWith(t), "rules", "--json")

	if strings.TrimSpace(got.stdout) != "[]" {
		t.Errorf("output = %q, want []", got.stdout)
	}
}

func TestRulesRefusesAnUnknownFlagOrArgument(t *testing.T) {
	for _, args := range [][]string{{"rules", "--xml"}, {"rules", "extra"}} {
		got := runWith(t, envWith(t, twoRules()...), args...)

		if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, args[1]) {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, got.code, got.stdout, got.stderr)
		}
	}
}

func TestRuleDocPrintsThePageFromTheEmbeddedCopy(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rule-doc", templateDrift)

	if got.code != cli.ExitOK || got.stdout != driftPage {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleDocNamesTheNearestIdentifiersForAnUnknownOne(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rule-doc", "actionsqaci.templatedrift")

	if got.code != cli.ExitUsage || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
	if !strings.Contains(got.stderr, "unknown rule") || !strings.Contains(got.stderr, templateDrift) {
		t.Errorf("stderr should say the rule is unknown and suggest %s:\n%s", templateDrift, got.stderr)
	}
}

func TestRuleDocForAnIdentifierNothingResemblesSuggestsNothing(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rule-doc", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz.zzzzzzzzzzz")

	if got.code != cli.ExitUsage {
		t.Errorf("exit %d", got.code)
	}
	if strings.Contains(got.stderr, "did you mean") {
		t.Errorf("a wild guess was offered:\n%s", got.stderr)
	}
}

func TestRuleDocRefusesAMalformedIdentifierWithoutTouchingThePages(t *testing.T) {
	got := runWith(t, envWith(t, twoRules()...), "rule-doc", "../README")

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "../README") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleDocNeedsExactlyOneIdentifier(t *testing.T) {
	for _, args := range [][]string{{"rule-doc"}, {"rule-doc", templateDrift, lintCoverage}} {
		got := runWith(t, envWith(t, twoRules()...), args...)

		if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, "usage:") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, got.code, got.stdout, got.stderr)
		}
	}
}

func TestRuleDocForARegisteredRuleWithNoPageIsAToolchainFault(t *testing.T) {
	// templateInj is registered but envWith embeds only templateDrift's page.
	got := runWith(t, envWith(t, twoRules()...), "rule-doc", templateInj)

	if got.code != cli.ExitToolchainFault || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
	if !strings.Contains(got.stderr, templateInj) {
		t.Errorf("stderr should name the rule whose page is missing:\n%s", got.stderr)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("the output is closed")
}

func TestACommandWhoseOutputCannotBeWrittenIsAToolchainFault(t *testing.T) {
	commands := [][]string{
		{"help"},
		{"version"},
		{"version", "--json"},
		{"rules"},
		{"rules", "--json"},
		{"rule-doc", templateDrift},
	}

	for _, args := range commands {
		var stderr bytes.Buffer

		code := cli.RunWith(envWith(t, twoRules()...), args, brokenWriter{}, &stderr)

		if code != cli.ExitToolchainFault {
			t.Errorf("%v: exit %d, want %d: a lost result must not look like success", args, code, cli.ExitToolchainFault)
		}
		if !strings.Contains(stderr.String(), "cannot write output") {
			t.Errorf("%v: stderr should say the output could not be written:\n%s", args, stderr.String())
		}
	}
}

// The production entry point wires the built-in registry and the pages embedded in the binary.
func TestTheProgramRunsAgainstTheRulesAndPagesItShips(t *testing.T) {
	const shipped = "actionsqaci.actionlint.expression"
	var stdout, stderr bytes.Buffer

	if code := cli.Run([]string{"rules"}, &stdout, &stderr); code != cli.ExitOK {
		t.Fatalf("exit %d; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), shipped+"  ") {
		t.Errorf("rules output does not list %s:\n%s", shipped, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"rule-doc", shipped}, &stdout, &stderr); code != cli.ExitOK {
		t.Fatalf("exit %d; stderr: %s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "# "+shipped+"\n") {
		t.Errorf("rule-doc printed:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"rule-doc", "actionsqaci.shellcheck.SC2115"}, &stdout, &stderr); code != cli.ExitOK {
		t.Fatalf("exit %d for a ShellCheck code; stderr: %s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "# actionsqaci.shellcheck.SC<nnnn>\n") {
		t.Errorf("rule-doc printed for a ShellCheck code:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"rule-doc", templateDrift}, &stdout, &stderr); code != cli.ExitUsage {
		t.Errorf("exit %d for a rule that does not ship, want %d", code, cli.ExitUsage)
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	got := runWith(t, envWith(t), "help")

	for _, command := range []string{"version", "rules", "rule-doc"} {
		if !strings.Contains(got.stdout, command) {
			t.Errorf("help does not mention %s:\n%s", command, got.stdout)
		}
	}
}
