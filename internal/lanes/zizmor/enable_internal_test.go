package zizmor

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// personaReports is a stand-in for zizmor that reports what a persona would: the regular one
// finds artipacked, the auditor one also finds two audits the regular one leaves out. Each run
// adds its arguments to the record.
func personaReports(t *testing.T) (binary, record string) {
	t.Helper()

	regular := string(encode(t, entry("artipacked", "d", "Medium", false, location(reportedPath, "Primary", "a", 0, 0, 0, 1))))
	auditor := string(encode(t,
		entry("artipacked", "d", "Medium", false, location(reportedPath, "Primary", "a", 0, 0, 0, 1)),
		entry("concurrency-limits", "d", "Low", false, location(reportedPath, "Primary", "b", 1, 0, 1, 1)),
		entry("undocumented-permissions", "d", "Low", false, location(reportedPath, "Primary", "c", 2, 0, 2, 1)),
	))

	return standIn(t, `for a in "$@"; do echo "$a" >> "$record"; done
case "$*" in
*--persona=auditor*) echo '`+auditor+`' ;;
*) echo '`+regular+`' ;;
esac`)
}

// personaBinary is personaReports with no record kept.
func personaBinary(t *testing.T) string {
	t.Helper()

	binary, record := personaReports(t)
	if record == "" {
		t.Fatal("a stand-in has no record path")
	}

	return binary
}

func upstreamIDs(found []finding.Finding) []string {
	ids := make([]string, 0, len(found))
	for _, f := range found {
		ids = append(ids, f.UpstreamID)
	}
	slices.Sort(ids)

	return ids
}

func TestWithNothingEnabledOnlyTheRegularPersonaRuns(t *testing.T) {
	binary, record := personaReports(t)

	found, err := lintEnabling(binary, reportedPath, nil, testLimits)

	if err != nil {
		t.Fatal(err)
	}
	if got := upstreamIDs(found); !slices.Equal(got, []string{"artipacked"}) {
		t.Errorf("findings = %q", got)
	}
	written, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "--persona=auditor") {
		t.Errorf("the auditor persona ran with nothing enabled:\n%s", written)
	}
}

func TestAnEnabledAuditIsReportedAndNoOtherOfTheWiderPersona(t *testing.T) {
	found, err := lintEnabling(personaBinary(t), reportedPath, []string{"concurrency-limits"}, testLimits)

	if err != nil {
		t.Fatal(err)
	}
	if got := upstreamIDs(found); !slices.Equal(got, []string{"artipacked", "concurrency-limits"}) {
		t.Errorf("findings = %q; want the regular persona's and only the enabled audit of the wider one", got)
	}
}

func TestARegularFindingIsReportedOnceWhateverIsEnabled(t *testing.T) {
	found, err := lintEnabling(personaBinary(t), reportedPath, []string{"concurrency-limits", "undocumented-permissions"}, testLimits)

	if err != nil {
		t.Fatal(err)
	}
	if got := upstreamIDs(found); !slices.Equal(got, []string{"artipacked", "concurrency-limits", "undocumented-permissions"}) {
		t.Errorf("findings = %q", got)
	}
}

func TestEnablingAnAuditRunsBothPersonasWithNothingOfTheProjectsApplied(t *testing.T) {
	binary, record := personaReports(t)

	if _, err := lintEnabling(binary, reportedPath, []string{"concurrency-limits"}, testLimits); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--persona=regular", "--persona=auditor"} {
		if got := strings.Count(string(written), want); got != 1 {
			t.Errorf("%s was given to %d runs, want 1:\n%s", want, got, written)
		}
	}
	for _, want := range []string{"--no-config", "--no-ignores", "--offline"} {
		if got := strings.Count(string(written), want); got != 2 {
			t.Errorf("%s was given to %d runs, want both:\n%s", want, got, written)
		}
	}
}

func TestEitherRunFailingIsAFailureAndNeverAPartialList(t *testing.T) {
	for name, persona := range map[string]string{"the regular run": "regular", "the wider run": "auditor"} {
		binary := standInBinary(t, `case "$*" in
*--persona=`+persona+`*) echo "error: cannot read the file" >&2; exit 2 ;;
*) echo '[]' ;;
esac`)

		found, err := lintEnabling(binary, reportedPath, []string{"concurrency-limits"}, testLimits)

		if err == nil || !strings.Contains(err.Error(), "cannot read the file") || found != nil {
			t.Errorf("%s failing: got %+v, %v", name, found, err)
		}
	}
}

func TestOnlyAnAuditTheRegularPersonaLeavesOutCanBeEnabled(t *testing.T) {
	cases := map[string]struct{ id, reason string }{
		"an audit that already runs":    {ID("artipacked"), "already runs it"},
		"an audit this build holds":     {ID("impostor-commit"), "does not run it"},
		"an audit zizmor does not have": {ID("nonsense"), "not an audit of zizmor's"},
		"another tool's rule":           {"actionsqaci.actionlint.glob", "not an audit of zizmor's"},
		"something that is no rule":     {"nonsense", "not an audit of zizmor's"},
	}

	for name, c := range cases {
		run, err := Enabling([]string{c.id})

		if err == nil || run != nil || !strings.Contains(err.Error(), c.id) || !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%s (%s): got %v, %v; want it refused because %q", name, c.id, run != nil, err, c.reason)
		}
	}
}

func TestEveryAuditTheRegularPersonaLeavesOutAndThisBuildRunsCanBeEnabled(t *testing.T) {
	var ids []string
	for _, a := range Audits() {
		if a.WiderPersonaOnly && a.Held == "" {
			ids = append(ids, ID(a.Name))
		}
	}
	if len(ids) == 0 {
		t.Fatal("no audit can be enabled")
	}

	run, err := Enabling(ids)

	if err != nil || run == nil {
		t.Errorf("got %v, %v", run != nil, err)
	}
}

func TestEnablingNothingIsNotAnError(t *testing.T) {
	run, err := Enabling(nil)

	if err != nil || run == nil {
		t.Errorf("got %v, %v", run != nil, err)
	}
}

func TestAnAuditTheRegularPersonaLeavesOutIsReportedByTheLaneOnlyOnceEnabled(t *testing.T) {
	const name = "concurrency-limits"
	target := inProject(t, fixtures[name].red)

	without, err := Lint(target)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := Enabling([]string{ID(name)})
	if err != nil {
		t.Fatal(err)
	}
	with, err := enabled(target)
	if err != nil {
		t.Fatal(err)
	}

	if slices.Contains(upstreamIDs(without), name) {
		t.Errorf("the lane reported %s without being asked: %q", name, upstreamIDs(without))
	}
	if !slices.Contains(upstreamIDs(with), name) {
		t.Errorf("the lane did not report %s once enabled: %q", name, upstreamIDs(with))
	}
	for _, id := range upstreamIDs(with) {
		if id != name && !slices.Contains(upstreamIDs(without), id) {
			t.Errorf("enabling %s also brought %s", name, id)
		}
	}
}

func TestAReportThatIsNotAListFailsTheEnablingRunLikeAnyOther(t *testing.T) {
	binary := standInBinary(t, `echo 'not a report'`)

	found, err := lintEnabling(binary, reportedPath, []string{"concurrency-limits"}, testLimits)

	if err == nil || found != nil {
		t.Errorf("got %+v, %v", found, err)
	}
}
