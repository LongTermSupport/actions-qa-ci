package firstparty

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/LongTermSupport/actions-qa-ci/internal/agentsummary"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// AgentSummaryCurrent is the identifier of the check that the generated region of the file an
// agent reads says what the build and the project record say.
const AgentSummaryCurrent = finding.ToolPrefix + ".agentSummaryCurrent"

// agentFiles are the files an agent reads that the summary is written to.
var agentFiles = []string{"CLAUDE.md", "AGENTS.md"}

// CheckAgentSummaryCurrent is the check on a project: for each of CLAUDE.md and AGENTS.md at the
// repository root that has a generated region, a finding at the region's begin marker when it
// differs from the one `rules --write-agent-summary` would write now, and a finding at the file
// when its markers do not form one region. A file without a region is the project's own and is
// left alone, as is one that is absent or is a link (the file behind a link is judged under its
// own name). The file at the path is the project record, which may be absent.
func CheckAgentSummaryCurrent(known func(id string) bool, current func() *registry.Registry) func(recordFile string) ([]finding.Finding, error) {
	return func(recordFile string) ([]finding.Finding, error) {
		reg := current()
		if reg == nil {
			return nil, errors.New("the check has no registry of rules to compare the summary with")
		}

		rec, err := record.Load(recordFile, known)
		if errors.Is(err, fs.ErrNotExist) {
			rec, err = nil, nil
		}
		if err != nil {
			return nil, err
		}

		entries := listing.Active(reg)
		listing.Narrow(reg, entries, rec)
		want, err := agentsummary.Region(entries)
		if err != nil {
			return nil, err
		}

		root, err := workingRoot()
		if err != nil {
			return nil, err
		}

		var found []finding.Finding
		for _, name := range agentFiles {
			data, problem, err := readTemplateFile(root, name)
			switch {
			case err != nil:
				return nil, err
			case problem == "is missing", problem == "is not a regular file: it is a symbolic link":
				continue
			case problem != "":
				return nil, fmt.Errorf("%w %s: it %s", check.ErrNotAnalysable, name, problem)
			}

			f, err := judgeRegion(name, data, want)
			if err != nil {
				return nil, err
			}
			found = append(found, f...)
		}

		return found, nil
	}
}

// judgeRegion compares the region of one file with the one it should have.
func judgeRegion(name string, data []byte, want string) ([]finding.Finding, error) {
	line, hasRegion, err := agentsummary.BeginLine(data)
	if err != nil {
		f, ferr := newFinding(AgentSummaryCurrent, name, 1, 1, fmt.Sprintf("the generated region of %s cannot be read: %v; fix the markers, they have to form exactly one region", name, err))

		return []finding.Finding{f}, ferr
	}
	if !hasRegion {
		return nil, nil
	}

	got, extracted, err := agentsummary.Extract(data)
	if err != nil {
		return nil, err
	}
	if !extracted {
		return nil, fmt.Errorf("the generated region of %s was found and then not found", name)
	}
	if got == want {
		return nil, nil
	}

	f, err := newFinding(AgentSummaryCurrent, name, line, 1,
		fmt.Sprintf("the generated region of %s is out of date: run `actions-qa rules --write-agent-summary %s` and commit the result", name, name))

	return []finding.Finding{f}, err
}

// AgentSummaryRule is the registry entry of the check. current gives the registry the check
// compares the summary with, which is the one this rule is in.
func AgentSummaryRule(known func(id string) bool, current func() *registry.Registry) registry.Rule {
	return registry.Rule{
		ID:      AgentSummaryCurrent,
		Summary: "The generated region of CLAUDE.md or AGENTS.md lists the defences this build has and what the project record narrows",
		Check:   CheckAgentSummaryCurrent(known, current),
	}
}
