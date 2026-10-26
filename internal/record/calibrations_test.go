package record_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestParseReadsTheEnabledDefencesWithTheirLines(t *testing.T) {
	data := "version: 1\ncalibrations:\n  enable:\n    - actionsqaci.zizmor.concurrency-limits\n    # a comment\n    - actionsqaci.zizmor.undocumented-permissions\nexceptions:\n" + goodException

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Calibrations.Enable
	if len(got) != 2 || got[0].ID != "actionsqaci.zizmor.concurrency-limits" || got[0].Line != 4 || got[1].ID != "actionsqaci.zizmor.undocumented-permissions" || got[1].Line != 6 {
		t.Fatalf("enable = %+v", got)
	}
	if len(rec.Exceptions) != 1 || rec.Exceptions[0].Line != 8 {
		t.Errorf("exceptions = %+v; they should still be read beside the calibrations", rec.Exceptions)
	}
}

func TestEnabledGivesTheIdentifiersInTheOrderTheyAreWritten(t *testing.T) {
	data := "version: 1\ncalibrations:\n  enable: [actionsqaci.zizmor.self-hosted-runner, actionsqaci.zizmor.concurrency-limits]\n"

	rec, err := record.Parse([]byte(data), everythingIsKnown)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.Enabled()
	if len(got) != 2 || got[0] != "actionsqaci.zizmor.self-hosted-runner" || got[1] != "actionsqaci.zizmor.concurrency-limits" {
		t.Errorf("Enabled() = %q", got)
	}
}

func TestARecordWithNoCalibrationsEnablesNothing(t *testing.T) {
	for name, data := range map[string]string{
		"none":          "version: 1\n",
		"an empty map":  "version: 1\ncalibrations: {}\n",
		"an empty list": "version: 1\ncalibrations:\n  enable: []\n",
	} {
		rec, err := record.Parse([]byte(data), everythingIsKnown)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}
		if len(rec.Enabled()) != 0 {
			t.Errorf("%s: Enabled() = %q", name, rec.Enabled())
		}
	}
}

func TestANilRecordEnablesNothing(t *testing.T) {
	var rec *record.Record

	if got := rec.Enabled(); len(got) != 0 {
		t.Errorf("Enabled() = %q", got)
	}
}

func TestParseRefusesAnEnableEntryThatCannotNameADefence(t *testing.T) {
	head := "version: 1\ncalibrations:\n  enable:\n"
	notKnown := func(id string) bool { return id != "actionsqaci.nonsense" }
	cases := map[string]struct{ data, mention string }{
		"an empty entry":          {head + "    -\n", "empty"},
		"a null entry":            {head + "    - null\n", "empty"},
		"an empty identifier":     {head + "    - \"\"\n", "missing"},
		"not in the scheme":       {head + "    - Not An Identifier\n", "scheme"},
		"an identifier no rule":   {head + "    - actionsqaci.nonsense\n", "unknown"},
		"the same twice":          {head + "    - actionsqaci.zizmor.concurrency-limits\n    - actionsqaci.zizmor.concurrency-limits\n", "twice"},
		"a number":                {head + "    - 7\n", "scheme"},
		"enable as a map":         {"version: 1\ncalibrations:\n  enable:\n    id: actionsqaci.zizmor.concurrency-limits\n", "unmarshal"},
		"enable as text":          {"version: 1\ncalibrations:\n  enable: actionsqaci.zizmor.concurrency-limits\n", "unmarshal"},
		"calibrations as a list":  {"version: 1\ncalibrations:\n  - a\n", "unmarshal"},
		"a calibration not built": {"version: 1\ncalibrations:\n  shellcheckSeverity: error\n", "shellcheckSeverity"},
		"an unbuilt one beside":   {head + "    - actionsqaci.zizmor.concurrency-limits\n  selfHostedRunnerLabels: [gpu]\n", "selfHostedRunnerLabels"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), notKnown)

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

func TestAnEnableFaultNamesTheEntryAndItsLine(t *testing.T) {
	data := "version: 1\ncalibrations:\n  enable:\n    - actionsqaci.zizmor.concurrency-limits\n    - Not An Identifier\n"

	_, err := record.Parse([]byte(data), everythingIsKnown)

	var invalid *record.InvalidError
	if !errors.As(err, &invalid) || len(invalid.Reasons) != 1 || !strings.Contains(invalid.Reasons[0], "enable entry 2 (line 5)") {
		t.Errorf("error = %v; want it to name enable entry 2 on line 5", err)
	}
}
