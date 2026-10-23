package bump_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func sum(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

func tarball(t *testing.T, member, content string) []byte {
	t.Helper()

	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	if err := writer.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// upstream is a fake GitHub: the release API and the downloads of one repository, with the
// requests it saw.
type upstream struct {
	server  *httptest.Server
	mu      sync.Mutex
	latest  string
	status  int
	seen    []*http.Request
	archive map[string][]byte
}

func newUpstream(t *testing.T, latest string) *upstream {
	t.Helper()

	u := &upstream{latest: latest, status: http.StatusOK, archive: map[string][]byte{}}
	u.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.seen = append(u.seen, r)
		u.mu.Unlock()

		if r.URL.Path == "/repos/koalaman/shellcheck/releases/latest" {
			w.WriteHeader(u.status)
			// A client that stops reading at its cap closes the connection, which fails this
			// write; that is the behaviour under test, not a fault of the fake.
			if _, err := w.Write([]byte(u.latest)); err != nil {
				t.Log(err)
			}

			return
		}
		if body, ok := u.archive[r.URL.Path]; ok {
			if _, err := w.Write(body); err != nil {
				t.Error(err)
			}

			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(u.server.Close)

	return u
}

func release(tag string, published time.Time) string {
	return `{"tag_name":"` + tag + `","published_at":"` + published.Format(time.RFC3339) + `","draft":false,"prerelease":false}`
}

// offer makes the fake serve the assets of a release of shellcheck.
func (u *upstream) offer(t *testing.T, version string) {
	t.Helper()

	member := "shellcheck-" + version + "/shellcheck"
	u.archive["/koalaman/shellcheck/releases/download/"+version+"/shellcheck-"+version+".linux.x86_64.tar.gz"] = tarball(t, member, "amd64 "+version)
	u.archive["/koalaman/shellcheck/releases/download/"+version+"/shellcheck-"+version+".linux.aarch64.tar.gz"] = tarball(t, member, "arm64 "+version)
}

// pinned is a manifest pinning shellcheck v0.11.0 from the fake.
func (u *upstream) pinned() toolchain.Manifest {
	asset := func(arch string) toolchain.Asset {
		return toolchain.Asset{
			URL:           u.server.URL + "/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux." + arch + ".tar.gz",
			ArchiveSHA256: strings.Repeat("a", 64),
			Member:        "shellcheck-v0.11.0/shellcheck",
			BinarySHA256:  strings.Repeat("b", 64),
		}
	}

	return toolchain.Manifest{Tools: map[string]toolchain.Tool{
		"shellcheck": {Version: "v0.11.0", Assets: map[string]toolchain.Asset{"linux_amd64": asset("x86_64"), "linux_arm64": asset("aarch64")}},
	}}
}

func (u *upstream) options(token string) bump.Options {
	return bump.Options{APIAddress: u.server.URL, Token: token, Cooldown: 72 * time.Hour, Now: now}
}

func (u *upstream) requests() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()

	return append([]*http.Request(nil), u.seen...)
}

func TestANewerReleaseOutsideTheCooldownIsPinnedForEveryPlatform(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-4*24*time.Hour)))
	u.offer(t, "v0.12.0")

	got, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options(""))
	if err != nil {
		t.Fatal(err)
	}

	tool := got.Manifest.Tools["shellcheck"]
	if tool.Version != "v0.12.0" || len(got.Changes) != 1 || got.Changes[0] != (bump.Change{Tool: "shellcheck", From: "v0.11.0", To: "v0.12.0"}) {
		t.Fatalf("tool %+v, changes %+v", tool, got.Changes)
	}
	want := map[string]toolchain.Asset{
		"linux_amd64": {
			URL:           u.server.URL + "/koalaman/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.linux.x86_64.tar.gz",
			ArchiveSHA256: sum(u.archive["/koalaman/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.linux.x86_64.tar.gz"]),
			Member:        "shellcheck-v0.12.0/shellcheck",
			BinarySHA256:  sum([]byte("amd64 v0.12.0")),
		},
		"linux_arm64": {
			URL:           u.server.URL + "/koalaman/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.linux.aarch64.tar.gz",
			ArchiveSHA256: sum(u.archive["/koalaman/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.linux.aarch64.tar.gz"]),
			Member:        "shellcheck-v0.12.0/shellcheck",
			BinarySHA256:  sum([]byte("arm64 v0.12.0")),
		},
	}
	for platform, asset := range want {
		if tool.Assets[platform] != asset {
			t.Errorf("%s: %+v, want %+v", platform, tool.Assets[platform], asset)
		}
	}
	if old := u.pinned().Tools["shellcheck"].Version; old != "v0.11.0" {
		t.Errorf("the manifest passed in was changed: %s", old)
	}
}

func TestTheSameReleaseChangesNothingAndDownloadsNothing(t *testing.T) {
	u := newUpstream(t, release("v0.11.0", now.Add(-30*24*time.Hour)))

	got, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options(""))
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Changes) != 0 || got.Manifest.Tools["shellcheck"].Version != "v0.11.0" {
		t.Errorf("changes %+v", got.Changes)
	}
	if n := len(u.requests()); n != 1 {
		t.Errorf("%d requests; only the release lookup is needed", n)
	}
}

func TestAReleaseYoungerThanTheCooldownIsLeftAloneAndSaid(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-24*time.Hour)))
	u.offer(t, "v0.12.0")

	got, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options(""))
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Changes) != 0 || got.Manifest.Tools["shellcheck"].Version != "v0.11.0" {
		t.Errorf("changes %+v", got.Changes)
	}
	if len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "v0.12.0") || !strings.Contains(got.Notes[0], "cooldown") {
		t.Errorf("notes %q; the wait must be said, with the release", got.Notes)
	}
	if n := len(u.requests()); n != 1 {
		t.Errorf("%d requests; nothing is downloaded inside the cooldown", n)
	}
}

func TestAReleaseExactlyAtTheCooldownIsTaken(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-72*time.Hour)))
	u.offer(t, "v0.12.0")

	got, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options(""))

	if err != nil || len(got.Changes) != 1 {
		t.Errorf("changes %+v, error %v", got.Changes, err)
	}
}

func TestWhatCannotBeTrustedIsRefusedAndNothingIsChanged(t *testing.T) {
	older := release("v0.10.0", now.Add(-30*24*time.Hour))
	cases := map[string]struct {
		latest string
		status int
		want   string
	}{
		"an older latest release":        {older, http.StatusOK, "older"},
		"a draft":                        {`{"tag_name":"v0.12.0","published_at":"2026-10-01T00:00:00Z","draft":true,"prerelease":false}`, http.StatusOK, "draft"},
		"a prerelease":                   {`{"tag_name":"v0.12.0","published_at":"2026-10-01T00:00:00Z","draft":false,"prerelease":true}`, http.StatusOK, "prerelease"},
		"no publication time":            {`{"tag_name":"v0.12.0","draft":false,"prerelease":false}`, http.StatusOK, "published_at"},
		"a publication time in future":   {release("v0.12.0", now.Add(48*time.Hour)), http.StatusOK, "future"},
		"a tag that is not a version":    {release("nightly", now.Add(-30*24*time.Hour)), http.StatusOK, "nightly"},
		"a tag with a path in it":        {release("v0.12.0/../x", now.Add(-30*24*time.Hour)), http.StatusOK, "v0.12.0/../x"},
		"a body that is not JSON":        {`<html>`, http.StatusOK, "JSON"},
		"a failing response":             {`{"message":"rate limited"}`, http.StatusForbidden, "403"},
		"a release with no assets named": {release("v0.12.0", now.Add(-30*24*time.Hour)), http.StatusOK, "v0.12.0"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			u := newUpstream(t, c.latest)
			u.status = c.status
			manifest := u.pinned()

			got, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want one naming %q", err, c.want)
			}
			if len(got.Changes) != 0 || len(got.Manifest.Tools) != 0 {
				t.Errorf("a refused bump returned %+v", got)
			}
			if manifest.Tools["shellcheck"].Version != "v0.11.0" {
				t.Error("the manifest passed in was changed")
			}
		})
	}
}

func TestAToolWithNoProvenanceRouteHereIsRefusedByName(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
	manifest := u.pinned()
	manifest.Tools["pinact"] = manifest.Tools["shellcheck"]

	_, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

	if err == nil || !strings.Contains(err.Error(), "pinact") || !strings.Contains(err.Error(), "provenance") {
		t.Errorf("error %v; a tool whose provenance route is not decided must not be bumped", err)
	}
}

func TestAnAssetNotOnTheReleasePageOfItsRepositoryIsRefused(t *testing.T) {
	for name, address := range map[string]string{
		"another path":     "/downloads/shellcheck-v0.11.0.linux.x86_64.tar.gz",
		"a query":          "/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.x86_64.tar.gz?x=1",
		"no version in it": "/koalaman/shellcheck/releases/download/latest/shellcheck.linux.x86_64.tar.gz",
	} {
		t.Run(name, func(t *testing.T) {
			u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
			u.offer(t, "v0.12.0")
			manifest := u.pinned()
			tool := manifest.Tools["shellcheck"]
			asset := tool.Assets["linux_amd64"]
			asset.URL = u.server.URL + address
			tool.Assets["linux_amd64"] = asset

			_, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

			if err == nil {
				t.Error("an asset that cannot be moved to the new version was accepted")
			}
		})
	}
}

func TestTheTokenGoesToTheReleaseLookupAndNowhereElse(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
	u.offer(t, "v0.12.0")

	if _, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options("a-token")); err != nil {
		t.Fatal(err)
	}

	for _, request := range u.requests() {
		header := request.Header.Get("Authorization")
		isLookup := strings.HasPrefix(request.URL.Path, "/repos/")
		if isLookup && header != "Bearer a-token" {
			t.Errorf("the lookup %s carried %q", request.URL.Path, header)
		}
		if !isLookup && header != "" {
			t.Errorf("the download %s carried a token: %q", request.URL.Path, header)
		}
	}
}

func TestAnAddressThatWouldSendTheTokenInTheClearIsRefused(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
	options := u.options("a-token")
	options.APIAddress = strings.Replace(u.server.URL, "https://", "http://", 1)

	_, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), options)

	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("error %v", err)
	}
	if n := len(u.requests()); n != 0 {
		t.Errorf("%d requests were made", n)
	}
}

func TestMarshalWritesTheEmbeddedManifestBackExactly(t *testing.T) {
	embedded, err := os.ReadFile("../toolchain/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := toolchain.Parse(embedded)
	if err != nil {
		t.Fatal(err)
	}

	written, err := bump.Marshal(manifest)

	if err != nil || !bytes.Equal(written, embedded) {
		t.Errorf("error %v; the writer must produce the file as it is kept, or an update would rewrite it whole:\n%s", err, written)
	}
}

func TestVersionsAreComparedAsNumbers(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.9.0", "v0.10.0", -1, true},
		{"v1.0.0", "v0.99.99", 1, true},
		{"v0.11.0", "v0.11.0", 0, true},
		{"0.11.0", "v0.11.0", 0, true},
		{"v1.2", "v1.2.0", 0, false},
		{"nightly", "v1.2.0", 0, false},
		{"v1.2.3-rc1", "v1.2.3", 0, false},
	}

	for _, c := range cases {
		got, ok := bump.Compare(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}
