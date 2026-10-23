package bump

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// pypiProjects names the tools that are distributed as wheels, with the PyPI project each comes
// from. A wheel is pinned by the sha256 PyPI publishes for it, which the download has to match:
// the route is "pypi-digest".
var pypiProjects = map[string]string{
	"zizmor": "zizmor",
}

// maxPyPI bounds the answer to a project lookup, which lists the files of every release.
const maxPyPI = 8 << 20

type pypiFile struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Digests  struct {
		SHA256 string `json:"sha256"`
	} `json:"digests"`
	Yanked bool   `json:"yanked"`
	Upload string `json:"upload_time_iso_8601"`
}

type pypiRelease struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	URLs []pypiFile `json:"urls"`
}

// pypiBase judges the address of PyPI the same way the API address is judged.
func pypiBase(address string) (*url.URL, error) {
	base, err := url.Parse(address)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("the PyPI address %q must be a plain https address with a host, no credentials and no query", address)
	}

	return base, nil
}

// wheelHost is the host every wheel of the tool is pinned at, once each is a plain https address
// of a wheel whose file name holds the pinned version exactly once, and so can be replaced by the
// same wheel of another version.
func wheelHost(tool toolchain.Tool) (string, error) {
	host := ""
	for platform, asset := range tool.Assets {
		address, err := url.Parse(asset.URL)
		if err != nil || address.Scheme != "https" || address.Hostname() == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
			return "", fmt.Errorf("the wheel for %s (%q) must be a plain https address with a host, no credentials, no query and no fragment", platform, asset.URL)
		}
		name := path.Base(address.Path)
		if !strings.HasSuffix(name, ".whl") || strings.Count(name, tool.Version) != 1 {
			return "", fmt.Errorf("the wheel for %s (%q) is not a .whl whose file name holds the pinned version %s exactly once, so it cannot be moved to another", platform, asset.URL, tool.Version)
		}
		if !strings.Contains(asset.Member, tool.Version) {
			return "", fmt.Errorf("the member %q of the wheel for %s does not hold the pinned version %s, so it cannot be moved to another", asset.Member, platform, tool.Version)
		}
		if host != "" && host != address.Host {
			return "", fmt.Errorf("the wheels of one tool come from two hosts, %s and %s", host, address.Host)
		}
		host = address.Host
	}
	if host == "" {
		return "", fmt.Errorf("the tool has no asset")
	}

	return host, nil
}

// bumpWheel returns the tool as it should be pinned now, and a note when a newer release is
// waiting out the cooldown. Every wheel it takes is one PyPI lists for that release, on the host
// the old one was pinned at, not yanked, and whose download has the sha256 PyPI publishes.
func bumpWheel(ctx context.Context, client *http.Client, base *url.URL, o Options, project string, tool toolchain.Tool) (toolchain.Tool, string, error) {
	host, err := wheelHost(tool)
	if err != nil {
		return toolchain.Tool{}, "", err
	}
	release, err := lookupPyPI(ctx, client, base, project)
	if err != nil {
		return toolchain.Tool{}, "", err
	}

	latest := release.Info.Version
	order, ok := Compare(tool.Version, latest)
	switch {
	case !ok:
		return toolchain.Tool{}, "", fmt.Errorf("cannot compare the pinned version %q with the latest release %q: both must be three numbers", tool.Version, latest)
	case order > 0:
		return toolchain.Tool{}, "", fmt.Errorf("the latest release %s is older than the pinned %s; a release is never rolled back", latest, tool.Version)
	case order == 0:
		return copyOf(tool), "", nil
	}

	chosen := map[string]pypiFile{}
	var newest time.Time
	for platform, asset := range tool.Assets {
		address, err := url.Parse(asset.URL)
		if err != nil {
			return toolchain.Tool{}, "", fmt.Errorf("cannot move %q: %w", asset.URL, err)
		}
		want := strings.Replace(path.Base(address.Path), tool.Version, latest, 1)

		file, found := findFile(release.URLs, want)
		if !found {
			return toolchain.Tool{}, "", fmt.Errorf("PyPI lists no file named %s for %s in %s", want, platform, latest)
		}
		uploaded, err := checkFile(file, host)
		if err != nil {
			return toolchain.Tool{}, "", fmt.Errorf("%s: %w", want, err)
		}
		if uploaded.After(newest) {
			newest = uploaded
		}
		chosen[platform] = file
	}

	age := o.Now.Sub(newest)
	switch {
	case age < 0:
		return toolchain.Tool{}, "", fmt.Errorf("the release %s claims to have been uploaded in the future (%s)", latest, newest.Format(time.RFC3339))
	case age < o.Cooldown:
		return copyOf(tool), fmt.Sprintf("%s is %s old, inside the %s cooldown; %s stays pinned", latest, age.Round(time.Minute), o.Cooldown, tool.Version), nil
	}

	next := toolchain.Tool{Version: latest, Assets: map[string]toolchain.Asset{}}
	for platform, asset := range tool.Assets {
		file := chosen[platform]
		member := strings.ReplaceAll(asset.Member, tool.Version, latest)
		pinned, err := toolchain.Pin(ctx, client, file.URL, member)
		if err != nil {
			return toolchain.Tool{}, "", fmt.Errorf("cannot pin %s for %s: %w", latest, platform, err)
		}
		if pinned.ArchiveSHA256 != file.Digests.SHA256 {
			return toolchain.Tool{}, "", fmt.Errorf("the download of %s is %s but PyPI publishes the digest %s", file.Filename, pinned.ArchiveSHA256, file.Digests.SHA256)
		}
		next.Assets[platform] = pinned
	}

	return next, "", nil
}

func findFile(files []pypiFile, name string) (pypiFile, bool) {
	for _, file := range files {
		if file.Filename == name {
			return file, true
		}
	}

	return pypiFile{}, false
}

// checkFile judges one file PyPI lists, and returns when it was uploaded.
func checkFile(file pypiFile, host string) (time.Time, error) {
	if file.Yanked {
		return time.Time{}, fmt.Errorf("the file is yanked")
	}
	address, err := url.Parse(file.URL)
	if err != nil || address.Scheme != "https" || address.Hostname() == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return time.Time{}, fmt.Errorf("the address %q must be a plain https address with a host, no credentials, no query and no fragment", file.URL)
	}
	if address.Host != host {
		return time.Time{}, fmt.Errorf("the file is at the host %s, not at %s where the wheel is pinned", address.Host, host)
	}
	if path.Base(address.Path) != file.Filename {
		return time.Time{}, fmt.Errorf("the address %q does not end in the file's name", file.URL)
	}
	if !sha256Pattern.MatchString(file.Digests.SHA256) {
		return time.Time{}, fmt.Errorf("PyPI publishes no sha256 digest (%q)", file.Digests.SHA256)
	}
	uploaded, err := time.Parse(time.RFC3339, file.Upload)
	if err != nil {
		return time.Time{}, fmt.Errorf("the upload time %q is not a time: %w", file.Upload, err)
	}

	return uploaded, nil
}

// lookupPyPI asks PyPI for a project and judges what it says.
func lookupPyPI(ctx context.Context, client *http.Client, base *url.URL, project string) (pypiRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.JoinPath("pypi", project, "json").String(), nil)
	if err != nil {
		return pypiRelease{}, fmt.Errorf("cannot ask for the latest release of %s: %w", project, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "actions-qa-dev")

	direct := *client
	direct.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	response, err := direct.Do(request)
	if err != nil {
		return pypiRelease{}, fmt.Errorf("cannot ask %s for the latest release of %s: %w", base.Host, project, err)
	}
	defer closeBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return pypiRelease{}, fmt.Errorf("the latest release of %s: %s", project, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPyPI+1))
	if err != nil {
		return pypiRelease{}, fmt.Errorf("cannot read the latest release of %s: %w", project, err)
	}
	if len(body) > maxPyPI {
		return pypiRelease{}, fmt.Errorf("the latest release of %s is larger than %d bytes", project, maxPyPI)
	}

	var found pypiRelease
	if err := json.Unmarshal(body, &found); err != nil {
		return pypiRelease{}, fmt.Errorf("the latest release of %s is not JSON: %w", project, err)
	}

	return found, nil
}
