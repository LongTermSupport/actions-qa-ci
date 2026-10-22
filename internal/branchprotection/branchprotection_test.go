package branchprotection_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/branchprotection"
)

// api is a fake GitHub: the response for each path, and the requests it saw.
type api struct {
	mu        sync.Mutex
	responses map[string]response
	seen      []string
	tokens    []string
	server    *httptest.Server
}

type response struct {
	status int
	body   string
}

func newAPI(t *testing.T, responses map[string]response) *api {
	t.Helper()

	a := &api{responses: responses}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.seen = append(a.seen, r.URL.EscapedPath())
		a.tokens = append(a.tokens, r.Header.Get("Authorization"))

		got, ok := a.responses[r.URL.EscapedPath()]
		if !ok {
			got = response{http.StatusNotFound, `{"message":"Not Found"}`}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(got.status)
		if _, err := w.Write([]byte(got.body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(a.server.Close)

	return a
}

func (a *api) client(t *testing.T) *branchprotection.Client {
	t.Helper()

	c, err := branchprotection.New(a.server.URL, "secret-token", nil)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

const (
	repoPath    = "/repos/owner/repo"
	classicPath = "/repos/owner/repo/branches/main/protection/required_status_checks"
	rulesPath   = "/repos/owner/repo/rules/branches/main"
)

func TestContextsAreTheUnionOfClassicProtectionAndRulesets(t *testing.T) {
	a := newAPI(t, map[string]response{
		repoPath:    {200, `{"default_branch":"main"}`},
		classicPath: {200, `{"strict":true,"contexts":["CI","Lint"],"checks":[{"context":"CI","app_id":15368},{"context":"Lint","app_id":-1}]}`},
		rulesPath:   {200, `[{"type":"deletion"},{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Lint"},{"context":"Scan","integration_id":1}]}}]`},
	})

	got, err := a.client(t).RequiredContexts("owner/repo", "default")

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"CI", "Lint", "Scan"}; !slices.Equal(got.Contexts, want) || got.Branch != "main" {
		t.Errorf("got %+v; want branch main with %q", got, want)
	}
}

func TestANamedBranchIsLookedUpAsItIsAndSlashesAreEscaped(t *testing.T) {
	a := newAPI(t, map[string]response{
		"/repos/owner/repo/branches/release%2F1/protection/required_status_checks": {200, `{"contexts":["CI"],"checks":[]}`},
		"/repos/owner/repo/rules/branches/release%2F1":                             {200, `[]`},
	})

	got, err := a.client(t).RequiredContexts("owner/repo", "release/1")

	if err != nil || !slices.Equal(got.Contexts, []string{"CI"}) || got.Branch != "release/1" {
		t.Errorf("got %+v, err %v; requests %v", got, err, a.seen)
	}
	if slices.Contains(a.seen, repoPath) {
		t.Errorf("the default branch was looked up for a named branch: %v", a.seen)
	}
}

func TestABranchWithNoProtectionRequiresNothing(t *testing.T) {
	a := newAPI(t, map[string]response{
		repoPath:    {200, `{"default_branch":"main"}`},
		classicPath: {404, `{"message":"Branch not protected","documentation_url":"x"}`},
		rulesPath:   {200, `[]`},
	})

	got, err := a.client(t).RequiredContexts("owner/repo", "default")

	if err != nil || len(got.Contexts) != 0 {
		t.Errorf("got %+v, err %v", got, err)
	}
}

func TestWhatCannotBeReadIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	ok := response{200, `{"default_branch":"main"}`}
	cases := map[string]map[string]response{
		"the repository is not found":    {},
		"the token is refused":           {repoPath: {401, `{"message":"Bad credentials"}`}},
		"protection needs more rights":   {repoPath: ok, classicPath: {403, `{"message":"Resource not accessible by integration"}`}, rulesPath: {200, `[]`}},
		"protection says 404 otherwise":  {repoPath: ok, classicPath: {404, `{"message":"Not Found"}`}, rulesPath: {200, `[]`}},
		"the rules cannot be read":       {repoPath: ok, classicPath: {200, `{"contexts":[]}`}, rulesPath: {500, `oops`}},
		"the answer is not JSON":         {repoPath: ok, classicPath: {200, `<html>`}, rulesPath: {200, `[]`}},
		"the default branch is missing":  {repoPath: {200, `{}`}},
		"too many rules to be sure":      {repoPath: ok, classicPath: {200, `{"contexts":[]}`}, rulesPath: {200, `[` + strings.Repeat(`{"type":"deletion"},`, 99) + `{"type":"deletion"}]`}},
		"a context that is not text":     {repoPath: ok, classicPath: {200, `{"contexts":[1]}`}, rulesPath: {200, `[]`}},
		"a rule check without a context": {repoPath: ok, classicPath: {200, `{"contexts":[]}`}, rulesPath: {200, `[{"type":"required_status_checks","parameters":{"required_status_checks":[{}]}}]`}},
	}

	for name, responses := range cases {
		a := newAPI(t, responses)

		got, err := a.client(t).RequiredContexts("owner/repo", "default")

		if !errors.Is(err, branchprotection.ErrUnreadable) {
			t.Errorf("%s: got %+v, err %v; want ErrUnreadable", name, got, err)
		}
	}
}

func TestTheTokenGoesOnlyInTheAuthorizationHeaderAndNeverIntoAnError(t *testing.T) {
	a := newAPI(t, map[string]response{repoPath: {401, `{"message":"Bad credentials secret-token"}`}})

	_, err := a.client(t).RequiredContexts("owner/repo", "default")

	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Errorf("err = %v; the token must not appear in it", err)
	}
	if len(a.tokens) == 0 || a.tokens[0] != "Bearer secret-token" {
		t.Errorf("authorization headers = %q", a.tokens)
	}
}

func TestARedirectIsNotFollowedSoTheTokenGoesNowhereElse(t *testing.T) {
	elsewhere := newAPI(t, map[string]response{})
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.server.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)
	client, err := branchprotection.New(redirecting.URL, "secret-token", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.RequiredContexts("owner/repo", "default")

	if !errors.Is(err, branchprotection.ErrUnreadable) {
		t.Errorf("err = %v", err)
	}
	if len(elsewhere.seen) != 0 {
		t.Errorf("the redirect was followed: %v", elsewhere.seen)
	}
}

func TestNewRefusesAnAddressThatWouldSendTheTokenInTheClear(t *testing.T) {
	for _, address := range []string{"http://api.example.com", "ftp://api.example.com", "api.example.com", "https://", "https://user:pass@api.example.com", ""} {
		if _, err := branchprotection.New(address, "t", nil); err == nil {
			t.Errorf("New(%q) accepted an address", address)
		}
	}
	for _, address := range []string{"https://api.github.com", "https://ghe.example.com/api/v3", "http://127.0.0.1:8080", "http://localhost:1"} {
		if _, err := branchprotection.New(address, "t", nil); err != nil {
			t.Errorf("New(%q): %v", address, err)
		}
	}
	if _, err := branchprotection.New("https://api.github.com", "", nil); err == nil {
		t.Error("New with no token accepted")
	}
}

func TestARepositoryThatIsNotOwnerSlashNameIsRefusedBeforeAnyRequest(t *testing.T) {
	a := newAPI(t, nil)

	for _, repo := range []string{"", "owner", "owner/", "/repo", "a/b/c", "owner/re po", "owner/../x", "owner/repo?x=1"} {
		if _, err := a.client(t).RequiredContexts(repo, "main"); err == nil {
			t.Errorf("repository %q accepted", repo)
		}
	}
	if len(a.seen) != 0 {
		t.Errorf("requests were made: %v", a.seen)
	}
}
