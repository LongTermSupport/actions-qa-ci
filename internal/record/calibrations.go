package record

import (
	"fmt"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Calibrations are what the project states about how the lanes run. Only what a lane acts on is
// accepted, so a calibration is never silently ignored.
type Calibrations struct {
	// Enable names the defences the lanes leave out unless the project asks for them. Asking for
	// one makes the standard stricter, so it needs no justification.
	Enable []Enabled `yaml:"enable"`
}

// calibrationsDocument is Calibrations as the decoder reads it, with entries that can be null.
type calibrationsDocument struct {
	Enable []*string `yaml:"enable"`
}

// Enabled is a defence the project asks the lanes to run.
type Enabled struct {
	// ID is the identifier of the defence.
	ID string

	// Line is where the entry is in the record file.
	Line int

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
}

// Enabled returns the identifiers of the defences the record asks for, in the order they are
// written. A nil record asks for none.
func (r *Record) Enabled() []string {
	if r == nil {
		return nil
	}

	ids := make([]string, 0, len(r.Calibrations.Enable))
	for _, entry := range r.Calibrations.Enable {
		ids = append(ids, entry.ID)
	}

	return ids
}

// enableFaults judges the enable section as a whole: each entry names a defence of this build,
// and none is named twice.
func enableFaults(entries []Enabled, known func(id string) bool) []string {
	var faults []string

	listed := map[string]bool{}
	for i, entry := range entries {
		if entry.null {
			continue // reported once, by Parse
		}
		where := fmt.Sprintf("enable entry %d (line %d)", i+1, entry.Line)

		if entry.ID == "" {
			faults = append(faults, where+": id is missing")

			continue
		}
		if _, err := finding.ClassifyIdentifier(entry.ID); err != nil {
			faults = append(faults, where+": "+err.Error())

			continue
		}
		if !known(entry.ID) {
			faults = append(faults, fmt.Sprintf("%s: id %q is unknown: no defence in this build has it", where, entry.ID))

			continue
		}
		if listed[entry.ID] {
			faults = append(faults, fmt.Sprintf("%s: id %q is listed twice", where, entry.ID))
		}
		listed[entry.ID] = true
	}

	return faults
}
