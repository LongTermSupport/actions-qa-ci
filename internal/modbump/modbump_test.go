package modbump_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/modbump"
)

const module = "github.com/rhysd/actionlint"

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

const goMod = `module example.com/project

go 1.25

require (
	github.com/rhysd/actionlint v1.7.12
	golang.org/x/sync v0.1.0 // indirect
)
`

// proxy is a fake of the Go module proxy that answers every request with one response and
// records the paths it was asked for.
type proxy struct {
	server *httptest.Server
	paths  []string
}

func newProxy(t *testing.T, status int, body string) *proxy {
	t.Helper()

	p := &proxy{}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.paths = append(p.paths, r.URL.Path)
		if status == http.StatusFound {
			http.Redirect(w, r, "https://example.invalid/elsewhere", status)

			return
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(p.server.Close)

	return p
}

func latest(version string, age time.Duration) string {
	return `{"Version":"` + version + `","Time":"` + now.Add(-age).Format(time.RFC3339) + `"}`
}

func (p *proxy) run(content string, cooldown time.Duration) (modbump.Result, error) {
	return modbump.Run(context.Background(), p.server.Client(), []byte(content), module, modbump.Options{ProxyAddress: p.server.URL, Cooldown: cooldown, Now: now})
}

func TestARequiredModuleWithANewerVersionPastTheCooldownMoves(t *testing.T) {
	p := newProxy(t, http.StatusOK, latest("v1.7.13", 5*24*time.Hour))

	got, err := p.run(goMod, 72*time.Hour)

	if err != nil {
		t.Fatal(err)
	}
	if got.Module != module || got.From != "v1.7.12" || got.To != "v1.7.13" || got.Note != "" {
		t.Errorf("got %+v", got)
	}
	if len(p.paths) != 1 || p.paths[0] != "/github.com/rhysd/actionlint/@latest" {
		t.Errorf("asked for %q", p.paths)
	}
}

func TestAModuleAtItsLatestVersionHasNoMove(t *testing.T) {
	p := newProxy(t, http.StatusOK, latest("v1.7.12", 40*24*time.Hour))

	got, err := p.run(goMod, 72*time.Hour)

	if err != nil || got.To != "" || got.Note != "" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestANewerVersionInsideTheCooldownIsLeftAloneAndSaysWhy(t *testing.T) {
	p := newProxy(t, http.StatusOK, latest("v1.7.13", 24*time.Hour))

	got, err := p.run(goMod, 72*time.Hour)

	if err != nil || got.To != "" || !strings.Contains(got.Note, "cooldown") || !strings.Contains(got.Note, "v1.7.13") {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestAModuleIsFoundInASingleLineRequireToo(t *testing.T) {
	p := newProxy(t, http.StatusOK, latest("v1.7.13", 5*24*time.Hour))

	got, err := p.run("module x\n\nrequire github.com/rhysd/actionlint v1.7.12\n", 72*time.Hour)

	if err != nil || got.From != "v1.7.12" || got.To != "v1.7.13" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestARequirementFollowedByACommentIsRead(t *testing.T) {
	for name, content := range map[string]string{
		"a comment after a space":      "module x\n\nrequire github.com/rhysd/actionlint v1.7.12 // pinned for now\n",
		"a comment with no space":      "module x\n\nrequire github.com/rhysd/actionlint v1.7.12// pinned for now\n",
		"a comment in a require block": "module x\n\nrequire (\n\t// the linter\n\tgithub.com/rhysd/actionlint v1.7.12 // indirect\n)\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := newProxy(t, http.StatusOK, latest("v1.7.13", 5*24*time.Hour))

			got, err := p.run(content, 72*time.Hour)

			if err != nil || got.From != "v1.7.12" || got.To != "v1.7.13" {
				t.Errorf("got %+v, %v", got, err)
			}
		})
	}
}

func TestAnAnswerThatCannotBeTrustedIsRefused(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"a latest older than the required version": {http.StatusOK, latest("v1.7.11", 40*24*time.Hour), "older"},
		"a version dated in the future":            {http.StatusOK, latest("v1.7.13", -time.Hour), "future"},
		"an answer that is not found":              {http.StatusNotFound, "not found", "404"},
		"an answer that is not JSON":               {http.StatusOK, "<html>", "JSON"},
		"a version with no leading v":              {http.StatusOK, latest("1.7.13", 5*24*time.Hour), "three numbers"},
		"a pre-release version":                    {http.StatusOK, latest("v1.8.0-rc.1", 5*24*time.Hour), "three numbers"},
		"a pseudo-version":                         {http.StatusOK, latest("v1.7.14-0.20260101000000-abcdef123456", 5*24*time.Hour), "three numbers"},
		"an incompatible version":                  {http.StatusOK, latest("v2.0.0+incompatible", 5*24*time.Hour), "three numbers"},
		"a version with a line break":              {http.StatusOK, `{"Version":"v1.7.13\n- injected","Time":"2026-10-01T00:00:00Z"}`, "three numbers"},
		"an answer with no time":                   {http.StatusOK, `{"Version":"v1.7.13"}`, "time"},
		"an answer that is far too large":          {http.StatusOK, `{"Version":"v1.7.13","Time":"2026-10-01T00:00:00Z","Padding":"` + strings.Repeat("a", 70_000) + `"}`, "larger"},
		"a redirect":                               {http.StatusFound, "", "302"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProxy(t, c.status, c.body)

			got, err := p.run(goMod, 72*time.Hour)

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %+v, %v; want an error containing %q", got, err, c.want)
			}
		})
	}
}

func TestAGoModThatCannotBeBumpedIsRefusedBeforeAnythingIsAsked(t *testing.T) {
	cases := map[string]string{
		"a module it does not require":         "module x\n\nrequire golang.org/x/sync v0.1.0\n",
		"a module it replaces":                 goMod + "\nreplace github.com/rhysd/actionlint => ../actionlint\n",
		"a module it replaces by a version":    goMod + "\nreplace github.com/rhysd/actionlint v1.7.12 => github.com/fork/actionlint v1.7.12\n",
		"a required version of a pre-release":  "module x\n\nrequire github.com/rhysd/actionlint v1.7.12-rc.1\n",
		"an empty go.mod":                      "",
		"a require that is a comment":          "module x\n\n// require github.com/rhysd/actionlint v1.7.12\n",
		"a module named in a trailing comment": "module x\n\nrequire golang.org/x/sync v0.1.0 // github.com/rhysd/actionlint v1.7.12\n",
		"only modules it is a prefix of":       "module x\n\nrequire (\n\tgithub.com/rhysd/actionlint/v2 v2.0.0\n\tgithub.com/rhysd/actionlint-extra v1.0.0\n)\n",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProxy(t, http.StatusOK, latest("v1.7.13", 5*24*time.Hour))

			if _, err := p.run(content, 72*time.Hour); err == nil {
				t.Error("want an error")
			}
			if len(p.paths) != 0 {
				t.Errorf("asked %q", p.paths)
			}
		})
	}
}

// neverAsked is a transport for a test that must be refused before anything is sent: a request
// fails the test, and nothing leaves the machine.
type neverAsked struct{ t *testing.T }

func (n neverAsked) RoundTrip(request *http.Request) (*http.Response, error) {
	n.t.Errorf("a request was made to %s", request.URL.Redacted())

	return nil, errors.New("no request is allowed")
}

func TestAProxyAddressThatCouldLeakOrBeRewrittenIsRefusedBeforeAnythingIsSent(t *testing.T) {
	for name, address := range map[string]string{
		"plain http":          "http://proxy.golang.org",
		"credentials":         "https://user:pass@proxy.golang.org",
		"no host":             "https://",
		"a port and no host":  "https://:8080",
		"a query":             "https://proxy.golang.org?x=1",
		"a fragment":          "https://proxy.golang.org#x",
		"something not a URL": "::not a url::",
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: neverAsked{t}}

			_, err := modbump.Run(context.Background(), client, []byte(goMod), module, modbump.Options{ProxyAddress: address, Cooldown: time.Hour, Now: now})

			if err == nil || !strings.Contains(err.Error(), "proxy address") {
				t.Errorf("want %q refused as a proxy address, got %v", address, err)
			}
		})
	}
}

func TestAModuleNameThatCouldChangeTheRequestIsRefused(t *testing.T) {
	for _, name := range []string{"", "github.com/Rhysd/actionlint", "github.com/rhysd/../x", "github.com/rhysd/actionlint?x=1", "github.com/rhysd/action lint", "/github.com/rhysd/actionlint", "github.com/rhysd/actionlint/@latest"} {
		t.Run(name, func(t *testing.T) {
			p := newProxy(t, http.StatusOK, latest("v1.7.13", 5*24*time.Hour))

			_, err := modbump.Run(context.Background(), p.server.Client(), []byte("module x\n\nrequire "+name+" v1.7.12\n"), name, modbump.Options{ProxyAddress: p.server.URL, Cooldown: time.Hour, Now: now})

			if err == nil || len(p.paths) != 0 {
				t.Errorf("err %v, asked %q", err, p.paths)
			}
		})
	}
}
