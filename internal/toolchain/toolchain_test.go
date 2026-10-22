package toolchain_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// pinnedBinary is the content the fixtures pin; any other content is a tampered binary.
const pinnedBinary = "the real binary"

func sum(data []byte) string {
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

// archive builds a gzipped tar holding one member.
func archive(t *testing.T, member string, content []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: member, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("cannot write the tar header: %v", err)
	}
	written, err := tw.Write(content)
	if err != nil || written != len(content) {
		t.Fatalf("cannot write the tar member: wrote %d of %d, %v", written, len(content), err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("cannot close the tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("cannot close the gzip: %v", err)
	}

	return buffer.Bytes()
}

func validManifest() string {
	hash := strings.Repeat("a", 64)

	return `{"tools":{"shellcheck":{"version":"v0.11.0","assets":{"linux_amd64":{` +
		`"url":"https://example.invalid/sc.tar.gz","archiveSha256":"` + hash + `",` +
		`"member":"shellcheck-v0.11.0/shellcheck","binarySha256":"` + hash + `"}}}}}`
}

func TestTheEmbeddedManifestPinsShellCheckForBothLinuxArchitectures(t *testing.T) {
	manifest, err := toolchain.Load()
	if err != nil {
		t.Fatalf("the embedded manifest must load: %v", err)
	}

	for _, platform := range []string{"linux_amd64", "linux_arm64"} {
		tool, asset, err := manifest.Lookup("shellcheck", platform)
		if err != nil {
			t.Fatalf("shellcheck must be pinned for %s: %v", platform, err)
		}
		if tool.Version != "v0.11.0" {
			t.Errorf("%s: version = %q, want v0.11.0", platform, tool.Version)
		}
		if !strings.HasPrefix(asset.URL, "https://github.com/koalaman/shellcheck/releases/download/v0.11.0/") {
			t.Errorf("%s: URL %q is not the pinned release", platform, asset.URL)
		}
	}
}

func TestParseAcceptsAWellFormedManifest(t *testing.T) {
	manifest, err := toolchain.Parse([]byte(validManifest()))
	if err != nil {
		t.Fatalf("a well-formed manifest must parse: %v", err)
	}

	tool, asset, err := manifest.Lookup("shellcheck", "linux_amd64")
	if err != nil {
		t.Fatalf("the pinned asset must be found: %v", err)
	}
	if tool.Version == "" || asset.URL == "" {
		t.Errorf("the pinned asset came back empty: %+v %+v", tool, asset)
	}
}

func TestParseRefusesWhatIsNotAWellFormedManifest(t *testing.T) {
	hash := strings.Repeat("a", 64)
	member := `"shellcheck-v0.11.0/shellcheck"`
	cases := map[string]string{
		"unknown field":       strings.Replace(validManifest(), `"version"`, `"extra":1,"version"`, 1),
		"trailing data":       validManifest() + `{}`,
		"not json":            `nope`,
		"no tools":            `{"tools":{}}`,
		"empty version":       strings.Replace(validManifest(), `"v0.11.0"`, `""`, 1),
		"short archive hash":  strings.Replace(validManifest(), `"archiveSha256":"`+hash, `"archiveSha256":"abc`, 1),
		"upper case hash":     strings.Replace(validManifest(), `"binarySha256":"`+hash, `"binarySha256":"`+strings.Repeat("A", 64), 1),
		"plain http":          strings.Replace(validManifest(), "https://", "http://", 1),
		"empty member":        strings.Replace(validManifest(), member, `""`, 1),
		"member with ..":      strings.Replace(validManifest(), member, `"../shellcheck"`, 1),
		"absolute member":     strings.Replace(validManifest(), member, `"/shellcheck"`, 1),
		"no assets":           `{"tools":{"shellcheck":{"version":"v0.11.0","assets":{}}}}`,
		"missing binary hash": strings.Replace(validManifest(), `,"binarySha256":"`+hash+`"`, ``, 1),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			manifest, err := toolchain.Parse([]byte(data))
			if err == nil {
				t.Fatalf("Parse accepted %s: %+v", name, manifest)
			}
		})
	}
}

func TestLookupNamesWhatIsNotPinned(t *testing.T) {
	manifest, err := toolchain.Parse([]byte(validManifest()))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tool, asset, err := manifest.Lookup("zizmor", "linux_amd64")
	if err == nil || !strings.Contains(err.Error(), "zizmor") {
		t.Errorf("an unpinned tool must be named in the error, got %v (%+v %+v)", err, tool, asset)
	}
	tool, asset, err = manifest.Lookup("shellcheck", "darwin_arm64")
	if err == nil || !strings.Contains(err.Error(), "darwin_arm64") {
		t.Errorf("an unpinned platform must be named in the error, got %v (%+v %+v)", err, tool, asset)
	}
}

func TestVerifyAcceptsTheBinaryTheManifestPins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shellcheck")
	if err := os.WriteFile(path, []byte(pinnedBinary), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := toolchain.Verify(path, toolchain.Asset{BinarySHA256: sum([]byte(pinnedBinary))}); err != nil {
		t.Fatalf("a binary that matches its pin must verify: %v", err)
	}
}

func TestVerifyRefusesAnyOtherBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shellcheck")
	if err := os.WriteFile(path, []byte("a swapped binary"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := toolchain.Verify(path, toolchain.Asset{BinarySHA256: sum([]byte(pinnedBinary))})
	if !errors.Is(err, toolchain.ErrMismatch) {
		t.Fatalf("a different binary must be refused as a mismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error must name the file, got %q", err.Error())
	}
}

func TestVerifyRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()

	if err := toolchain.Verify(dir, toolchain.Asset{BinarySHA256: sum(nil)}); err == nil {
		t.Error("a directory must not verify")
	}
	if err := toolchain.Verify(filepath.Join(dir, "absent"), toolchain.Asset{BinarySHA256: sum(nil)}); !errors.Is(err, toolchain.ErrNotFound) {
		t.Errorf("a missing file must be reported as not found, got %v", err)
	}
}

func resolveFixture(t *testing.T) toolchain.Manifest {
	t.Helper()

	data := strings.ReplaceAll(validManifest(), strings.Repeat("a", 64), sum([]byte(pinnedBinary)))
	manifest, err := toolchain.Parse([]byte(data))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	return manifest
}

func TestResolveReturnsTheVerifiedBinaryFromTheFirstDirectoryHoldingOne(t *testing.T) {
	manifest := resolveFixture(t)
	empty, holder := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(holder, "shellcheck"), []byte(pinnedBinary), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", []string{empty, holder})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(holder, "shellcheck"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveNeverFallsThroughPastABinaryThatFailsVerification(t *testing.T) {
	manifest := resolveFixture(t)
	tampered, good := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(tampered, "shellcheck"), []byte("tampered"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(good, "shellcheck"), []byte(pinnedBinary), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", []string{tampered, good})
	if !errors.Is(err, toolchain.ErrMismatch) {
		t.Fatalf("a tampered binary must stop the search with a mismatch, got path %q and error %v", got, err)
	}
}

func TestResolveNamesEveryDirectoryItLookedIn(t *testing.T) {
	manifest := resolveFixture(t)
	first, second := t.TempDir(), t.TempDir()

	got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", []string{first, second})
	if !errors.Is(err, toolchain.ErrNotFound) {
		t.Fatalf("no binary anywhere must be reported as not found, got %q and %v", got, err)
	}
	for _, dir := range []string{first, second} {
		if !strings.Contains(err.Error(), dir) {
			t.Errorf("the error must name %s, got %q", dir, err.Error())
		}
	}
}

func TestResolveRefusesAToolThatIsNotPinnedForThePlatform(t *testing.T) {
	manifest := resolveFixture(t)

	got, err := toolchain.Resolve(manifest, "shellcheck", "plan9_386", []string{t.TempDir()})
	if err == nil {
		t.Errorf("an unpinned platform must be refused, got %q", got)
	}
}

func fetchFixture(t *testing.T, tamper func(*toolchain.Asset)) (*httptest.Server, toolchain.Asset) {
	t.Helper()

	const member = "shellcheck-v0.11.0/shellcheck"
	body := archive(t, member, []byte(pinnedBinary))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		written, err := w.Write(body)
		if err != nil || written != len(body) {
			t.Errorf("test server cannot write: wrote %d of %d, %v", written, len(body), err)
		}
	}))
	t.Cleanup(server.Close)

	asset := toolchain.Asset{
		URL:           server.URL + "/shellcheck.tar.gz",
		ArchiveSHA256: sum(body),
		Member:        member,
		BinarySHA256:  sum([]byte(pinnedBinary)),
	}
	if tamper != nil {
		tamper(&asset)
	}

	return server, asset
}

func TestFetchWritesTheVerifiedExecutable(t *testing.T) {
	server, asset := fetchFixture(t, nil)
	dir := t.TempDir()

	path, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, "shellcheck")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := filepath.Join(dir, "shellcheck"); path != want {
		t.Errorf("Fetch = %q, want %q", path, want)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the fetched binary: %v", err)
	}
	if !bytes.Equal(written, []byte(pinnedBinary)) {
		t.Error("the fetched binary differs from the archive member")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot stat the fetched binary: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the fetched binary is not executable: %v", info.Mode())
	}
}

func TestFetchWritesNothingWhenAHashDoesNotMatch(t *testing.T) {
	cases := map[string]func(*toolchain.Asset){
		"archive hash": func(a *toolchain.Asset) { a.ArchiveSHA256 = sum([]byte("other")) },
		"binary hash":  func(a *toolchain.Asset) { a.BinarySHA256 = sum([]byte("other")) },
		"member name":  func(a *toolchain.Asset) { a.Member = "shellcheck-v0.11.0/other" },
	}

	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			server, asset := fetchFixture(t, tamper)
			dir := t.TempDir()

			path, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, "shellcheck")
			if err == nil {
				t.Fatalf("Fetch accepted a wrong %s and wrote %s", name, path)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatalf("cannot read the destination: %v", readErr)
			}
			if len(entries) != 0 {
				t.Errorf("a refused fetch left %d entries behind", len(entries))
			}
		})
	}
}

func TestFetchRefusesAFailingResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	asset := toolchain.Asset{URL: server.URL + "/x", ArchiveSHA256: sum(nil), Member: "m", BinarySHA256: sum(nil)}
	path, err := toolchain.Fetch(context.Background(), server.Client(), asset, t.TempDir(), "shellcheck")
	if err == nil {
		t.Errorf("a 404 must be an error, got %q", path)
	}
}

func TestFetchAllPlacesEveryToolPinnedForThePlatform(t *testing.T) {
	server, asset := fetchFixture(t, nil)
	data := `{"tools":{"shellcheck":{"version":"v0.11.0","assets":{"linux_amd64":{` +
		`"url":"` + asset.URL + `","archiveSha256":"` + asset.ArchiveSHA256 + `",` +
		`"member":"` + asset.Member + `","binarySha256":"` + asset.BinarySHA256 + `"}}}}}`
	manifest, err := toolchain.Parse([]byte(data))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	dir := t.TempDir()

	paths, err := toolchain.FetchAll(context.Background(), server.Client(), manifest, "linux_amd64", dir)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if want := []string{filepath.Join(dir, "shellcheck")}; len(paths) != 1 || paths[0] != want[0] {
		t.Errorf("FetchAll = %v, want %v", paths, want)
	}
	if err := toolchain.Verify(paths[0], asset); err != nil {
		t.Errorf("the placed binary must verify: %v", err)
	}
}

func TestFetchAllRefusesAPlatformNoToolIsPinnedFor(t *testing.T) {
	manifest := resolveFixture(t)

	paths, err := toolchain.FetchAll(context.Background(), http.DefaultClient, manifest, "plan9_386", t.TempDir())
	if err == nil {
		t.Errorf("an unpinned platform must be refused, got %v", paths)
	}
}
