package check_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const (
	optInA = "actionsqaci.fake.a"
	optInB = "actionsqaci.fake.b"
)

// offering is a lane that holds back two defences, and reports as findings the ones it was
// asked to run.
func offering(name string, record *[][]string) check.Lane {
	return check.Lane{
		Name:    name,
		Selects: func(string) bool { return true },
		Run:     func(string) ([]finding.Finding, error) { return nil, nil },
		OptIn:   []string{optInA, optInB},
		Enable: func(ids []string) (func(string) ([]finding.Finding, error), error) {
			*record = append(*record, slices.Clone(ids))

			return func(string) ([]finding.Finding, error) {
				return []finding.Finding{{ID: strings.Join(ids, "+")}}, nil
			}, nil
		},
	}
}

func plain(name string) check.Lane {
	return check.Lane{
		Name:    name,
		Selects: func(string) bool { return true },
		Run:     func(string) ([]finding.Finding, error) { return []finding.Finding{{ID: name}}, nil },
	}
}

func TestEnablingGivesTheLaneThatHoldsADefenceBackTheDefencesAskedFor(t *testing.T) {
	var asked [][]string
	lanes := []check.Lane{plain("p"), offering("o", &asked)}

	enabled, err := check.Enabling(lanes, []string{optInB, optInA})

	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !slices.Equal(asked[0], []string{optInB, optInA}) {
		t.Errorf("the lane was asked for %q", asked)
	}
	got, err := enabled[1].Run("x")
	if err != nil || len(got) != 1 || got[0].ID != optInB+"+"+optInA {
		t.Errorf("the enabled lane ran %+v, %v", got, err)
	}
	if again, err := enabled[0].Run("x"); err != nil || len(again) != 1 || again[0].ID != "p" {
		t.Errorf("a lane that holds nothing back changed: %+v, %v", again, err)
	}
	if enabled[1].Name != "o" || !slices.Equal(enabled[1].OptIn, lanes[1].OptIn) {
		t.Errorf("the enabled lane lost what it was: %+v", enabled[1])
	}
}

func TestEnablingNothingLeavesTheLanesAsTheyWere(t *testing.T) {
	var asked [][]string
	lanes := []check.Lane{offering("o", &asked)}

	enabled, err := check.Enabling(lanes, nil)

	if err != nil || len(asked) != 0 {
		t.Fatalf("err = %v, asked = %q", err, asked)
	}
	if got, err := enabled[0].Run("x"); err != nil || len(got) != 0 {
		t.Errorf("the lane ran %+v, %v", got, err)
	}
}

func TestEnablingLeavesTheLanesItWasGivenAsTheyWere(t *testing.T) {
	var asked [][]string
	lanes := []check.Lane{offering("o", &asked)}

	if _, err := check.Enabling(lanes, []string{optInA}); err != nil {
		t.Fatal(err)
	}

	if got, err := lanes[0].Run("x"); err != nil || len(got) != 0 {
		t.Errorf("the lane given was changed: it ran %+v, %v", got, err)
	}
}

func TestADefenceNoGivenLaneHoldsBackIsLeftForTheLaneThatDoes(t *testing.T) {
	// A run of some lanes only: a defence of a lane that is not run has nothing to enable.
	enabled, err := check.Enabling([]check.Lane{plain("p")}, []string{optInA})

	if err != nil || len(enabled) != 1 {
		t.Errorf("got %+v, %v", enabled, err)
	}
}

func TestADefenceIsEnabledOnlyOnceWhateverTheOrder(t *testing.T) {
	var asked [][]string
	lanes := []check.Lane{offering("o", &asked)}

	if _, err := check.Enabling(lanes, []string{optInA, optInA}); err != nil {
		t.Fatal(err)
	}

	if len(asked) != 1 || !slices.Equal(asked[0], []string{optInA}) {
		t.Errorf("the lane was asked for %q", asked)
	}
}

func TestALaneThatCannotEnableWhatItOffersIsRefused(t *testing.T) {
	lane := plain("p")
	lane.OptIn = []string{optInA}

	_, err := check.Enabling([]check.Lane{lane}, []string{optInA})

	if err == nil || !strings.Contains(err.Error(), "p") {
		t.Errorf("err = %v; want the lane named", err)
	}
	if _, err := check.Run([]check.Lane{lane}, []string{"x"}); err == nil {
		t.Error("a run with such a lane should be refused")
	}
}

func TestAnIsolatedLaneCannotHoldDefencesBack(t *testing.T) {
	// The child process an isolated lane runs in is started without the project record, so what
	// the record enabled would silently not happen.
	var asked [][]string
	lane := offering("o", &asked)
	lane.Isolated = true

	_, err := check.Run([]check.Lane{lane}, []string{"x"})

	if err == nil || !strings.Contains(err.Error(), "o") {
		t.Errorf("err = %v; want the lane refused and named", err)
	}
}

func TestALaneFailingToEnableIsAnError(t *testing.T) {
	boom := errors.New("cannot enable")
	lane := plain("p")
	lane.OptIn = []string{optInA}
	lane.Enable = func([]string) (func(string) ([]finding.Finding, error), error) { return nil, boom }

	_, err := check.Enabling([]check.Lane{lane}, []string{optInA})

	if !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestOffersSaysWhetherALaneHoldsADefenceBack(t *testing.T) {
	var asked [][]string
	lanes := []check.Lane{plain("p"), offering("o", &asked)}

	if !check.Offers(lanes, optInA) || !check.Offers(lanes, optInB) {
		t.Error("a defence a lane holds back is offered")
	}
	if check.Offers(lanes, "actionsqaci.fake.c") || check.Offers(lanes[:1], optInA) {
		t.Error("a defence no lane holds back is not offered")
	}
}
