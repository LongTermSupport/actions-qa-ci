package bump_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const (
	oldWheelVersion = "1.31.0"
	newWheelVersion = "1.31.1"
)

// wheelBytes is a wheel of a version holding the executable the manifest pins as a member.
func wheelBytes(t *testing.T, version, content string) []byte {
	t.Helper()

	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	member, err := writer.Create("zizmor-" + version + ".data/scripts/zizmor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// pypiFile is one file of the latest release as PyPI's JSON describes it.
type pypiFile struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Digests  struct {
		SHA256 string `json:"sha256"`
	} `json:"digests"`
	Yanked bool   `json:"yanked"`
	Upload string `json:"upload_time_iso_8601"`
}

// pypi is a fake PyPI and a fake file host in one server: the JSON of the zizmor project and the
// wheels it points at.
type pypi struct {
	server *httptest.Server
	mu     sync.Mutex
	tokens []string
	files  []pypiFile
	latest string
	status int
	body   string
	paths  []string
	// tolerant lets a write fail: a client that stops reading an oversized answer is what the
	// test is about.
	tolerant bool
	// stream, when above zero, is how many bytes of filler the answer is made of, written a
	// megabyte at a time; sent is how many of them reached the socket.
	stream int
	sent   atomic.Int64
}

var platforms = map[string]string{"linux_amd64": "x86_64", "linux_arm64": "aarch64"}

func wheelName(version, arch string) string {
	return "zizmor-" + version + "-py3-none-musllinux_1_2_" + arch + ".whl"
}

func newPyPI(t *testing.T, latest string, age time.Duration) *pypi {
	t.Helper()

	p := &pypi{latest: latest, status: http.StatusOK}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.tokens = append(p.tokens, r.Header.Get("Authorization"))
		p.paths = append(p.paths, r.URL.Path)
		p.mu.Unlock()

		switch {
		case r.URL.Path == "/pypi/zizmor/json":
			if p.status == http.StatusFound {
				http.Redirect(w, r, "https://example.invalid/elsewhere", p.status)

				return
			}
			w.WriteHeader(p.status)
			if p.stream > 0 {
				chunk := make([]byte, 1<<20)
				for int(p.sent.Load()) < p.stream {
					written, err := w.Write(chunk)
					p.sent.Add(int64(written))
					if err != nil {
						return
					}
				}

				return
			}
			body := p.body
			if body == "" {
				encoded, err := json.Marshal(map[string]any{"info": map[string]string{"version": p.latest}, "urls": p.files})
				if err != nil {
					t.Error(err)
				}
				body = string(encoded)
			}
			if _, err := w.Write([]byte(body)); err != nil && !p.tolerant {
				t.Error(err)
			}
		case strings.HasPrefix(r.URL.Path, "/packages/"):
			version := newWheelVersion
			if strings.Contains(r.URL.Path, oldWheelVersion) {
				version = oldWheelVersion
			}
			if _, err := w.Write(wheelBytes(t, version, "binary of "+r.URL.Path)); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)

	for _, arch := range platforms {
		name := wheelName(latest, arch)
		file := pypiFile{Filename: name, URL: p.server.URL + "/packages/aa/bb/" + name, Upload: now.Add(-age).Format(time.RFC3339)}
		file.Digests.SHA256 = sum(wheelBytes(t, latest, "binary of /packages/aa/bb/"+name))
		p.files = append(p.files, file)
	}

	return p
}

// pinned is a manifest pinning zizmor at the old version, from the same host.
func (p *pypi) pinned(t *testing.T) toolchain.Manifest {
	t.Helper()

	assets := map[string]toolchain.Asset{}
	for platform, arch := range platforms {
		name := wheelName(oldWheelVersion, arch)
		address := p.server.URL + "/packages/cc/dd/" + name
		assets[platform] = toolchain.Asset{
			URL:           address,
			ArchiveSHA256: sum(wheelBytes(t, oldWheelVersion, "binary of /packages/cc/dd/"+name)),
			Member:        "zizmor-" + oldWheelVersion + ".data/scripts/zizmor",
			BinarySHA256:  sum([]byte("binary of /packages/cc/dd/" + name)),
		}
	}

	return toolchain.Manifest{Tools: map[string]toolchain.Tool{"zizmor": {Version: oldWheelVersion, Assets: assets}}}
}

// withEachWheel returns the manifest with the address of each wheel of zizmor changed.
func withEachWheel(manifest toolchain.Manifest, change func(address string) string) toolchain.Manifest {
	tool := manifest.Tools["zizmor"]
	for platform, asset := range tool.Assets {
		asset.URL = change(asset.URL)
		tool.Assets[platform] = asset
	}

	return manifest
}

func (p *pypi) options(token string) bump.Options {
	return bump.Options{APIAddress: p.server.URL, PyPIAddress: p.server.URL, Token: token, Cooldown: 72 * time.Hour, Now: now}
}

func (p *pypi) run(t *testing.T, m toolchain.Manifest, o bump.Options) (bump.Result, error) {
	t.Helper()

	return bump.Run(context.Background(), p.server.Client(), m, o)
}

func TestAWheelToolMovesToThePyPIReleaseOnceItIsPastTheCooldown(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)

	got, err := p.run(t, p.pinned(t), p.options(""))

	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0] != (bump.Change{Tool: "zizmor", From: oldWheelVersion, To: newWheelVersion}) {
		t.Fatalf("changes = %+v", got.Changes)
	}
	moved := got.Manifest.Tools["zizmor"]
	if moved.Version != newWheelVersion {
		t.Errorf("version = %s", moved.Version)
	}
	for platform, arch := range platforms {
		name := wheelName(newWheelVersion, arch)
		address := p.server.URL + "/packages/aa/bb/" + name
		want := toolchain.Asset{
			URL:           address,
			ArchiveSHA256: sum(wheelBytes(t, newWheelVersion, "binary of /packages/aa/bb/"+name)),
			Member:        "zizmor-" + newWheelVersion + ".data/scripts/zizmor",
			BinarySHA256:  sum([]byte("binary of /packages/aa/bb/" + name)),
		}
		if moved.Assets[platform] != want {
			t.Errorf("%s: asset = %+v, want %+v", platform, moved.Assets[platform], want)
		}
	}
}

func TestAWheelToolAtTheLatestReleaseChangesNothing(t *testing.T) {
	p := newPyPI(t, oldWheelVersion, 40*24*time.Hour)

	got, err := p.run(t, p.pinned(t), p.options(""))

	if err != nil || len(got.Changes) != 0 || got.Manifest.Tools["zizmor"].Version != oldWheelVersion {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestAWheelReleaseInsideTheCooldownIsLeftAloneAndSaid(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 24*time.Hour)

	got, err := p.run(t, p.pinned(t), p.options(""))

	if err != nil || len(got.Changes) != 0 || len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "cooldown") {
		t.Errorf("got %+v, %v", got, err)
	}
	if got.Manifest.Tools["zizmor"].Version != oldWheelVersion {
		t.Errorf("version = %s; it must stay pinned", got.Manifest.Tools["zizmor"].Version)
	}
}

func TestAWheelWhosePyPIDigestDiffersFromTheDownloadIsRefused(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
	p.files[0].Digests.SHA256 = strings.Repeat("0", 64)

	got, err := p.run(t, p.pinned(t), p.options(""))

	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Errorf("got %+v, %v; want a digest refusal", got, err)
	}
	if len(got.Changes) != 0 || len(got.Manifest.Tools) != 0 {
		t.Errorf("a refused run returned %+v", got)
	}
}

func TestWhatPyPIOffersThatCannotBeTrustedIsRefused(t *testing.T) {
	cases := map[string]struct {
		change func(p *pypi)
		want   string
	}{
		"a file with no published digest": {func(p *pypi) { p.files[0].Digests.SHA256 = "" }, "no sha256"},
		"a yanked file":                   {func(p *pypi) { p.files[0].Yanked = true }, "yanked"},
		"a platform with no file":         {func(p *pypi) { p.files = p.files[:1] }, "no file"},
		"a file on another host":          {func(p *pypi) { p.files[0].URL = "https://evil.example/packages/aa/bb/" + p.files[0].Filename }, "not at"},
		"a file address with credentials": {func(p *pypi) { p.files[0].URL = strings.Replace(p.files[0].URL, "https://", "https://user:pass@", 1) }, "plain https"},
		"a file address with a query":     {func(p *pypi) { p.files[0].URL += "?token=1" }, "plain https"},
		"a file address over plain http":  {func(p *pypi) { p.files[0].URL = strings.Replace(p.files[0].URL, "https://", "http://", 1) }, "plain https"},
		"a digest in capitals":            {func(p *pypi) { p.files[0].Digests.SHA256 = strings.ToUpper(p.files[0].Digests.SHA256) }, "no sha256"},
		"a digest that is too short":      {func(p *pypi) { p.files[0].Digests.SHA256 = p.files[0].Digests.SHA256[:63] }, "no sha256"},
		"a digest that is not hex":        {func(p *pypi) { p.files[0].Digests.SHA256 = strings.Repeat("z", 64) }, "no sha256"},
		"a file address of another name":  {func(p *pypi) { p.files[0].URL = p.server.URL + "/packages/aa/bb/other.whl" }, "name"},
		"a latest older than the pinned":  {func(p *pypi) { p.latest = "1.30.0" }, "older"},
		"a pre-release latest":            {func(p *pypi) { p.latest = "1.32.0rc1" }, "three numbers"},
		"an upload time in the future":    {func(p *pypi) { p.files[0].Upload = now.Add(time.Hour).Format(time.RFC3339) }, "future"},
		"a file with no upload time":      {func(p *pypi) { p.files[0].Upload = "" }, "upload"},
		"an answer that is not found":     {func(p *pypi) { p.status = http.StatusNotFound; p.body = "gone" }, "404"},
		"an answer that is not JSON":      {func(p *pypi) { p.body = "<html>" }, "JSON"},
		"a redirect":                      {func(p *pypi) { p.status = http.StatusFound }, "302"},
		"an answer larger than the cap": {func(p *pypi) {
			p.tolerant = true
			p.body = `{"info":{"version":"1.31.1"},"pad":"` + strings.Repeat("a", 9<<20) + `"}`
		}, "larger"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
			c.change(p)

			got, err := p.run(t, p.pinned(t), p.options(""))

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %+v, %v; want an error containing %q", got, err, c.want)
			}
			if len(got.Manifest.Tools) != 0 {
				t.Errorf("a refused run returned %+v", got.Manifest)
			}
		})
	}
}

func TestAWheelAssetThatCannotBeMovedIsRefusedBeforeAnythingIsAsked(t *testing.T) {
	cases := map[string]func(address string) string{
		"an address that is not https":    func(a string) string { return strings.Replace(a, "https://", "http://", 1) },
		"an address with a query":         func(a string) string { return a + "?x=1" },
		"a file that is not a wheel":      func(a string) string { return strings.TrimSuffix(a, ".whl") + ".tar.gz" },
		"a file name without the version": func(a string) string { return strings.ReplaceAll(a, oldWheelVersion, "9.9.9") },
		"a file name holding it twice":    func(a string) string { return strings.Replace(a, "py3-none", oldWheelVersion+"-py3-none", 1) },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPyPI(t, newWheelVersion, 5*24*time.Hour)

			_, err := p.run(t, withEachWheel(p.pinned(t), change), p.options(""))

			if err == nil {
				t.Error("want an error")
			}
			if len(p.paths) != 0 {
				t.Errorf("asked %q", p.paths)
			}
		})
	}
}

func TestTheCooldownIsJudgedOnTheNewestFileOfTheRelease(t *testing.T) {
	// one platform's file is old, the other's is an hour old: whichever the platforms are
	// visited in, the release is inside the cooldown.
	for _, fresh := range []int{0, 1} {
		p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
		p.files[fresh].Upload = now.Add(-time.Hour).Format(time.RFC3339)

		for range 40 {
			got, err := p.run(t, p.pinned(t), p.options(""))

			if err != nil || len(got.Changes) != 0 || len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "cooldown") {
				t.Fatalf("file %d fresh: got %+v, %v", fresh, got, err)
			}
		}
	}
}

func TestAWheelWhoseMemberDoesNotHoldTheVersionIsRefusedBeforeAnythingIsAsked(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
	manifest := p.pinned(t)
	tool := manifest.Tools["zizmor"]
	for platform, asset := range tool.Assets {
		asset.Member = "zizmor/scripts/zizmor"
		tool.Assets[platform] = asset
	}

	_, err := p.run(t, manifest, p.options(""))

	if err == nil || !strings.Contains(err.Error(), "member") {
		t.Errorf("got %v; want the member refused", err)
	}
	if len(p.paths) != 0 {
		t.Errorf("asked %q", p.paths)
	}
}

func TestWheelsPinnedAtTwoHostsAreRefusedBeforeAnythingIsAsked(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
	manifest := p.pinned(t)
	tool := manifest.Tools["zizmor"]
	asset := tool.Assets["linux_arm64"]
	asset.URL = strings.Replace(asset.URL, p.server.URL, "https://other.example", 1)
	tool.Assets["linux_arm64"] = asset

	_, err := p.run(t, manifest, p.options(""))

	if err == nil || !strings.Contains(err.Error(), "two hosts") {
		t.Errorf("got %v; want the two hosts refused", err)
	}
	if len(p.paths) != 0 {
		t.Errorf("asked %q", p.paths)
	}
}

func TestAnOversizedPyPIAnswerIsNotReadPastTheCap(t *testing.T) {
	const streamed = 128 << 20
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
	p.tolerant = true
	p.stream = streamed

	_, err := p.run(t, p.pinned(t), p.options(""))

	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("got %v; want the answer refused as larger than the cap", err)
	}
	if sent := p.sent.Load(); sent > streamed/2 {
		t.Errorf("the client took %d of %d bytes; it must stop reading at the cap", sent, streamed)
	}
}

func TestThePyPIAddressMustBePlainHTTPSAndIsNeededForAWheelTool(t *testing.T) {
	for name, address := range map[string]string{
		"empty":       "",
		"plain http":  "http://pypi.org",
		"credentials": "https://user:pass@pypi.org",
		"a port only": "https://:8443",
		"a query":     "https://pypi.org?x=1",
	} {
		t.Run(name, func(t *testing.T) {
			p := newPyPI(t, newWheelVersion, 5*24*time.Hour)
			options := p.options("")
			options.PyPIAddress = address

			_, err := p.run(t, p.pinned(t), options)

			if err == nil || !strings.Contains(err.Error(), "PyPI") {
				t.Errorf("got %v; want the address refused as a PyPI address", err)
			}
			if len(p.paths) != 0 {
				t.Errorf("asked %q", p.paths)
			}
		})
	}
}

func TestTheGitHubTokenGoesToNeitherPyPINorTheFileHost(t *testing.T) {
	p := newPyPI(t, newWheelVersion, 5*24*time.Hour)

	if _, err := p.run(t, p.pinned(t), p.options("a-secret-token")); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, header := range p.tokens {
		if header != "" {
			t.Errorf("a request carried the Authorization header %q", header)
		}
	}
}
