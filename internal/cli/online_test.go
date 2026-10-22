package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/cli"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const pullRequestWorkflow = "name: ci\non: pull_request\npermissions: {}\njobs:\n  build:\n    name: CI\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n"

// fakeGitHub is a GitHub that answers the three requests the comparison makes, and counts them.
type fakeGitHub struct {
	server   *httptest.Server
	requests atomic.Int32
}

func reply(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Error(err)
	}
}

func newFakeGitHub(t *testing.T, protected string) *fakeGitHub {
	t.Helper()

	fake := &fakeGitHub{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		switch r.URL.Path {
		case "/repos/owner/repo":
			reply(t, w, http.StatusOK, `{"default_branch":"main"}`)
		case "/repos/owner/repo/branches/main/protection/required_status_checks":
			reply(t, w, http.StatusOK, `{"contexts":[`+protected+`],"checks":[]}`)
		case "/repos/owner/repo/rules/branches/main":
			reply(t, w, http.StatusOK, `[]`)
		default:
			reply(t, w, http.StatusNotFound, `{"message":"Not Found"}`)
		}
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func onlineProject(t *testing.T, apiURL string) {
	t.Helper()

	t.Setenv("ACTIONS_QA_CI_TOOLS_DIR", toolsDirectory(t))
	t.Setenv("GITHUB_API_URL", apiURL)
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	repository(t, map[string]string{
		".github/workflows/ci.yml": pullRequestWorkflow,
		record.Path:                "version: 1\nrequiredChecks:\n  - context: CI\n",
	})
}

func TestOnlineComparesTheRecordWithTheProtectionAndIsCleanWhenTheyAgree(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)

	got := runWith(t, realEnv(t, nil), "check", "--online", "--format", "agent")

	if got.code != cli.ExitOK || got.stdout != "0 findings\n" || fake.requests.Load() == 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, requests %d", got.code, got.stdout, got.stderr, fake.requests.Load())
	}
}

func TestOnlineReportsAProtectionThatDisagreesWithTheRecord(t *testing.T) {
	fake := newFakeGitHub(t, `"CI","Legacy"`)
	onlineProject(t, fake.server.URL)

	got := runWith(t, realEnv(t, nil), "check", "--online", "--format", "agent")

	if got.code != cli.ExitFindings || !strings.Contains(got.stdout, record.Path+":3:1 actionsqaci.requiredChecks") || !strings.Contains(got.stdout, `"Legacy"`) {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestOfflineMakesNoRequestAndSaysTheProtectionWasNotCompared(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)

	got := runWith(t, realEnv(t, nil), "check", "--format", "agent")

	if got.code != cli.ExitOK || fake.requests.Load() != 0 {
		t.Errorf("exit %d, requests %d, stderr %q; nothing may reach the network without --online", got.code, fake.requests.Load(), got.stderr)
	}
	if !strings.Contains(got.stderr, "not compared with the branch protection") || !strings.Contains(got.stderr, "--online") {
		t.Errorf("stderr %q; a clean result must not be read as covering the protection", got.stderr)
	}
}

func TestOnlineNeedsATokenAndARepository(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)

	for _, variable := range []string{"GITHUB_TOKEN", "GITHUB_REPOSITORY"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv(variable, "")

			got := runWith(t, realEnv(t, nil), "check", "--online")

			if got.code != cli.ExitUsage || got.stdout != "" || !strings.Contains(got.stderr, variable) {
				t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
			}
		})
	}
	if fake.requests.Load() != 0 {
		t.Errorf("requests = %d", fake.requests.Load())
	}
}

func TestAnExceptionForTheOnlineComparisonIsStaleOnlyWhenTheComparisonRan(t *testing.T) {
	for name, placement := range map[string]string{
		"at the record": "    path: " + record.Path + "\n",
		"everywhere":    "",
	} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeGitHub(t, `"CI"`)
			onlineProject(t, fake.server.URL)
			accepting := "version: 1\nrequiredChecks:\n  - context: CI\nexceptions:\n" +
				"  - id: actionsqaci.requiredChecks\n" + placement +
				"    hazard: \"The protection may require a context the record has not yet been taught about\"\n" +
				"    scope: \"Only the record itself, and only until the protection and the record agree\"\n" +
				"    decision: \"Owner: https://github.com/LongTermSupport/php-qa-ci/issues/310\"\n"
			repository(t, map[string]string{
				".github/workflows/ci.yml": pullRequestWorkflow,
				record.Path:                accepting,
			})

			offline := runWith(t, realEnv(t, nil), "check", "--format", "agent")
			online := runWith(t, realEnv(t, nil), "check", "--online", "--format", "agent")

			if offline.code != cli.ExitOK || strings.Contains(offline.stdout, "staleException") {
				t.Errorf("offline: exit %d, stdout %q; the exception may be for a finding only the comparison makes, which was not made", offline.code, offline.stdout)
			}
			if online.code != cli.ExitFindings || !strings.Contains(online.stdout, "actionsqaci.staleException") {
				t.Errorf("online: exit %d, stdout %q, stderr %q; the comparison ran and found nothing for it to accept", online.code, online.stdout, online.stderr)
			}
		})
	}
}

func TestOnlineRefusesARepositoryThatIsNotOwnerSlashName(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)
	t.Setenv("GITHUB_REPOSITORY", "not a repository")

	got := runWith(t, realEnv(t, nil), "check", "--online")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "GITHUB_REPOSITORY") || fake.requests.Load() != 0 {
		t.Errorf("exit %d, stderr %q, requests %d; a wrong setting is a usage error, not a protection that could not be read", got.code, got.stderr, fake.requests.Load())
	}
}

func TestOnlineWithAProtectionItCannotReadIsNoVerdictAndNeverShowsTheToken(t *testing.T) {
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply(t, w, http.StatusForbidden, `{"message":"bad token secret-token"}`)
	}))
	t.Cleanup(denied.Close)
	onlineProject(t, denied.URL)

	got := runWith(t, realEnv(t, nil), "check", "--online", "--format", "agent")

	if got.code != cli.ExitNoVerdict || got.stdout != "" || !strings.Contains(got.stderr, "no verdict") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if strings.Contains(got.stderr, "secret-token") || strings.Contains(got.stdout, "secret-token") {
		t.Errorf("the token was printed: %q %q", got.stdout, got.stderr)
	}
}

func TestOnlineRefusesAnAddressThatWouldSendTheTokenInTheClear(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)
	t.Setenv("GITHUB_API_URL", "http://api.example.com")

	got := runWith(t, realEnv(t, nil), "check", "--online")

	if got.code != cli.ExitUsage || !strings.Contains(got.stderr, "GITHUB_API_URL") || fake.requests.Load() != 0 {
		t.Errorf("exit %d, stderr %q, requests %d", got.code, got.stderr, fake.requests.Load())
	}
}

func TestTheOnlineLaneCanBeChosenOnlyWithOnline(t *testing.T) {
	fake := newFakeGitHub(t, `"CI"`)
	onlineProject(t, fake.server.URL)

	without := runWith(t, realEnv(t, nil), "check", "-t", "requiredChecksOnline")
	with := runWith(t, realEnv(t, nil), "check", "--online", "-t", "requiredChecksOnline", "--format", "agent")

	if without.code != cli.ExitUsage || !strings.Contains(without.stderr, "--online") {
		t.Errorf("without: exit %d, stderr %q", without.code, without.stderr)
	}
	if with.code != cli.ExitOK || with.stdout != "0 findings\n" {
		t.Errorf("with: exit %d, stdout %q, stderr %q", with.code, with.stdout, with.stderr)
	}
}
