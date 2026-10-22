package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
)

// isolate is the lanes with each one that asks to be isolated made to run in a process of its
// own. With no isolation, the lanes are returned as they are.
func isolate(isolation *Isolation, lanes []check.Lane) []check.Lane {
	if isolation == nil {
		return lanes
	}

	command := func(ctx context.Context, lane, path string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, isolation.Executable, "lane", lane, path)
		cmd.Env = append(os.Environ(), isolation.Environment...)

		return cmd
	}

	isolated := make([]check.Lane, len(lanes))
	for i, lane := range lanes {
		if lane.Isolated {
			lane = check.Isolated(lane, command, isolation.Limits)
		}
		isolated[i] = lane
	}

	return isolated
}

// laneCommand is what a lane process does: run one lane on one file and write the findings as
// JSON to standard output. A lane that fails says why on standard error and exits with
// check.ChildFault.
func laneCommand(env Env, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		return usageError(stderr, "lane takes a lane name and one path")
	}

	var chosen *check.Lane
	for i := range env.Lanes {
		if env.Lanes[i].Name == args[0] {
			chosen = &env.Lanes[i]
		}
	}
	if chosen == nil {
		return usageError(stderr, fmt.Sprintf("there is no lane %q", args[0]))
	}

	path, problem, err := repositoryPath(args[1])
	if err != nil {
		return fault(stderr, err)
	}
	if problem != "" {
		return refusal(stderr, problem)
	}
	if !chosen.Selects(path) {
		return refusal(stderr, fmt.Sprintf("lane %s does not analyse %s", chosen.Name, args[1]))
	}

	return check.Serve(*chosen, path, stdout, stderr)
}
