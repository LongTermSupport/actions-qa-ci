package record_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestParseReadsTheTemplatesAndTheirLines(t *testing.T) {
	data := "version: 1\ntemplates:\n  - source: templates/github-actions/php-qa-ci.yml\n    copies:\n      - .github/workflows/qa.yml\n      - examples/qa.yml\n  - source: a/b.yml\n    copies: [c/d.yml]\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	if len(rec.Templates) != 2 {
		t.Fatalf("templates = %+v", rec.Templates)
	}
	first, second := rec.Templates[0], rec.Templates[1]
	if first.Source != "templates/github-actions/php-qa-ci.yml" || len(first.Copies) != 2 || first.Copies[1] != "examples/qa.yml" || first.Line != 3 {
		t.Errorf("first template = %+v; want its source, two copies and line 3", first)
	}
	if second.Source != "a/b.yml" || len(second.Copies) != 1 || second.Line != 7 {
		t.Errorf("second template = %+v; want line 7", second)
	}
}

func TestARecordWithTemplatesAndExceptionsKeepsBoth(t *testing.T) {
	data := "version: 1\ntemplates:\n  - source: a.yml\n    copies: [b.yml]\nexceptions:\n" + goodException

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil || len(rec.Templates) != 1 || len(rec.Exceptions) != 1 || rec.Exceptions[0].Line != 6 {
		t.Errorf("record = %+v, err = %v; want one of each, the exception on line 6", rec, err)
	}
}

func TestParseRefusesATemplateThatDeclaresNothingUsable(t *testing.T) {
	cases := map[string]struct{ data, mention string }{
		"no source":                         {"version: 1\ntemplates:\n  - copies: [b.yml]\n", "source"},
		"no copies":                         {"version: 1\ntemplates:\n  - source: a.yml\n", "copies"},
		"empty copies":                      {"version: 1\ntemplates:\n  - source: a.yml\n    copies: []\n", "copies"},
		"an empty entry":                    {"version: 1\ntemplates:\n  -\n", "empty"},
		"a copy that is the source":         {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [a.yml]\n", "itself"},
		"a copy twice":                      {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [b.yml, b.yml]\n", "twice"},
		"a copy of two sources":             {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [c.yml]\n  - source: b.yml\n    copies: [c.yml]\n", "twice"},
		"a source that is another's copy":   {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [b.yml]\n  - source: b.yml\n    copies: [c.yml]\n", "both"},
		"an absolute source":                {"version: 1\ntemplates:\n  - source: /etc/x.yml\n    copies: [b.yml]\n", "source"},
		"a parent copy":                     {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [../b.yml]\n", "copies"},
		"a glob copy":                       {"version: 1\ntemplates:\n  - source: a.yml\n    copies: [.github/workflows/*.yml]\n", "copies"},
		"a backslash source":                {"version: 1\ntemplates:\n  - source: a\\b.yml\n    copies: [c.yml]\n", "source"},
		"a mode this build does not act on": {"version: 1\ntemplates:\n  - source: a.yml\n    mode: identical-except\n    copies: [b.yml]\n", "mode"},
		"copies as text":                    {"version: 1\ntemplates:\n  - source: a.yml\n    copies: b.yml\n", "unmarshal"},
		"templates as a map":                {"version: 1\ntemplates:\n  source: a.yml\n", "unmarshal"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), everythingIsKnown)

		var invalid *record.InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v; want the record refused as invalid", name, err)

			continue
		}
		if !strings.Contains(strings.Join(invalid.Reasons, "\n"), c.mention) {
			t.Errorf("%s: reasons %q should mention %q", name, invalid.Reasons, c.mention)
		}
	}
}

func TestParseRefusesEveryIndirectionInTheTemplatesSectionToo(t *testing.T) {
	cases := map[string]string{
		"an alias":  "version: 1\ntemplates:\n  - source: &s a.yml\n    copies: [*s]\n",
		"an anchor": "version: 1\ntemplates:\n  - &t\n    source: a.yml\n    copies: [b.yml]\n",
		"a tag":     "version: 1\ntemplates:\n  - source: !!str a.yml\n    copies: [b.yml]\n",
	}

	for name, data := range cases {
		if _, err := record.Parse([]byte(data), everythingIsKnown); err == nil {
			t.Errorf("%s: the record was accepted", name)
		}
	}
}
