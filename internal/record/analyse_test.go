package record_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const goodUnanalysed = "    - path: tests/fixtures/**\n" +
	"      justification:\n" +
	"        hazard: \"Fixture workflows hold the very mistakes the rules are tested against\"\n" +
	"        scope: \"Only the fixtures directory, whose files are never run by any workflow\"\n"

func TestParseReadsTheUnanalysedEntriesWithTheirLines(t *testing.T) {
	data := "version: 1\nanalyse:\n  unanalysed:\n" + goodUnanalysed +
		"    - path: docs/examples/*.yml\n      justification:\n        hazard: \"Examples shown in the documentation\"\n        scope: \"Only the examples\"\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Analyse.Unanalysed
	if len(got) != 2 || got[0].Path != "tests/fixtures/**" || got[0].Line != 4 || got[1].Path != "docs/examples/*.yml" || got[1].Line != 8 {
		t.Fatalf("unanalysed = %+v", got)
	}
	if !strings.HasPrefix(got[0].Justification.Hazard, "Fixture workflows") || !strings.HasPrefix(got[0].Justification.Scope, "Only the fixtures") {
		t.Errorf("justification = %+v", got[0].Justification)
	}
}

func TestARecordKeepsAnalyseWithEverySectionAtOnce(t *testing.T) {
	data := "version: 1\nanalyse:\n  unanalysed:\n" + goodUnanalysed + "requiredChecks:\n  - context: QA\ntemplates:\n  - source: a.yml\n    copies: [b.yml]\nexceptions:\n" + goodException

	rec, err := record.Parse([]byte(data), everythingIsKnown)

	if err != nil || len(rec.Analyse.Unanalysed) != 1 || len(rec.RequiredChecks) != 1 || len(rec.Templates) != 1 || len(rec.Exceptions) != 1 {
		t.Errorf("record = %+v, err = %v", rec, err)
	}
	if err == nil && (rec.Templates[0].Line != 11 || rec.Exceptions[0].Line != 14 || rec.RequiredChecks[0].Line != 9) {
		t.Errorf("lines = %d, %d and %d", rec.RequiredChecks[0].Line, rec.Templates[0].Line, rec.Exceptions[0].Line)
	}
}

func TestParseRefusesAnUnanalysedEntryThatCannotMatchAFile(t *testing.T) {
	head := "version: 1\nanalyse:\n  unanalysed:\n"
	cases := map[string]struct{ data, mention string }{
		"no path":             {head + "    - justification:\n        hazard: x\n        scope: y\n", "path"},
		"a class":             {head + "    - path: 'a/[ab].yml'\n", "class"},
		"an absolute path":    {head + "    - path: /etc/x.yml\n", "absolute"},
		"a parent segment":    {head + "    - path: ../x.yml\n", ".."},
		"the same twice":      {head + "    - path: a/**\n    - path: a/**\n", "twice"},
		"an empty entry":      {head + "    -\n", "empty"},
		"an unknown field":    {head + "    - path: a/**\n      reason: because\n", "reason"},
		"unanalysed as a map": {"version: 1\nanalyse:\n  unanalysed:\n    path: a/**\n", "unmarshal"},
		"analyse as a list":   {"version: 1\nanalyse:\n  - a\n", "unmarshal"},
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

func TestParseReadsTheIncludedPatternsWithTheirLines(t *testing.T) {
	data := "version: 1\nanalyse:\n  include:\n    - templates/github-actions/**\n    # a comment\n    - 'docs/*.yml'\n  unanalysed:\n" + goodUnanalysed

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Analyse.Include
	if len(got) != 2 || got[0].Path != "templates/github-actions/**" || got[0].Line != 4 || got[1].Path != "docs/*.yml" || got[1].Line != 6 {
		t.Fatalf("include = %+v", got)
	}
	if len(rec.Analyse.Unanalysed) != 1 || rec.Analyse.Unanalysed[0].Line != 8 {
		t.Errorf("unanalysed = %+v; it should still be read beside include", rec.Analyse.Unanalysed)
	}
}

func TestParseReadsAnIncludeWrittenInFlowStyle(t *testing.T) {
	rec, err := record.Parse([]byte("version: 1\nanalyse:\n  include: [a/**, b/*.yml]\n"), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Analyse.Include
	if len(got) != 2 || got[0].Path != "a/**" || got[1].Path != "b/*.yml" || got[0].Line != 3 || got[1].Line != 3 {
		t.Errorf("include = %+v", got)
	}
}

func TestParseRefusesAnIncludedPatternThatCannotMatchAFile(t *testing.T) {
	head := "version: 1\nanalyse:\n  include:\n"
	cases := map[string]struct{ data, mention string }{
		"a class":           {head + "    - 'a/[ab].yml'\n", "class"},
		"an absolute path":  {head + "    - /etc/x.yml\n", "absolute"},
		"a parent segment":  {head + "    - ../x.yml\n", ".."},
		"the same twice":    {head + "    - a/**\n    - a/**\n", "twice"},
		"an empty entry":    {head + "    -\n", "empty"},
		"an empty string":   {head + "    - ''\n", "missing"},
		"a map entry":       {head + "    - path: a/**\n", "unmarshal"},
		"include as a map":  {"version: 1\nanalyse:\n  include:\n    path: a/**\n", "unmarshal"},
		"include as scalar": {"version: 1\nanalyse:\n  include: a/**\n", "unmarshal"},
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

func TestAnUnanalysedEntryWithNoJustificationIsReadAndJudgedByItsFaults(t *testing.T) {
	data := "version: 1\nanalyse:\n  unanalysed:\n    - path: a/**\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	faults := rec.Analyse.Unanalysed[0].Faults()
	if len(faults) != 2 || faults[0].Field != "hazard" || faults[1].Field != "scope" {
		t.Errorf("faults = %+v; a missing justification is for the justification check to report", faults)
	}
}

func TestUnanalysedFaultsFollowTheStandardOfTheExceptions(t *testing.T) {
	good := record.Unanalysed{Path: "a/**", Justification: record.Justification{
		Hazard: "Fixture workflows hold the very mistakes the rules are tested against",
		Scope:  "Only the fixtures directory, whose files are never run by any workflow",
	}}
	if faults := good.Faults(); len(faults) != 0 {
		t.Errorf("a stated justification has faults: %+v", faults)
	}

	generic := good
	generic.Justification.Hazard = "legacy"
	same := good
	same.Justification.Scope = good.Justification.Hazard
	short := good
	short.Justification.Scope = "tests only"
	for name, entry := range map[string]record.Unanalysed{"generic": generic, "same as the hazard": same, "short": short} {
		if len(entry.Faults()) != 1 {
			t.Errorf("%s: faults = %+v; want one", name, entry.Faults())
		}
	}
}
