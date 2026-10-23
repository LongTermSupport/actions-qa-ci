package bump_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// withBoth returns the manifest with the address of each of its assets changed.
func withBoth(manifest toolchain.Manifest, change func(address string) string) toolchain.Manifest {
	tool := manifest.Tools["shellcheck"]
	for platform, asset := range tool.Assets {
		asset.URL = change(asset.URL)
		tool.Assets[platform] = asset
	}

	return manifest
}

// withAmd64 returns the manifest with the address of its amd64 asset changed.
func withAmd64(manifest toolchain.Manifest, change func(address string) string) toolchain.Manifest {
	tool := manifest.Tools["shellcheck"]
	asset := tool.Assets["linux_amd64"]
	asset.URL = change(asset.URL)
	tool.Assets["linux_amd64"] = asset

	return manifest
}

func TestAnAssetThatIsNotAPlainReleaseDownloadOfThePinnedVersionIsRefusedBeforeAnyRequest(t *testing.T) {
	good := "/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.x86_64.tar.gz"
	cases := map[string]func(server string) string{
		"plain http":           func(server string) string { return strings.Replace(server, "https://", "http://", 1) + good },
		"a scheme of its own":  func(server string) string { return strings.Replace(server, "https://", "ftp://", 1) + good },
		"no host":              func(string) string { return "https://" + good },
		"an empty host":        func(string) string { return "https:///" + strings.TrimPrefix(good, "/") },
		"credentials":          func(server string) string { return strings.Replace(server, "https://", "https://user:pw@", 1) + good },
		"a fragment":           func(server string) string { return server + good + "#x" },
		"a query":              func(server string) string { return server + good + "?x=1" },
		"an owner with a dash": func(server string) string { return server + strings.Replace(good, "/koalaman/", "/-koalaman/", 1) },
		"a repository hidden":  func(server string) string { return server + strings.Replace(good, "/shellcheck/", "/.shellcheck/", 1) },
		"another word for releases": func(server string) string {
			return server + strings.Replace(good, "/releases/", "/archive/", 1)
		},
		"another word for download": func(server string) string {
			return server + strings.Replace(good, "/download/", "/latest/", 1)
		},
		"another tag than the pin": func(server string) string {
			return server + strings.Replace(good, "/download/v0.11.0/", "/download/v0.10.0/", 1)
		},
		"a file without the version": func(server string) string {
			return server + strings.Replace(good, "shellcheck-v0.11.0.linux", "shellcheck.linux", 1)
		},
		"a path one segment short": func(server string) string { return server + "/koalaman/releases/download/v0.11.0/x-v0.11.0.tar.gz" },
		"a path one segment long":  func(server string) string { return server + "/a" + good },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
			u.offer(t, "v0.12.0")
			manifest := withBoth(u.pinned(), func(string) string { return change(u.server.URL) })

			got, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

			if err == nil {
				t.Errorf("an asset the update cannot move safely was accepted: %+v", got.Manifest)
			}
			if n := len(u.requests()); n != 0 {
				t.Errorf("%d requests were made before the asset was judged", n)
			}
		})
	}
}

func TestTheAssetsOfOneToolMustComeFromOneRepository(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
	manifest := withAmd64(u.pinned(), func(address string) string {
		return strings.Replace(address, "/koalaman/", "/someone-else/", 1)
	})

	_, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

	if err == nil || !strings.Contains(err.Error(), "two repositories") {
		t.Errorf("error %v", err)
	}
}

func TestOnlyTheTagAndTheFileNameMoveWhateverElseHoldsTheVersion(t *testing.T) {
	var mu sync.Mutex
	var downloads []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/v0.11.0/shellcheck/releases/latest":
			if _, err := w.Write([]byte(release("v0.12.0", now.Add(-30*24*time.Hour)))); err != nil {
				t.Error(err)
			}
		case strings.HasPrefix(r.URL.Path, "/v0.11.0/shellcheck/releases/download/v0.12.0/"):
			mu.Lock()
			downloads = append(downloads, r.URL.Path)
			mu.Unlock()
			if _, err := w.Write(tarball(t, "shellcheck-v0.12.0/shellcheck", "new")); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	asset := func(arch string) toolchain.Asset {
		return toolchain.Asset{
			URL:           server.URL + "/v0.11.0/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux." + arch + ".tar.gz",
			ArchiveSHA256: strings.Repeat("a", 64),
			Member:        "shellcheck-v0.11.0/shellcheck",
			BinarySHA256:  strings.Repeat("b", 64),
		}
	}
	manifest := toolchain.Manifest{Tools: map[string]toolchain.Tool{
		"shellcheck": {Version: "v0.11.0", Assets: map[string]toolchain.Asset{"linux_amd64": asset("x86_64"), "linux_arm64": asset("aarch64")}},
	}}

	got, err := bump.Run(context.Background(), server.Client(), manifest, bump.Options{APIAddress: server.URL, Cooldown: 72 * time.Hour, Now: now})
	if err != nil {
		t.Fatal(err)
	}

	want := server.URL + "/v0.11.0/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.linux.x86_64.tar.gz"
	if address := got.Manifest.Tools["shellcheck"].Assets["linux_amd64"].URL; address != want {
		t.Errorf("address %q, want %q; the owner and the repository must not change with the version", address, want)
	}
	if len(downloads) != 2 {
		t.Errorf("downloads %v", downloads)
	}
}

func TestTheLookupDoesNotFollowARedirectSoTheTokenCannotLeaveWithIt(t *testing.T) {
	var mu sync.Mutex
	var leaked []string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		leaked = append(leaked, r.Header.Get("Authorization"))
		mu.Unlock()
	}))
	t.Cleanup(elsewhere.Close)
	redirecting := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)
	u := newUpstream(t, "")
	options := u.options("SECRET")
	options.APIAddress = redirecting.URL

	_, err := bump.Run(context.Background(), redirecting.Client(), u.pinned(), options)

	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Errorf("error %v; a redirect on the lookup must be refused", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(leaked) != 0 {
		t.Errorf("the redirect target was reached: %q", leaked)
	}
}

func TestAnAddressWithCredentialsOrNoHostIsNotAnAPIAddress(t *testing.T) {
	for _, address := range []string{"https://user:pw@api.example", "https://user@api.example", "https:///x", "https://", "HTTP://api.example", "api.example", ""} {
		u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
		options := u.options("a-token")
		options.APIAddress = address

		if _, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), options); err == nil {
			t.Errorf("the API address %q was accepted", address)
		}
		if n := len(u.requests()); n != 0 {
			t.Errorf("%q: %d requests were made", address, n)
		}
	}
}

func TestAnAnswerLargerThanTheCapIsRefusedNotRead(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour))+strings.Repeat(" ", 2<<20))

	_, err := bump.Run(context.Background(), u.server.Client(), u.pinned(), u.options(""))

	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("error %v", err)
	}
}

func TestARunThatFailsOnALaterToolReturnsNothingOfTheEarlierOne(t *testing.T) {
	u := newUpstream(t, release("v0.12.0", now.Add(-30*24*time.Hour)))
	u.offer(t, "v0.12.0")
	manifest := u.pinned()
	manifest.Tools["zizmor"] = manifest.Tools["shellcheck"]

	got, err := bump.Run(context.Background(), u.server.Client(), manifest, u.options(""))

	if err == nil || !strings.Contains(err.Error(), "zizmor") {
		t.Fatalf("error %v", err)
	}
	if len(got.Changes) != 0 || len(got.Manifest.Tools) != 0 || len(got.Notes) != 0 {
		t.Errorf("a failed run returned %+v; shellcheck was bumped before zizmor was refused", got)
	}
}

func TestMarshalLeavesAmpersandsAndAngleBracketsAsTheyAre(t *testing.T) {
	written, err := bump.Marshal(toolchain.Manifest{Tools: map[string]toolchain.Tool{
		"tool": {Version: "v1.0.0", Assets: map[string]toolchain.Asset{"linux_amd64": {
			URL: "https://example.com/a?b=1&c=<2>", ArchiveSHA256: strings.Repeat("a", 64), Member: "m", BinarySHA256: strings.Repeat("b", 64),
		}}},
	}})

	if err != nil || !strings.Contains(string(written), "a?b=1&c=<2>") || strings.Contains(string(written), "\\"+"u0026") {
		t.Errorf("error %v, written:\n%s", err, written)
	}
}
