// Package bump proposes the next release of each tool the toolchain manifest pins. It looks up
// the latest release of the tool's repository, waits out a cooldown, downloads the assets for
// every platform and pins what it got. It trusts the download as far as the tool's own provenance
// route reaches, and only tools whose route is implemented here are bumped at all, so a tool whose
// route is still undecided is refused, never updated on weaker evidence.
package bump

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// routes names the tools this package may bump, with the provenance their releases have. A tool
// that is not here is refused: its route is a decision that has not been made.
var routes = map[string]string{
	"shellcheck": "none upstream: the release as GitHub serves it is pinned by its sha256",
	"zizmor":     "pypi-digest: the musl wheel is pinned by its sha256, which the download must match as PyPI publishes it",
}

// maxRelease bounds the answer to a release lookup.
const maxRelease = 1 << 20

var (
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
)

// Options say where to look and how long a release must have been out.
type Options struct {
	// APIAddress is the https address of the GitHub API.
	APIAddress string
	// PyPIAddress is the https address of PyPI, needed when a tool is a wheel. No token is sent
	// to it.
	PyPIAddress string
	// Token is sent to the release lookup, and to nothing else. It may be empty.
	Token string
	// Cooldown is how old a release must be before it is taken.
	Cooldown time.Duration
	// Now is the time the age of a release is judged at.
	Now time.Time
}

// Change is one tool moved to a newer release.
type Change struct {
	Tool, From, To string
}

// Result is the manifest as it would be after the bump, what changed in it, and what was left
// alone and why.
type Result struct {
	Manifest toolchain.Manifest
	Changes  []Change
	Notes    []string
}

// Run bumps every tool of the manifest that has a newer release past the cooldown. It is all or
// nothing: any tool that cannot be bumped safely fails the run, and nothing is returned.
func Run(ctx context.Context, client *http.Client, m toolchain.Manifest, o Options) (Result, error) {
	api, err := url.Parse(o.APIAddress)
	if err != nil || api.Scheme != "https" || api.Host == "" || api.User != nil {
		return Result{}, fmt.Errorf("the API address %q must be an https address with no credentials, so the token is never sent in the clear", o.APIAddress)
	}

	names := make([]string, 0, len(m.Tools))
	for name := range m.Tools {
		names = append(names, name)
	}
	slices.Sort(names)

	result := Result{Manifest: toolchain.Manifest{Tools: map[string]toolchain.Tool{}}}
	for _, name := range names {
		tool := m.Tools[name]
		if _, ok := routes[name]; !ok {
			return Result{}, fmt.Errorf("%s cannot be bumped: no provenance route is decided for it, and a release is not taken on weaker evidence than upstream offers", name)
		}

		var next toolchain.Tool
		var note string
		var err error
		if project, isWheel := pypiProjects[name]; isWheel {
			var base *url.URL
			if base, err = pypiBase(o.PyPIAddress); err == nil {
				next, note, err = bumpWheel(ctx, client, base, o, project, tool)
			}
		} else {
			next, note, err = bumpTool(ctx, client, api, o, name, tool)
		}
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", name, err)
		}
		if note != "" {
			result.Notes = append(result.Notes, name+": "+note)
		}
		if next.Version != tool.Version {
			result.Changes = append(result.Changes, Change{Tool: name, From: tool.Version, To: next.Version})
		}
		result.Manifest.Tools[name] = next
	}

	return result, nil
}

// bumpTool returns the tool as it should be pinned now, and a note when a newer release is
// waiting out the cooldown.
func bumpTool(ctx context.Context, client *http.Client, api *url.URL, o Options, name string, tool toolchain.Tool) (toolchain.Tool, string, error) {
	repository, err := repositoryOf(tool)
	if err != nil {
		return toolchain.Tool{}, "", err
	}
	latest, err := lookup(ctx, client, api, o.Token, repository)
	if err != nil {
		return toolchain.Tool{}, "", err
	}

	order, ok := Compare(tool.Version, latest.Tag)
	switch {
	case !ok:
		return toolchain.Tool{}, "", fmt.Errorf("cannot compare the pinned version %q with the latest release %q: both must be three numbers", tool.Version, latest.Tag)
	case order > 0:
		return toolchain.Tool{}, "", fmt.Errorf("the latest release %s is older than the pinned %s; a release is never rolled back", latest.Tag, tool.Version)
	case order == 0:
		return copyOf(tool), "", nil
	}

	age := o.Now.Sub(latest.Published)
	switch {
	case age < 0:
		return toolchain.Tool{}, "", fmt.Errorf("the release %s claims to be published in the future (%s)", latest.Tag, latest.Published.Format(time.RFC3339))
	case age < o.Cooldown:
		return copyOf(tool), fmt.Sprintf("%s is %s old, inside the %s cooldown; %s stays pinned", latest.Tag, age.Round(time.Minute), o.Cooldown, tool.Version), nil
	}

	next := toolchain.Tool{Version: latest.Tag, Assets: map[string]toolchain.Asset{}}
	for platform, asset := range tool.Assets {
		moved, err := moveAsset(asset.URL, tool.Version, latest.Tag)
		if err != nil {
			return toolchain.Tool{}, "", err
		}
		member := strings.ReplaceAll(asset.Member, tool.Version, latest.Tag)
		pinned, err := toolchain.Pin(ctx, client, moved, member)
		if err != nil {
			return toolchain.Tool{}, "", fmt.Errorf("cannot pin %s for %s: %w", latest.Tag, platform, err)
		}
		next.Assets[platform] = pinned
	}

	return next, "", nil
}

// moveAsset moves the address of a release download to another version by changing the tag and
// the file name and nothing else, so the host, the owner and the repository stay as pinned
// however many times the version happens to occur in them. The address has been through
// repositoryOf, so its path has the form /owner/name/releases/download/<tag>/<file>.
func moveAsset(address, from, to string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("cannot move %q: %w", address, err)
	}
	segments := strings.Split(parsed.Path, "/")
	if len(segments) != 7 {
		return "", fmt.Errorf("cannot move %q: it is not a release download", address)
	}
	segments[5] = to
	segments[6] = strings.ReplaceAll(segments[6], from, to)
	parsed.Path = strings.Join(segments, "/")
	parsed.RawPath = ""

	return parsed.String(), nil
}

func copyOf(tool toolchain.Tool) toolchain.Tool {
	assets := make(map[string]toolchain.Asset, len(tool.Assets))
	for platform, asset := range tool.Assets {
		assets[platform] = asset
	}

	return toolchain.Tool{Version: tool.Version, Assets: assets}
}

// repositoryOf is the owner/name every asset of the tool is downloaded from. Each asset has to
// be a release download of the pinned version, with no query, so that moving it to another
// version is a replacement of that version in its address and nothing else.
func repositoryOf(tool toolchain.Tool) (string, error) {
	repository := ""
	for platform, asset := range tool.Assets {
		address, err := url.Parse(asset.URL)
		if err != nil || address.Scheme != "https" || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
			return "", fmt.Errorf("the asset for %s (%q) must be a plain https address with a host, no credentials, no query and no fragment", platform, asset.URL)
		}
		parts := strings.Split(address.Path, "/")
		if len(parts) != 7 || parts[0] != "" || parts[3] != "releases" || parts[4] != "download" || !namePattern.MatchString(parts[1]) || !namePattern.MatchString(parts[2]) {
			return "", fmt.Errorf("the asset for %s (%q) is not a release download of the form /owner/name/releases/download/<tag>/<file>", platform, asset.URL)
		}
		if parts[5] != tool.Version || !strings.Contains(parts[6], tool.Version) {
			return "", fmt.Errorf("the asset for %s (%q) does not hold the pinned version %s in its tag and file name, so it cannot be moved to another", platform, asset.URL, tool.Version)
		}
		here := parts[1] + "/" + parts[2]
		if repository != "" && repository != here {
			return "", fmt.Errorf("the assets of one tool come from two repositories, %s and %s", repository, here)
		}
		repository = here
	}
	if repository == "" {
		return "", fmt.Errorf("the tool has no asset")
	}

	return repository, nil
}

type release struct {
	Tag        string    `json:"tag_name"`
	Published  time.Time `json:"published_at"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
}

// lookup asks the API for the latest release of a repository and judges what it says.
func lookup(ctx context.Context, client *http.Client, api *url.URL, token, repository string) (release, error) {
	address := api.JoinPath("repos", repository, "releases", "latest").String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return release{}, fmt.Errorf("cannot ask for the latest release: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "actions-qa-dev")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	// A redirect is not followed: the token would go wherever it points, and GitHub's own answer
	// to this request is not a redirect.
	direct := *client
	direct.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	response, err := direct.Do(request)
	if err != nil {
		return release{}, fmt.Errorf("cannot ask %s for the latest release: %w", api.Host, err)
	}
	defer closeBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("the latest release of %s: %s", repository, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxRelease+1))
	if err != nil {
		return release{}, fmt.Errorf("cannot read the latest release of %s: %w", repository, err)
	}
	if len(body) > maxRelease {
		return release{}, fmt.Errorf("the latest release of %s is larger than %d bytes", repository, maxRelease)
	}

	var found release
	if err := json.Unmarshal(body, &found); err != nil {
		return release{}, fmt.Errorf("the latest release of %s is not JSON: %w", repository, err)
	}
	switch {
	case found.Draft:
		return release{}, fmt.Errorf("the latest release of %s (%q) is a draft", repository, found.Tag)
	case found.Prerelease:
		return release{}, fmt.Errorf("the latest release of %s (%q) is a prerelease", repository, found.Tag)
	case found.Published.IsZero():
		return release{}, fmt.Errorf("the latest release of %s (%q) has no published_at, so its age is unknown", repository, found.Tag)
	}

	return found, nil
}

// Compare orders two versions of three numbers, with or without a leading v. The second result
// is false when either is anything else, such as a prerelease or a name.
func Compare(a, b string) (int, bool) {
	left, ok := parts(a)
	if !ok {
		return 0, false
	}
	right, ok := parts(b)
	if !ok {
		return 0, false
	}

	for i := range left {
		if left[i] != right[i] {
			if left[i] < right[i] {
				return -1, true
			}

			return 1, true
		}
	}

	return 0, true
}

func parts(version string) ([3]int, bool) {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return [3]int{}, false
	}
	var numbers [3]int
	for i := range numbers {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, false
		}
		numbers[i] = n
	}

	return numbers, true
}

// Marshal writes a manifest the way the file is kept: two-space indent, fields in the order of
// the structure, tools and platforms by name, no escaping of & or <, and a final newline.
func Marshal(m toolchain.Manifest) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(m); err != nil {
		return nil, fmt.Errorf("cannot write the manifest: %w", err)
	}

	return out.Bytes(), nil
}

func closeBody(closer io.Closer) {
	if err := closer.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close a response: %v\n", err)
	}
}
