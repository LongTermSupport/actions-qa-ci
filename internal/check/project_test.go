package check_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

const subject = ".github/actions-qa-ci.yml"

// projectLane is a lane about the project: it selects the record, and has it as its subject so
// that it runs even where the record does not exist.
func projectLane(ran *[]string) check.Lane {
	return check.Lane{
		Name:    "project",
		Subject: subject,
		Selects: func(file string) bool { return file == subject },
		Run: func(file string) ([]finding.Finding, error) {
			*ran = append(*ran, file)

			return nil, nil
		},
	}
}

func TestAProjectLaneRunsOnceOnItsSubjectEvenWhereTheSubjectIsNotAFile(t *testing.T) {
	var ran []string

	_, err := check.RunProject([]check.Lane{projectLane(&ran)}, []string{"a.yml"})

	if err != nil || !slices.Equal(ran, []string{subject}) {
		t.Errorf("ran %q, error %v; want one run, on the subject", ran, err)
	}
}

func TestAProjectLaneDoesNotRunTwiceWhereTheSubjectIsAlsoAFile(t *testing.T) {
	var ran []string

	_, err := check.RunProject([]check.Lane{projectLane(&ran)}, []string{"a.yml", subject})

	if err != nil || !slices.Equal(ran, []string{subject}) {
		t.Errorf("ran %q, error %v; want one run", ran, err)
	}
}

func TestAnOrdinaryRunLeavesAProjectLaneToTheFilesItIsGiven(t *testing.T) {
	var ran []string

	if _, err := check.Run([]check.Lane{projectLane(&ran)}, []string{"a.yml"}); err != nil || len(ran) != 0 {
		t.Errorf("ran %q, error %v; a run over named files is not a run over the project", ran, err)
	}
	if _, err := check.Run([]check.Lane{projectLane(&ran)}, []string{subject}); err != nil || !slices.Equal(ran, []string{subject}) {
		t.Errorf("ran %q, error %v; a named subject is analysed like any file", ran, err)
	}
}

func TestAProjectLaneThatFailsStopsTheRunNamingTheSubject(t *testing.T) {
	boom := errors.New("boom")
	lane := check.Lane{Name: "project", Subject: subject, Selects: func(string) bool { return false },
		Run: func(string) ([]finding.Finding, error) { return nil, boom }}

	found, err := check.RunProject([]check.Lane{lane}, nil)

	var failed *check.LaneError
	if !errors.As(err, &failed) || failed.Path != subject || !errors.Is(err, boom) || found != nil {
		t.Errorf("findings %v, error %v", found, err)
	}
}
