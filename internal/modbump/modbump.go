// Package modbump decides whether a Go module the program is built from has a newer release that
// is old enough to adopt. It reads go.mod and the module proxy and changes nothing: the caller
// runs go get, which fetches the module and checks it against the checksum database.
package modbump

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
)

const maxAnswer = 64 << 10

// A module path is written lower case, so it needs no escaping in the proxy's address, and it
// cannot hold a character that changes what is asked for.
var modulePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(/[a-z0-9._-]+)+$`)

// Options say where to look and how long a version must have been out.
type Options struct {
	// ProxyAddress is the https address of the module proxy.
	ProxyAddress string
	// Cooldown is how old a version must be before it is taken.
	Cooldown time.Duration
	// Now is the time the age of a version is judged at.
	Now time.Time
}

// Result says what to do with a module. To is empty when there is nothing to move to; Note says
// why when a newer version is waiting out the cooldown.
type Result struct {
	Module, From, To, Note string
}

type answer struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
}

// Run judges the module as go.mod requires it against the latest version the proxy knows.
func Run(ctx context.Context, client *http.Client, goMod []byte, module string, o Options) (Result, error) {
	if !modulePattern.MatchString(module) || hasDotSegment(module) {
		return Result{}, fmt.Errorf("%q is not a module path that can be asked for", module)
	}
	proxy, err := url.Parse(o.ProxyAddress)
	if err != nil || proxy.Scheme != "https" || proxy.Host == "" || proxy.User != nil || proxy.RawQuery != "" || proxy.Fragment != "" {
		return Result{}, fmt.Errorf("the proxy address %q must be a plain https address with a host, no credentials and no query", o.ProxyAddress)
	}
	current, err := required(goMod, module)
	if err != nil {
		return Result{}, err
	}

	found, err := lookup(ctx, client, proxy, module)
	if err != nil {
		return Result{}, err
	}

	order, comparable := bump.Compare(current, found.Version)
	if !comparable {
		return Result{}, fmt.Errorf("cannot compare the required %s with the latest %s: both must be three numbers", current, found.Version)
	}
	switch {
	case order > 0:
		return Result{}, fmt.Errorf("the latest version of %s is %s, older than the required %s; a version is never rolled back", module, found.Version, current)
	case order == 0:
		return Result{Module: module, From: current}, nil
	}

	age := o.Now.Sub(found.Time)
	switch {
	case age < 0:
		return Result{}, fmt.Errorf("%s %s claims to be published in the future (%s)", module, found.Version, found.Time.Format(time.RFC3339))
	case age < o.Cooldown:
		return Result{Module: module, From: current, Note: fmt.Sprintf("%s %s is %s old, inside the %s cooldown; %s stays required", module, found.Version, age.Round(time.Minute), o.Cooldown, current)}, nil
	}

	return Result{Module: module, From: current, To: found.Version}, nil
}

func hasDotSegment(module string) bool {
	for _, segment := range strings.Split(module, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}

	return false
}

// required is the version go.mod requires the module at. A module that is replaced is refused,
// as moving what is required would change nothing about what is built.
func required(goMod []byte, module string) (string, error) {
	version, block := "", ""
	for _, line := range strings.Split(string(goMod), "\n") {
		if comment := strings.Index(line, "//"); comment >= 0 {
			line = line[:comment]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		kind, args := block, fields
		switch {
		case block != "" && fields[0] == ")":
			block = ""

			continue
		case block == "" && len(fields) == 2 && fields[1] == "(":
			block = fields[0]

			continue
		case block == "":
			kind, args = fields[0], fields[1:]
		}

		if len(args) == 0 || args[0] != module {
			continue
		}
		switch kind {
		case "replace":
			return "", fmt.Errorf("go.mod replaces %s, so moving the version it requires would change nothing that is built", module)
		case "require":
			if len(args) < 2 {
				return "", fmt.Errorf("go.mod requires %s with no version", module)
			}
			version = args[1]
		}
	}

	if version == "" {
		return "", fmt.Errorf("go.mod does not require %s", module)
	}
	if _, ok := bump.Compare(version, version); !ok {
		return "", fmt.Errorf("go.mod requires %s at %q, which is not a version of three numbers", module, version)
	}

	return version, nil
}

// lookup asks the proxy for the latest version of the module and judges what it says.
func lookup(ctx context.Context, client *http.Client, proxy *url.URL, module string) (answer, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.JoinPath(module, "@latest").String(), nil)
	if err != nil {
		return answer{}, fmt.Errorf("cannot ask for the latest version of %s: %w", module, err)
	}
	request.Header.Set("User-Agent", "actions-qa-dev")

	direct := *client
	direct.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	response, err := direct.Do(request)
	if err != nil {
		return answer{}, fmt.Errorf("cannot ask %s for the latest version of %s: %w", proxy.Host, module, err)
	}
	defer closeBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return answer{}, fmt.Errorf("the latest version of %s: %s", module, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer+1))
	if err != nil {
		return answer{}, fmt.Errorf("cannot read the latest version of %s: %w", module, err)
	}
	if len(body) > maxAnswer {
		return answer{}, fmt.Errorf("the latest version of %s is larger than %d bytes", module, maxAnswer)
	}

	var found answer
	if err := json.Unmarshal(body, &found); err != nil {
		return answer{}, fmt.Errorf("the latest version of %s is not JSON: %w", module, err)
	}
	if _, ok := bump.Compare(found.Version, found.Version); !ok {
		return answer{}, fmt.Errorf("the latest version of %s (%q) is not a version of three numbers, so it is not a release", module, found.Version)
	}
	if found.Time.IsZero() {
		return answer{}, fmt.Errorf("the latest version of %s (%s) has no time, so its age is unknown", module, found.Version)
	}

	return found, nil
}

func closeBody(closer io.Closer) {
	if err := closer.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close a response: %v\n", err)
	}
}
