package check

import (
	"fmt"
	"slices"
)

// Offers says whether some lane holds back the defence with this identifier, so that the project
// record can ask for it.
func Offers(lanes []Lane, id string) bool {
	return slices.ContainsFunc(lanes, func(lane Lane) bool { return slices.Contains(lane.OptIn, id) })
}

// Enabling returns the lanes with each lane that holds defences back running the ones the
// project record asks for. A defence none of the given lanes holds back belongs to a lane that is
// not being run, and is left. The lanes given are left as they were.
func Enabling(lanes []Lane, ids []string) ([]Lane, error) {
	enabled := slices.Clone(lanes)
	for i, lane := range lanes {
		var wanted []string
		for _, id := range ids {
			if slices.Contains(lane.OptIn, id) && !slices.Contains(wanted, id) {
				wanted = append(wanted, id)
			}
		}
		if len(wanted) == 0 {
			continue
		}
		if lane.Enable == nil {
			return nil, fmt.Errorf("lane %s holds defences back and cannot enable them", lane.Name)
		}

		run, err := lane.Enable(wanted)
		if err != nil {
			return nil, fmt.Errorf("lane %s cannot enable what the project record asks for: %w", lane.Name, err)
		}
		enabled[i].Run = run
	}

	return enabled, nil
}
