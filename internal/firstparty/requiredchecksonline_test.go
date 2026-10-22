package firstparty_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/branchprotection"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
)

// protections answers, for each branch the record names, what the repository requires there.
type protections map[string]branchprotection.Result

func (p protections) read(branch string) (branchprotection.Result, error) {
	if got, ok := p[branch]; ok {
		return got, nil
	}

	return branchprotection.Result{}, branchprotection.ErrUnreadable
}

func onlineRecord(checks ...string) string {
	text := "version: 1\nrequiredChecks:\n"
	for _, c := range checks {
		text += "  - " + c + "\n"
	}

	return text
}

func compare(t *testing.T, recordText string, p protections) ([]finding.Finding, error) {
	t.Helper()
	inTree(t, map[string]string{recordPath: recordText})

	return firstparty.CheckRequiredChecksOnline(everythingIsKnown, p.read)(recordPath)
}

func TestAProtectionThatMatchesTheRecordIsClean(t *testing.T) {
	got, err := compare(t, onlineRecord("{context: CI}", "{context: Lint, branch: default}"),
		protections{"default": {Branch: "main", Contexts: []string{"CI", "Lint"}}})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestARequiredContextTheProtectionLacksIsAFindingAtItsEntry(t *testing.T) {
	got, err := compare(t, onlineRecord("{context: CI}", "{context: Scan}"),
		protections{"default": {Branch: "main", Contexts: []string{"CI"}}})

	if err != nil || len(got) != 1 {
		t.Fatalf("findings %+v, error %v; want one", got, err)
	}
	f := got[0]
	if f.ID != firstparty.RequiredChecks || f.Path != recordPath || f.Line != 4 || f.Severity != finding.SeverityError {
		t.Errorf("finding %+v; want an error at line 4 of the record", f)
	}
	for _, want := range []string{`"Scan"`, "main", "does not require"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q should mention %q", f.Message, want)
		}
	}
	if err := f.Validate(); err != nil {
		t.Error(err)
	}
}

func TestAContextTheProtectionRequiresAndTheRecordDoesNotListIsAFinding(t *testing.T) {
	got, err := compare(t, onlineRecord("{context: CI}"),
		protections{"default": {Branch: "main", Contexts: []string{"CI", "Legacy"}}})

	if err != nil || len(got) != 1 {
		t.Fatalf("findings %+v, error %v; want one", got, err)
	}
	if got[0].Path != recordPath || got[0].Line != 3 || !strings.Contains(got[0].Message, `"Legacy"`) || !strings.Contains(got[0].Message, "does not list") {
		t.Errorf("finding %+v; want one at the first entry for the branch, naming Legacy", got[0])
	}
}

func TestEachBranchTheRecordNamesIsComparedWithItsOwnProtection(t *testing.T) {
	got, err := compare(t, onlineRecord("{context: CI}", "{context: Release, branch: release}"),
		protections{
			"default": {Branch: "main", Contexts: []string{"CI"}},
			"release": {Branch: "release", Contexts: []string{"CI"}},
		})

	if err != nil || len(got) != 2 {
		t.Fatalf("findings %+v, error %v; want a missing Release and an unlisted CI on release", got, err)
	}
	if !strings.Contains(got[0].Message, `"CI"`) && !strings.Contains(got[1].Message, `"CI"`) {
		t.Errorf("findings %+v", got)
	}
}

func TestTwoSpellingsOfTheSameBranchAreOneBranch(t *testing.T) {
	got, err := compare(t, onlineRecord("{context: CI, branch: default}", "{context: Lint, branch: main}"),
		protections{
			"default": {Branch: "main", Contexts: []string{"CI", "Lint"}},
			"main":    {Branch: "main", Contexts: []string{"CI", "Lint"}},
		})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; the default branch and its name are one protection, and both entries are listed", got, err)
	}
}

func TestAProtectionIsReadOncePerBranchWhateverTheSpelling(t *testing.T) {
	reads := 0
	read := func(branch string) (branchprotection.Result, error) {
		reads++

		return branchprotection.Result{Branch: "main", Contexts: []string{"CI"}}, nil
	}
	inTree(t, map[string]string{recordPath: onlineRecord("{context: CI}", "{context: CI, branch: main}")})

	if _, err := firstparty.CheckRequiredChecksOnline(everythingIsKnown, read)(recordPath); err != nil {
		t.Fatal(err)
	}

	if reads != 2 {
		t.Errorf("reads = %d; one for each spelling, none repeated", reads)
	}
}

func TestAProjectWithNoRequiredChecksHasNothingToCompare(t *testing.T) {
	got, err := compare(t, "version: 1\n", protections{})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; the protection must not even be read", got, err)
	}
}

func TestAProtectionThatCannotBeReadIsNoVerdictNamingTheBranch(t *testing.T) {
	_, err := compare(t, onlineRecord("{context: CI, branch: release}"), protections{})

	if !errors.Is(err, check.ErrNotAnalysable) || !errors.Is(err, branchprotection.ErrUnreadable) || !strings.Contains(err.Error(), "release") {
		t.Errorf("error = %v; a comparison that could not be made is not a clean result", err)
	}
}
