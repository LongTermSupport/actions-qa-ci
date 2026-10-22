package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/LongTermSupport/actions-qa-ci/internal/branchprotection"
	"github.com/LongTermSupport/actions-qa-ci/internal/check"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

// onlineLaneName is the lane that compares the record's required checks with the protection the
// repository has. It exists only in a run that asked to go online.
const onlineLaneName = "requiredChecksOnline"

const defaultAPIAddress = "https://api.github.com"

// onlineLane makes the lane that reads the branch protection from the GitHub API, using the
// token in GITHUB_TOKEN (or GH_TOKEN), the repository in GITHUB_REPOSITORY and the API at
// GITHUB_API_URL (GitHub's own by default). The code is ExitOK when the lane could be made.
func onlineLane(env Env, stderr io.Writer) (check.Lane, int) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return check.Lane{}, refusal(stderr, "--online needs a GitHub token in GITHUB_TOKEN (or GH_TOKEN), with permission to read branch protection")
	}
	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		return check.Lane{}, refusal(stderr, "--online needs the repository in GITHUB_REPOSITORY, as owner/name")
	}
	if err := branchprotection.ValidRepository(repository); err != nil {
		return check.Lane{}, refusal(stderr, "GITHUB_REPOSITORY is wrong: "+err.Error())
	}
	address := os.Getenv("GITHUB_API_URL")
	if address == "" {
		address = defaultAPIAddress
	}

	client, err := branchprotection.New(address, token, nil)
	if err != nil {
		return check.Lane{}, refusal(stderr, fmt.Sprintf("GITHUB_API_URL or the token cannot be used: %v", err))
	}

	known := func(id string) bool {
		_, ok := env.Registry.Lookup(id)

		return ok
	}
	read := func(branch string) (branchprotection.Result, error) {
		return client.RequiredContexts(repository, branch)
	}

	return check.Lane{
		Name:    onlineLaneName,
		Selects: func(path string) bool { return path == record.Path },
		Run:     firstparty.CheckRequiredChecksOnline(known, read),
	}, ExitOK
}

// judgeable is which exceptions of the record a run can call stale. An exception for the
// required checks at the record may accept a finding that only the online comparison makes, so
// a run that did not go online cannot say it matched nothing; it is left as matched. (Such an
// exception is called stale only by a run that did compare.)
func judgeable(rec *record.Record, matched []bool, online bool) []bool {
	if online {
		return matched
	}

	adjusted := make([]bool, len(rec.Exceptions))
	copy(adjusted, matched)
	for i, e := range rec.Exceptions {
		if e.ID == firstparty.RequiredChecks && e.Path == record.Path {
			adjusted[i] = true
		}
	}

	return adjusted
}

// noteOffline says, on the diagnostic stream, that the record's required checks were not
// compared with the repository's branch protection, so a clean result is not read as covering it.
func noteOffline(rec *record.Record, stderr io.Writer) error {
	if rec == nil || len(rec.RequiredChecks) == 0 {
		return nil
	}

	_, err := fmt.Fprintln(stderr, "actions-qa: the required checks in the record were not compared with the branch protection (offline); pass --online, with GITHUB_TOKEN and GITHUB_REPOSITORY, to compare")

	return err
}
