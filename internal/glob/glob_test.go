package glob_test

import (
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/glob"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"a.yml", "a.yml", true},
		{"a.yml", "b.yml", false},
		{"a.yml", "dir/a.yml", false},
		{"*.yml", "a.yml", true},
		{"*.yml", "dir/a.yml", false},
		{"*.yml", ".yml", true},
		{"dir/*.yml", "dir/a.yml", true},
		{"dir/*.yml", "dir/sub/a.yml", false},
		{"a?.yml", "ab.yml", true},
		{"a?.yml", "a.yml", false},
		{"a?.yml", "a/.yml", false},
		{"a?.yml", "aé.yml", true},
		{"**", "a.yml", true},
		{"**", "a/b/c.yml", true},
		{"tests/**", "tests/a.yml", true},
		{"tests/**", "tests/a/b.yml", true},
		{"tests/**", "tests", false},
		{"tests/**", "other/tests/a.yml", false},
		{"**/a.yml", "a.yml", true},
		{"**/a.yml", "x/y/a.yml", true},
		{"**/a.yml", "x/b.yml", false},
		{"a/**/b.yml", "a/b.yml", true},
		{"a/**/b.yml", "a/x/y/b.yml", true},
		{"a/**/b.yml", "x/a/b.yml", false},
		{"**/fixtures/**", "tests/fixtures/ci.yml", true},
		{"**/fixtures/**", "fixtures/ci.yml", true},
		{"**/fixtures/**", "tests/ci.yml", false},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXcYYb", false},
		{"*", "", true},
		{"a*", "a", true},
	}

	for _, c := range cases {
		if got := glob.Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestCheckAcceptsThePatternsTheRecordMayHold(t *testing.T) {
	for _, pattern := range []string{"a.yml", "*.yml", "tests/fixtures/**", "**/action.yml", "a/**/b?.yml", "templates/github-actions/**"} {
		if err := glob.Check(pattern); err != nil {
			t.Errorf("%q: %v", pattern, err)
		}
	}
}

func TestCheckRefusesWhatItCouldReadTwoWays(t *testing.T) {
	cases := map[string]string{
		"":            "empty",
		"/abs/*.yml":  "absolute",
		"dir/":        "end with",
		"a//b":        "empty segment",
		"a/../b":      "..",
		"./a":         "clean",
		"a\\b":        "backslash",
		"a/[ab].yml":  "class",
		"a/{x,y}.yml": "class",
		"!a.yml":      "negat",
		"a/**b":       "whole segment",
		"a/b**":       "whole segment",
		"a/***":       "whole segment",
		"a\x00b":      "control",
		"a\nb":        "control",
		"tests/‮":     "control",
	}

	for pattern, mention := range cases {
		err := glob.Check(pattern)

		if err == nil {
			t.Errorf("%q was accepted", pattern)

			continue
		}
		if !strings.Contains(err.Error(), mention) {
			t.Errorf("%q: error %q should mention %q", pattern, err, mention)
		}
	}
}

func TestMatchDoesNotTakeLongToRefuseAPathologicalPattern(t *testing.T) {
	pattern := strings.Repeat("*a", 40) + "b"
	name := strings.Repeat("a", 200)

	if glob.Match(pattern, name) {
		t.Errorf("matched")
	}
}
