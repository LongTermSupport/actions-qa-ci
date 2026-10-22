package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/agentsummary"
	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestRulesWritesTheActiveDefencesAsARegionOfAFileThatDidNotExist(t *testing.T) {
	repository(t, map[string]string{record.Path: excepting(entry(templateInj, "", ""))})
	env := familyEnv(t, twoRules()...)

	got := runWith(t, env, "rules", "--write-agent-summary=CLAUDE.md")

	if got.code != cli.ExitOK || !strings.Contains(got.stdout, templateDrift+"  A workflow matches its template") {
		t.Fatalf("exit %d, stdout %q, stderr %q; the listing is printed as it always is", got.code, got.stdout, got.stderr)
	}
	if got.stderr != "actions-qa: wrote the agent summary to CLAUDE.md\n" {
		t.Errorf("stderr = %q", got.stderr)
	}
	written := readFile(t, "CLAUDE.md")
	for _, want := range []string{
		agentsummary.Begin + "\n",
		"- `" + templateDrift + "` — A workflow matches its template\n",
		"  - narrowed: actionsqaci.zizmor.template-injection is disabled everywhere by " + record.Path + ":",
		"\n" + agentsummary.End + "\n",
	} {
		if !strings.Contains(written, want) {
			t.Errorf("CLAUDE.md lacks %q:\n%s", want, written)
		}
	}
}

func TestRulesKeepsTheRestOfTheFileAndASecondRunChangesNothing(t *testing.T) {
	repository(t, map[string]string{"AGENTS.md": "# Mine\n\nKeep this.\n"})
	env := familyEnv(t, twoRules()...)

	first := runWith(t, env, "rules", "--write-agent-summary", "AGENTS.md")
	afterFirst := readFile(t, "AGENTS.md")
	second := runWith(t, env, "rules", "--write-agent-summary", "AGENTS.md")

	if first.code != cli.ExitOK || second.code != cli.ExitOK {
		t.Fatalf("exits %d and %d, stderr %q and %q", first.code, second.code, first.stderr, second.stderr)
	}
	if !strings.HasPrefix(afterFirst, "# Mine\n\nKeep this.\n\n"+agentsummary.Begin) {
		t.Errorf("the project's own text must come first and be kept:\n%s", afterFirst)
	}
	if readFile(t, "AGENTS.md") != afterFirst {
		t.Error("a second run changed the file")
	}
}

func TestRulesReplacesTheFileInsteadOfRewritingItInPlace(t *testing.T) {
	repository(t, map[string]string{"CLAUDE.md": "# Mine\n\nKeep this.\n"})
	if err := os.Chmod("CLAUDE.md", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Link("CLAUDE.md", "elsewhere.md"); err != nil {
		t.Fatal(err)
	}

	got := runWith(t, familyEnv(t, twoRules()...), "rules", "--write-agent-summary=CLAUDE.md")

	if got.code != cli.ExitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	// A file that is written to in place can be cut short by a failure half way; one that is
	// replaced is whole before it is in place, so the other name for the old one still has all
	// of the old text.
	if readFile(t, "elsewhere.md") != "# Mine\n\nKeep this.\n" {
		t.Errorf("the file was rewritten in place: %q", readFile(t, "elsewhere.md"))
	}
	info, err := os.Stat("CLAUDE.md")
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, err = %v; the file keeps the permissions it had", info.Mode().Perm(), err)
	}
	left, err := filepath.Glob(".*")
	if err != nil || len(left) != 1 || left[0] != ".git" {
		t.Errorf("files beginning with a dot after the write: %v, %v; a temporary file was left", left, err)
	}
}

func TestRulesWritingTheSummaryStillPrintsTheListingWhenAsked(t *testing.T) {
	repository(t, map[string]string{})
	env := familyEnv(t, twoRules()...)

	got := runWith(t, env, "rules", "--json", "--write-agent-summary=CLAUDE.md")

	if got.code != cli.ExitOK || !strings.Contains(got.stdout, `"id": "`+templateDrift+`"`) {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(readFile(t, "CLAUDE.md"), templateDrift) {
		t.Error("the file was not written")
	}
}

func TestRulesRefusesToRewriteAFileWhoseRegionIsBroken(t *testing.T) {
	broken := "# Mine\n" + agentsummary.Begin + "\nno end\n"
	repository(t, map[string]string{"CLAUDE.md": broken})

	got := runWith(t, familyEnv(t, twoRules()...), "rules", "--write-agent-summary=CLAUDE.md")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "CLAUDE.md") || got.stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if readFile(t, "CLAUDE.md") != broken {
		t.Error("a file with a broken region was rewritten")
	}
}

func TestRulesWillNotWriteTheSummaryThroughALinkOrIntoADirectory(t *testing.T) {
	repository(t, map[string]string{"real.md": "keep\n"})
	if err := os.Symlink("real.md", "link.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("dir.md", 0o700); err != nil {
		t.Fatal(err)
	}
	env := familyEnv(t, twoRules()...)

	for _, target := range []string{"link.md", "dir.md"} {
		got := runWith(t, env, "rules", "--write-agent-summary="+target)

		if got.code == cli.ExitOK || got.stdout != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want a refusal", target, got.code, got.stdout, got.stderr)
		}
	}
	if readFile(t, "real.md") != "keep\n" {
		t.Error("the file behind a link was written")
	}
}

func TestRulesWritesNothingWhenTheRecordIsInvalid(t *testing.T) {
	repository(t, map[string]string{record.Path: "version: 1\nbogus: true\n"})

	got := runWith(t, familyEnv(t, twoRules()...), "rules", "--write-agent-summary=CLAUDE.md")

	if got.code != cli.ExitUsage {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if _, err := os.Stat("CLAUDE.md"); !os.IsNotExist(err) {
		t.Error("a summary was written from a record that is not valid, which hides what it narrows")
	}
}

func TestRulesWriteAgentSummaryNeedsAFile(t *testing.T) {
	repository(t, map[string]string{})
	env := familyEnv(t, twoRules()...)

	for name, args := range map[string][]string{
		"no value":    {"rules", "--write-agent-summary"},
		"empty value": {"rules", "--write-agent-summary="},
		"a flag":      {"rules", "--write-agent-summary", "--json"},
		"twice":       {"rules", "--write-agent-summary=a.md", "--write-agent-summary=b.md"},
	} {
		got := runWith(t, env, args...)

		if got.code != cli.ExitUsage || got.stdout != "" {
			t.Errorf("%s: exit %d, stdout %q", name, got.code, got.stdout)
		}
	}
	if entries, err := filepath.Glob("*.md"); err != nil || len(entries) != 0 {
		t.Errorf("files written: %v, %v", entries, err)
	}
}
