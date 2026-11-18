package ghfilter_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/ghfilter"
)

// The cases of GitHub's filter pattern cheat sheet ("Patterns to match file paths" and
// "Patterns to match branches and tags" in the workflow syntax reference), each example match
// and each stated non-match.
func TestMatchFollowsGitHubsFilterPatternCheatSheet(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "README.md", true},
		{"*", "server.rb", true},
		{"*", "docs/README.md", false},
		{"*.jsx?", "page.js", true},
		{"*.jsx?", "page.jsx", true},
		{"*.jsx?", "page.jsxx", false},
		{"**", "all/the/files.md", true},
		{"*.js", "app.js", true},
		{"*.js", "js/index.js", false},
		{"**.js", "index.js", true},
		{"**.js", "js/index.js", true},
		{"**.js", "src/js/app.js", true},
		{"docs/*", "docs/README.md", true},
		{"docs/*", "docs/mona/octocat.txt", false},
		{"docs/**", "docs/README.md", true},
		{"docs/**", "docs/mona/octocat.txt", true},
		{"docs/**", "other/docs/a.md", false},
		{"docs/**/*.md", "docs/README.md", true},
		{"docs/**/*.md", "docs/mona/hello-world.md", true},
		{"docs/**/*.md", "docs/a/markdown/file.md", true},
		{"**/docs/**", "docs/hello.md", true},
		{"**/docs/**", "dir/docs/my-file.txt", true},
		{"**/docs/**", "space/docs/plan/space.doc", true},
		{"**/README.md", "README.md", true},
		{"**/README.md", "js/README.md", true},
		{"**/README.md", "js/NOTREADME.md", false},
		{"**/*src/**", "a/src/app.js", true},
		{"**/*src/**", "my-src/code/js/app.js", true},
		{"**/*-post.md", "my-post.md", true},
		{"**/*-post.md", "path/their-post.md", true},
		{"**/migrate-*.sql", "migrate-10909.sql", true},
		{"**/migrate-*.sql", "db/migrate-v1.0.sql", true},
		{"**/migrate-*.sql", "db/sept/migrate-v1.sql", true},
		{"feature/*", "feature/my-branch", true},
		{"feature/*", "feature/beta-a/my-branch", false},
		{"feature/**", "feature/beta-a/my-branch", true},
		{"main", "main", true},
		{"main", "mainline", false},
		{"v2*", "v2.0", true},
		{"v[12].[0-9]+.[0-9]+", "v1.10.1", true},
		{"v[12].[0-9]+.[0-9]+", "v2.0.0", true},
		{"v[12].[0-9]+.[0-9]+", "v3.0.0", false},
		{"[CB]at", "Cat", true},
		{"[CB]at", "Hat", false},
		{"[1-2]00", "200", true},
		{"a.b", "aXb", false},
		{`\*.md`, "*.md", true},
		{`\*.md`, "a.md", false},
		{"CLAUDE/Plan/[0-9][0-9][0-9][0-9][0-9]-*/**", "CLAUDE/Plan/00197-x/PLAN.md", true},
		{"CLAUDE/Plan/[0-9][0-9][0-9][0-9][0-9]-*/**", "CLAUDE/Plan/CLAUDE.md", false},
	}

	for _, c := range cases {
		if got := ghfilter.Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestIncludedReadsThePatternsInOrderAndTheLastMatchWins(t *testing.T) {
	cases := []struct {
		patterns []string
		name     string
		want     bool
	}{
		{[]string{"*.md", "!README.md"}, "hello.md", true},
		{[]string{"*.md", "!README.md"}, "README.md", false},
		{[]string{"*.md", "!README.md"}, "docs/hello.md", false},
		{[]string{"*.md", "!README.md", "README*"}, "README.md", true},
		{[]string{"*.md", "!README.md", "README*"}, "README.doc", true},
		{[]string{"!README.md"}, "README.md", false},
		{[]string{"!README.md"}, "other.go", false},
		{nil, "anything", false},
	}

	for _, c := range cases {
		if got := ghfilter.Included(c.patterns, c.name); got != c.want {
			t.Errorf("Included(%q, %q) = %v, want %v", c.patterns, c.name, got, c.want)
		}
	}
}

// A paths filter of the shape that keeps a docs-only push from running CI while instruction
// Markdown and code still run it: everything, less plan folders, docs and Markdown, plus the
// Markdown agents load as instructions.
func TestADocsFilterWithReincludedInstructionMarkdown(t *testing.T) {
	filter := []string{
		"**",
		"!CLAUDE/Plan/[0-9][0-9][0-9][0-9][0-9]-*/**",
		"!CLAUDE/Plan/Completed/[0-9][0-9][0-9][0-9][0-9]-*/**",
		"!docs/**",
		"!**.md",
		"**/CLAUDE.md",
		"**/AGENTS.md",
		".claude/**",
		".github/**",
	}
	cases := map[string]bool{
		"README.md":                              false,
		"docs/runbook.md":                        false,
		"docs/diagram.png":                       false,
		"notes/design.md":                        false,
		"CLAUDE/Plan/00001-x/PLAN.md":            false,
		"CLAUDE/Plan/00001-x/deploy.bash":        false,
		"CLAUDE/Plan/Completed/00001-x/run.bash": false,
		"CLAUDE.md":                              true,
		"sub/CLAUDE.md":                          true,
		"AGENTS.md":                              true,
		".claude/rules/x.md":                     true,
		".github/workflows/ci.yml":               true,
		"CLAUDE/Plan/CLAUDE.md":                  true,
		"CLAUDE/Plan/mkplan.bash":                true,
		"playbooks/site.yml":                     true,
		"docs/CLAUDE.md":                         true,
	}

	for name, want := range cases {
		if got := ghfilter.Included(filter, name); got != want {
			t.Errorf("Included(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestIgnoredMatchesAPathAnyPatternMatches(t *testing.T) {
	ignore := []string{"**.md", "docs/**"}
	cases := map[string]bool{
		"README.md":        true,
		"docs/a.png":       true,
		"src/main.go":      false,
		"docs":             false,
		"sub/dir/notes.md": true,
	}
	for name, want := range cases {
		if got := ghfilter.Ignored(ignore, name); got != want {
			t.Errorf("Ignored(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestAPatternGitHubCannotReadMatchesNothing(t *testing.T) {
	for _, pattern := range []string{"", "[a", "a[]", "[!a]", `a\`} {
		if ghfilter.Match(pattern, "a") || ghfilter.Match(pattern, "") {
			t.Errorf("Match(%q) matched; a pattern that cannot be read must match nothing", pattern)
		}
		if ghfilter.Valid(pattern) {
			t.Errorf("Valid(%q) = true", pattern)
		}
	}
	for _, pattern := range []string{"**", "!docs/**", "v[12].[0-9]+", `\*.md`} {
		if !ghfilter.Valid(pattern) {
			t.Errorf("Valid(%q) = false", pattern)
		}
	}
}
