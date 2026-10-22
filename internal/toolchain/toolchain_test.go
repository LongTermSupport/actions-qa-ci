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
	"syscall"
	"testing"
	"time"

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
	type refusal struct{ data, mention string }
	cases := map[string]refusal{
		"unknown field":          {strings.Replace(validManifest(), `"version"`, `"extra":1,"version"`, 1), "extra"},
		"trailing object":        {validManifest() + `{}`, "after its end"},
		"trailing closing brace": {validManifest() + `}`, "after its end"},
		"trailing closing array": {validManifest() + `]`, "after its end"},
		"not json":               {`nope`, "not valid"},
		"no tools":               {`{"tools":{}}`, "no tool"},
		"empty version":          {strings.Replace(validManifest(), `"v0.11.0"`, `""`, 1), "version"},
		"short archive hash":     {strings.Replace(validManifest(), `"archiveSha256":"`+hash, `"archiveSha256":"abc`, 1), "archiveSha256"},
		"upper case hash":        {strings.Replace(validManifest(), `"binarySha256":"`+hash, `"binarySha256":"`+strings.Repeat("A", 64), 1), "binarySha256"},
		"plain http":             {strings.Replace(validManifest(), "https://", "http://", 1), "https"},
		"https with no host":     {strings.Replace(validManifest(), "https://example.invalid/sc.tar.gz", "https:///sc.tar.gz", 1), "host"},
		"empty member":           {strings.Replace(validManifest(), member, `""`, 1), "member"},
		"member with ..":         {strings.Replace(validManifest(), member, `"../shellcheck"`, 1), "member"},
		"absolute member":        {strings.Replace(validManifest(), member, `"/shellcheck"`, 1), "member"},
		"member is a dot":        {strings.Replace(validManifest(), member, `"."`, 1), "member"},
		"no assets":              {`{"tools":{"shellcheck":{"version":"v0.11.0","assets":{}}}}`, "no asset"},
		"missing binary hash":    {strings.Replace(validManifest(), `,"binarySha256":"`+hash+`"`, ``, 1), "binarySha256"},
		"duplicate hash key":     {strings.Replace(validManifest(), `"binarySha256":"`+hash+`"`, `"binarySha256":"`+hash+`","binarySha256":"`+strings.Repeat("b", 64)+`"`, 1), "twice"},
		"duplicate platform":     {strings.Replace(validManifest(), `"linux_amd64":{`, `"linux_amd64":{},"linux_amd64":{`, 1), "twice"},
		"duplicate tool":         {strings.Replace(validManifest(), `{"tools":{`, `{"tools":{"shellcheck":{},`, 1), "twice"},
		"wrong-case field":       {strings.Replace(validManifest(), `"url"`, `"URL"`, 1), "URL"},
		"wrong-case top key":     {strings.Replace(validManifest(), `"tools"`, `"Tools"`, 1), "Tools"},
		"platform typo":          {strings.Replace(validManifest(), `linux_amd64`, `Linux-amd65`, 1), "Linux-amd65"},
		"platform not shipped":   {strings.Replace(validManifest(), `linux_amd64`, `linux_386`, 1), "linux_386"},
		"tool name with a slash": {strings.Replace(validManifest(), `"shellcheck":{"version"`, `"../evil":{"version"`, 1), "../evil"},
		"empty tool name":        {strings.Replace(validManifest(), `"shellcheck":{"version"`, `"":{"version"`, 1), "tool name"},
		"member with a NUL":      {strings.Replace(validManifest(), member, `"a\u0000b"`, 1), "member"},
		"blank version":          {strings.Replace(validManifest(), `"v0.11.0"`, `" "`, 1), "version"},
		"URL with credentials":   {strings.Replace(validManifest(), "https://example.invalid", "https://user:pw@example.invalid", 1), "credentials"},
		"upper case tool name":   {strings.Replace(validManifest(), `"shellcheck":{"version"`, `"ShellCheck":{"version"`, 1), "ShellCheck"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			manifest, err := toolchain.Parse([]byte(c.data))
			if err == nil {
				t.Fatalf("Parse accepted %s: %+v", name, manifest)
			}
			if !strings.Contains(err.Error(), c.mention) {
				t.Errorf("the error for %s must mention %q, got %q", name, c.mention, err.Error())
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
	type refusal struct {
		tamper   func(*toolchain.Asset)
		mismatch bool
		mention  string
	}
	cases := map[string]refusal{
		"archive hash": {func(a *toolchain.Asset) { a.ArchiveSHA256 = sum([]byte("other")) }, true, "archive"},
		"binary hash":  {func(a *toolchain.Asset) { a.BinarySHA256 = sum([]byte("other")) }, true, "shellcheck-v0.11.0/shellcheck"},
		"member name":  {func(a *toolchain.Asset) { a.Member = "shellcheck-v0.11.0/other" }, false, "no member"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			server, asset := fetchFixture(t, c.tamper)
			dir := t.TempDir()

			path, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, "shellcheck")
			if err == nil {
				t.Fatalf("Fetch accepted a wrong %s and wrote %s", name, path)
			}
			if errors.Is(err, toolchain.ErrMismatch) != c.mismatch || !strings.Contains(err.Error(), c.mention) {
				t.Errorf("the refusal for a wrong %s must be a mismatch=%v mentioning %q, got %v", name, c.mismatch, c.mention, err)
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

func TestVerifyRefusesABinaryThatIsNotExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shellcheck")
	if err := os.WriteFile(path, []byte(pinnedBinary), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := toolchain.Verify(path, toolchain.Asset{BinarySHA256: sum([]byte(pinnedBinary))})
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Errorf("a pinned binary that cannot run must be refused by name, got %v", err)
	}
}

func TestVerifyDoesNotBlockOnAFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shellcheck")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- toolchain.Verify(path, toolchain.Asset{BinarySHA256: sum(nil)}) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("a FIFO must be refused as not a regular file, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Verify blocked on a FIFO instead of refusing it")
	}
}

func TestFetchAllNeverDestroysAFileThatWasAlreadyThere(t *testing.T) {
	server, good := fetchFixture(t, nil)
	entry := `{"url":"` + good.URL + `","archiveSha256":"` + good.ArchiveSHA256 + `",` +
		`"member":"` + good.Member + `","binarySha256":"` + good.BinarySHA256 + `"}`
	data := `{"tools":{` +
		`"alpha":{"version":"v1","assets":{"linux_amd64":` + entry + `}},` +
		`"beta":{"version":"v1","assets":{"linux_amd64":` + entry + `}}}}`
	manifest, err := toolchain.Parse([]byte(data))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	dir := t.TempDir()
	earlier := filepath.Join(dir, "alpha")
	if err := os.WriteFile(earlier, []byte("an earlier alpha"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "beta"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	paths, err := toolchain.FetchAll(context.Background(), server.Client(), manifest, "linux_amd64", dir)
	if err == nil {
		t.Fatalf("FetchAll wrote over a directory: %v", paths)
	}
	kept, readErr := os.ReadFile(earlier)
	if readErr != nil || string(kept) != "an earlier alpha" {
		t.Errorf("the file that was already there was changed or removed: %q, %v", kept, readErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("cannot read the destination: %v", readErr)
	}
	if len(entries) != 2 {
		t.Errorf("staged files were left behind: %d entries", len(entries))
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

func TestVerifyRefusesASymbolicLinkEvenToTheRightContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte(pinnedBinary), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	link := filepath.Join(dir, "shellcheck")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := toolchain.Verify(link, toolchain.Asset{BinarySHA256: sum([]byte(pinnedBinary))})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a symbolic link must be refused by name, got %v", err)
	}
}

func TestResolveStopsAtADanglingSymbolicLinkAndSaysSo(t *testing.T) {
	manifest := resolveFixture(t)
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "absent"), filepath.Join(dir, "shellcheck")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", []string{dir})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a dangling link must stop the search and be named, got %q and %v", got, err)
	}
}

func TestResolveRefusesADirectoryThatIsEmptyOrRelative(t *testing.T) {
	manifest := resolveFixture(t)

	for _, dir := range []string{"", ".", "bin", "../bin"} {
		got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", []string{dir})
		if err == nil || !strings.Contains(err.Error(), "absolute") {
			t.Errorf("directory %q must be refused as not absolute, got %q and %v", dir, got, err)
		}
	}
}

func TestResolveWithNoDirectoryToLookInIsNotFound(t *testing.T) {
	manifest := resolveFixture(t)

	got, err := toolchain.Resolve(manifest, "shellcheck", "linux_amd64", nil)
	if !errors.Is(err, toolchain.ErrNotFound) {
		t.Errorf("no directory must be reported as not found, got %q and %v", got, err)
	}
}

func TestFetchRefusesANameThatIsNotAPlainToolName(t *testing.T) {
	server, asset := fetchFixture(t, nil)

	for _, name := range []string{"", "../x", "a/b", ".hidden", "Shell"} {
		dir := t.TempDir()

		path, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, name)
		if err == nil {
			t.Errorf("name %q was accepted and wrote %s", name, path)
		}
		entries, readErr := os.ReadDir(dir)
		if readErr != nil || len(entries) != 0 {
			t.Errorf("name %q left %d entries behind (%v)", name, len(entries), readErr)
		}
	}
}

func TestFetchAllWritesNothingWhenALaterToolFails(t *testing.T) {
	server, good := fetchFixture(t, nil)
	bad := good
	bad.ArchiveSHA256 = sum([]byte("other"))
	entry := func(asset toolchain.Asset) string {
		return `{"url":"` + good.URL + `","archiveSha256":"` + asset.ArchiveSHA256 + `",` +
			`"member":"` + asset.Member + `","binarySha256":"` + asset.BinarySHA256 + `"}`
	}
	data := `{"tools":{` +
		`"alpha":{"version":"v1","assets":{"linux_amd64":` + entry(good) + `}},` +
		`"beta":{"version":"v1","assets":{"linux_amd64":` + entry(bad) + `}}}}`
	manifest, err := toolchain.Parse([]byte(data))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	dir := t.TempDir()

	paths, err := toolchain.FetchAll(context.Background(), server.Client(), manifest, "linux_amd64", dir)
	if err == nil {
		t.Fatalf("FetchAll accepted a tool that fails its pin: %v", paths)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("cannot read the destination: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("a failed FetchAll left %d entries behind", len(entries))
	}
}

func TestFetchRefusesAnArchiveThatExpandsBeyondTheBoundBeforeReachingTheMember(t *testing.T) {
	const junk = 300 << 20
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "junk", Mode: 0o644, Size: junk, Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	zeros := make([]byte, 1<<20)
	for written := 0; written < junk; written += len(zeros) {
		n, err := tw.Write(zeros)
		if err != nil || n != len(zeros) {
			t.Fatalf("setup: wrote %d, %v", n, err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: "m", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	n, err := tw.Write([]byte("x"))
	if err != nil || n != 1 {
		t.Fatalf("setup: wrote %d, %v", n, err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	body := buffer.Bytes()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		written, writeErr := w.Write(body)
		if writeErr != nil || written != len(body) {
			t.Errorf("test server cannot write: %v", writeErr)
		}
	}))
	t.Cleanup(server.Close)
	asset := toolchain.Asset{URL: server.URL + "/a.tar.gz", ArchiveSHA256: sum(body), Member: "m", BinarySHA256: sum([]byte("x"))}

	path, fetchErr := toolchain.Fetch(context.Background(), server.Client(), asset, t.TempDir(), "tool")
	if fetchErr == nil || !strings.Contains(fetchErr.Error(), "larger") {
		t.Errorf("an archive that expands without bound must be refused as too large, got %q and %v", path, fetchErr)
	}
}
