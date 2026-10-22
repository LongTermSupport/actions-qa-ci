package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

const workflowFile = "ci.yml"

func finds(id, path string) finding.Finding {
	return finding.Finding{
		ID:       id,
		Tool:     "actions-qa-ci",
		Path:     path,
		Line:     3,
		Col:      5,
		Severity: finding.SeverityError,
		Message:  "workflow drifted",
	}
}

// inWorkdir makes a working directory holding one workflow file and returns its relative path.
func inWorkdir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, workflowFile), []byte("name: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	return workflowFile
}

func harnessEnv(t *testing.T, check func(string) ([]finding.Finding, error)) cli.Env {
	t.Helper()

	return envWith(t,
		registry.Rule{ID: templateDrift, Summary: "A workflow matches its template", Check: check},
		registry.Rule{ID: lintCoverage, Summary: "Every workflow is linted"},
	)
}

func TestRuleReportsWhatTheRuleFoundOnTheFileAndExitsWithFindings(t *testing.T) {
	path := inWorkdir(t)
	env := harnessEnv(t, func(p string) ([]finding.Finding, error) {
		return []finding.Finding{finds(templateDrift, p)}, nil
	})

	got := runWith(t, env, "rule", templateDrift, path)

	if got.code != cli.ExitFindings {
		t.Fatalf("exit %d; stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "ci.yml:3:5 "+templateDrift+" workflow drifted\n") ||
		!strings.HasSuffix(got.stdout, "1 finding\n") {
		t.Errorf("stdout:\n%s", got.stdout)
	}
}

func TestRuleSaysSoWhenTheRuleDoesNotFire(t *testing.T) {
	path := inWorkdir(t)
	env := harnessEnv(t, func(string) ([]finding.Finding, error) { return nil, nil })

	got := runWith(t, env, "rule", templateDrift, path)

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleHandsTheRuleTheRepositoryRelativePathNotTheOneTyped(t *testing.T) {
	inWorkdir(t)
	var seen string
	env := harnessEnv(t, func(p string) ([]finding.Finding, error) {
		seen = p

		return nil, nil
	})

	absolute, err := filepath.Abs(workflowFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, typed := range []string{absolute, "./" + workflowFile, "sub/../" + workflowFile} {
		seen = ""
		if err := os.MkdirAll("sub", 0o700); err != nil {
			t.Fatal(err)
		}

		got := runWith(t, env, "rule", templateDrift, typed)

		if got.code != cli.ExitOK || seen != workflowFile {
			t.Errorf("%s: exit %d, rule saw %q, want %q; stderr: %s", typed, got.code, seen, workflowFile, got.stderr)
		}
	}
}

func TestRulePrintsOnlyTheNamedRulesFindings(t *testing.T) {
	path := inWorkdir(t)
	env := harnessEnv(t, func(p string) ([]finding.Finding, error) {
		return []finding.Finding{finds(templateDrift, p), finds(lintCoverage, p)}, nil
	})

	got := runWith(t, env, "rule", templateDrift, path)

	if got.code != cli.ExitToolchainFault || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
	if !strings.Contains(got.stderr, lintCoverage) {
		t.Errorf("stderr should name the identifier the rule was not entitled to report:\n%s", got.stderr)
	}
}

func TestRuleReportsARuleThatFailsAsAToolchainFault(t *testing.T) {
	path := inWorkdir(t)
	env := harnessEnv(t, func(string) ([]finding.Finding, error) {
		return nil, errors.New("the parser fell over")
	})

	got := runWith(t, env, "rule", templateDrift, path)

	if got.code != cli.ExitToolchainFault || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
	if !strings.Contains(got.stderr, templateDrift) || !strings.Contains(got.stderr, "the parser fell over") {
		t.Errorf("stderr should name the rule and the cause:\n%s", got.stderr)
	}
}

func TestRuleRefusesARuleThatCannotRunOnItsOwn(t *testing.T) {
	path := inWorkdir(t)

	got := runWith(t, harnessEnv(t, nil), "rule", lintCoverage, path)

	if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, lintCoverage) {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestRuleNamesTheNearestIdentifiersForAnUnknownOne(t *testing.T) {
	path := inWorkdir(t)

	got := runWith(t, harnessEnv(t, nil), "rule", "actionsqaci.templatedrift", path)

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "unknown rule") || !strings.Contains(got.stderr, templateDrift) {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestRuleRefusesAPathThatIsNotAFileInsideTheRepository(t *testing.T) {
	inWorkdir(t)
	if err := os.Mkdir("a-directory", 0o700); err != nil {
		t.Fatal(err)
	}
	env := harnessEnv(t, func(string) ([]finding.Finding, error) { return nil, nil })

	for _, path := range []string{"missing.yml", "a-directory", "../outside.yml", "/etc/hostname"} {
		got := runWith(t, env, "rule", templateDrift, path)

		if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, path) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", path, got.code, got.stdout, got.stderr)
		}
	}
}

func TestRuleTakesAFormat(t *testing.T) {
	path := inWorkdir(t)
	env := harnessEnv(t, func(p string) ([]finding.Finding, error) {
		return []finding.Finding{finds(templateDrift, p)}, nil
	})

	asJSON := runWith(t, env, "rule", "--format", "json", templateDrift, path)
	if asJSON.code != cli.ExitFindings || !strings.HasPrefix(asJSON.stdout, "[") {
		t.Errorf("json: exit %d, stdout %q", asJSON.code, asJSON.stdout)
	}

	asText := runWith(t, env, "rule", "--format", "text", templateDrift, path)
	if asText.code != cli.ExitFindings || !strings.HasPrefix(asText.stdout, "ci.yml\n  3:5  error") {
		t.Errorf("text: exit %d, stdout %q", asText.code, asText.stdout)
	}

	unknown := runWith(t, env, "rule", "--format", "xml", templateDrift, path)
	if unknown.code != cli.ExitUsage || !strings.Contains(unknown.stderr, "xml") {
		t.Errorf("unknown format: exit %d, stderr %q", unknown.code, unknown.stderr)
	}
}

func TestRuleNeedsAnIdentifierAndAPath(t *testing.T) {
	inWorkdir(t)
	env := harnessEnv(t, nil)

	for _, args := range [][]string{{"rule"}, {"rule", templateDrift}, {"rule", templateDrift, "a", "b"}, {"rule", "--format"}, {"rule", "--bogus", templateDrift, workflowFile}} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitUsage || got.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q", args, got.code, got.stdout)
		}
	}
}

func TestRuleAnIdentifierThatLooksLikeAFlagIsStillAnIdentifier(t *testing.T) {
	path := inWorkdir(t)

	got := runWith(t, harnessEnv(t, nil), "rule", "--", "-x", path)

	if got.code != cli.ExitUsage || got.stdout != "" {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}
