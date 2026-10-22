// Package branchprotection reads, from the GitHub API, the status check contexts a branch of a
// repository requires before a merge: its classic branch protection and the rulesets that apply
// to it. It makes a request only when asked to, sends the token only to the address it was given
// (https, or a loopback address for a test), and never follows a redirect, so the token goes
// nowhere else. What it cannot read is an error and never an empty answer, because "requires
// nothing" and "could not look" are different facts.
package branchprotection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// ErrUnreadable marks a protection that could not be read, for any reason.
var ErrUnreadable = errors.New("cannot read the branch protection")

// DefaultBranch is the name that stands for the repository's default branch.
const DefaultBranch = "default"

const (
	maxBody    = 1 << 20
	timeout    = 30 * time.Second
	rulesLimit = 100
)

var (
	repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	onlyDots       = regexp.MustCompile(`^\.+$`)
)

// Client reads the protection of branches of repositories on one GitHub API.
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

// Result is the contexts a branch requires, sorted and each once, and the branch they are for.
type Result struct {
	Branch   string
	Contexts []string
}

// New makes a client for the API at address, which has to be https (or a loopback address, for a
// test) with no credentials, query or fragment of its own. A nil httpClient is a default one.
// Redirects are never followed, whatever client is given.
func New(address, token string, httpClient *http.Client) (*Client, error) {
	base, err := url.Parse(address)
	switch {
	case err != nil || address == "":
		return nil, fmt.Errorf("the GitHub API address %q is not a URL", finding.Escape(address))
	case base.Host == "" || base.Hostname() == "":
		return nil, fmt.Errorf("the GitHub API address %q has no host", finding.Escape(address))
	case base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.ForceQuery:
		return nil, fmt.Errorf("the GitHub API address %q carries credentials, a query or a fragment", finding.Escape(address))
	case base.Scheme != "https" && !(base.Scheme == "http" && loopback(base.Hostname())):
		return nil, fmt.Errorf("the GitHub API address %q is not https, and the token would go in the clear", finding.Escape(address))
	}
	if token == "" || strings.ContainsFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return nil, errors.New("the GitHub token is empty or has a space or control character")
	}

	var inner http.Client
	if httpClient != nil {
		inner = *httpClient
	}
	if inner.Timeout == 0 {
		inner.Timeout = timeout
	}

	inner.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	return &Client{base: base, token: token, http: &inner}, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// ValidRepository says whether text is an owner/name a request can be made for.
func ValidRepository(repository string) error {
	parts := strings.Split(repository, "/")
	if !repositoryName.MatchString(repository) || onlyDots.MatchString(parts[0]) || onlyDots.MatchString(parts[1]) {
		return fmt.Errorf("the repository %q is not owner/name", finding.Escape(repository))
	}

	return nil
}

// RequiredContexts reads the contexts the branch of the repository (owner/name) requires. The
// branch DefaultBranch means the repository's default branch, which is looked up.
func (c *Client) RequiredContexts(repository, branch string) (Result, error) {
	if err := ValidRepository(repository); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(branch) == "" || strings.ContainsFunc(branch, unicode.IsControl) || strings.Contains(branch, "..") ||
		slices.Contains(strings.Split(branch, "/"), ".") {
		return Result{}, fmt.Errorf("the branch %q is not a branch name", finding.Escape(branch))
	}

	root := "repos/" + repository
	if branch == DefaultBranch {
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := c.get(root, nil, &repo); err != nil {
			return Result{}, err
		}
		if repo.DefaultBranch == "" {
			return Result{}, fmt.Errorf("%w: %s names no default branch", ErrUnreadable, repository)
		}
		branch = repo.DefaultBranch
	}
	escaped := url.PathEscape(branch)

	contexts, err := c.classic(root + "/branches/" + escaped + "/protection/required_status_checks")
	if err != nil {
		return Result{}, err
	}
	fromRules, err := c.rules(root + "/rules/branches/" + escaped)
	if err != nil {
		return Result{}, err
	}

	all := append(contexts, fromRules...)
	slices.Sort(all)

	return Result{Branch: branch, Contexts: slices.Compact(all)}, nil
}

// classic reads the contexts of the branch protection, none when the branch has none.
func (c *Client) classic(path string) ([]string, error) {
	var protection struct {
		Contexts []string `json:"contexts"`
		Checks   []struct {
			Context string `json:"context"`
		} `json:"checks"`
	}

	err := c.get(path, nil, &protection)
	var missing *notFoundError
	if errors.As(err, &missing) && (missing.message == "Branch not protected" || missing.message == "Required status checks not enabled") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	contexts := slices.Clone(protection.Contexts)
	for _, check := range protection.Checks {
		contexts = append(contexts, check.Context)
	}

	return contexts, nil
}

// rules reads the contexts the rulesets that apply to the branch require.
func (c *Client) rules(path string) ([]string, error) {
	var rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			Checks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	}
	if err := c.get(path, url.Values{"per_page": {fmt.Sprint(rulesLimit)}}, &rules); err != nil {
		return nil, err
	}
	if len(rules) >= rulesLimit {
		return nil, fmt.Errorf("%w: the branch has %d or more rules, which is more than one page and not all were read", ErrUnreadable, rulesLimit)
	}

	var contexts []string
	for _, rule := range rules {
		if rule.Type != "required_status_checks" {
			continue
		}
		for _, check := range rule.Parameters.Checks {
			if check.Context == "" {
				return nil, fmt.Errorf("%w: a ruleset requires a status check with no context", ErrUnreadable)
			}
			contexts = append(contexts, check.Context)
		}
	}

	return contexts, nil
}

// notFoundError is a 404, with the message GitHub gave.
type notFoundError struct{ message string }

func (e *notFoundError) Error() string { return "not found: " + e.message }

// get requests the path under the API address and decodes the JSON answer into out.
func (c *Client) get(path string, query url.Values, out any) error {
	target := strings.TrimRight(c.base.String(), "/") + "/" + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnreadable, c.redact(err.Error()))
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "actions-qa-ci")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnreadable, c.redact(err.Error()))
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err := errors.Join(readErr, response.Body.Close()); err != nil {
		return fmt.Errorf("%w: %s", ErrUnreadable, c.redact(err.Error()))
	}
	if len(body) > maxBody {
		return fmt.Errorf("%w: the answer for %s is larger than %d bytes", ErrUnreadable, path, maxBody)
	}

	if response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %w", ErrUnreadable, &notFoundError{message: c.message(body)})
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s answered %d %s", ErrUnreadable, path, response.StatusCode, c.message(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: the answer for %s is not what was expected: %s", ErrUnreadable, path, c.redact(err.Error()))
	}

	return nil
}

// message is the "message" GitHub gives with an error, shortened and made safe to print.
func (c *Client) message(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.Message == "" {
		return ""
	}
	text := c.redact(parsed.Message)
	if len(text) > 200 {
		text = text[:200] + "…"
	}

	return finding.Escape(text)
}

// redact removes the token from text, wherever it came from.
func (c *Client) redact(text string) string {
	return strings.ReplaceAll(text, c.token, "[token]")
}
