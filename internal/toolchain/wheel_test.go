package toolchain_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

const wheelMember = "zizmor-1.0.0.data/scripts/zizmor"

type wheelEntry struct {
	name    string
	content []byte
	mode    fs.FileMode
}

// wheel builds a zip, the form a Python wheel is distributed in, from the entries in order.
func wheel(t *testing.T, entries ...wheelEntry) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(entry.mode)
		member, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("cannot start the zip entry %s: %v", entry.name, err)
		}
		if written, err := member.Write(entry.content); err != nil || written != len(entry.content) {
			t.Fatalf("cannot write the zip entry %s: wrote %d of %d, %v", entry.name, written, len(entry.content), err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("cannot close the zip: %v", err)
	}

	return buffer.Bytes()
}

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if written, err := w.Write(body); err != nil || written != len(body) {
			t.Errorf("test server cannot write: wrote %d of %d, %v", written, len(body), err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestAWheelIsPinnedAndFetchedLikeAnyOtherArchive(t *testing.T) {
	body := wheel(t,
		wheelEntry{"zizmor-1.0.0.dist-info/METADATA", []byte("Name: zizmor\n"), 0o644},
		wheelEntry{wheelMember, []byte(pinnedBinary), 0o755},
	)
	server := serve(t, body)

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/zizmor-1.0.0-py3-none-musllinux_1_2_x86_64.whl", wheelMember)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ArchiveSHA256 != sum(body) || asset.BinarySHA256 != sum([]byte(pinnedBinary)) || asset.Member != wheelMember {
		t.Fatalf("asset = %+v", asset)
	}

	dir := t.TempDir()
	path, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, "zizmor")
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil || string(written) != pinnedBinary {
		t.Errorf("the fetched binary is %q, %v", written, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode %v, %v; the fetched binary must be executable whatever mode the wheel records", info, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("entries %v, %v", entries, err)
	}
}

func TestAWheelWhoseBytesDifferFromThePinIsRefused(t *testing.T) {
	body := wheel(t, wheelEntry{wheelMember, []byte(pinnedBinary), 0o755})
	server := serve(t, body)
	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", wheelMember)
	if err != nil {
		t.Fatal(err)
	}
	asset.BinarySHA256 = strings.Repeat("0", 64)

	path, err := toolchain.Fetch(context.Background(), server.Client(), asset, t.TempDir(), "zizmor")

	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("want a mismatch, got %q and %v", path, err)
	}
}

func TestAWheelThatCannotGiveUpItsMemberIsRefused(t *testing.T) {
	cases := map[string]struct {
		body   []byte
		member string
		want   string
	}{
		"a member that is not there": {wheel(t, wheelEntry{"other", []byte("x"), 0o644}), wheelMember, "no member"},
		"a member whose mode says directory": {
			wheel(t, wheelEntry{wheelMember, []byte("x"), fs.ModeDir | 0o755}), wheelMember, "not a regular file",
		},
		"a member named as a directory": {
			wheel(t, wheelEntry{wheelMember + "/", nil, fs.ModeDir | 0o755}), wheelMember + "/", "relative path",
		},
		"a member that is a symbolic link": {
			wheel(t, wheelEntry{wheelMember, []byte("/etc/passwd"), fs.ModeSymlink | 0o777}), wheelMember, "not a regular file",
		},
		"a member named twice": {
			wheel(t, wheelEntry{wheelMember, []byte("first"), 0o755}, wheelEntry{wheelMember, []byte("second"), 0o755}), wheelMember, "more than once",
		},
		"bytes that begin like a zip and are not one": {append([]byte("PK\x03\x04"), []byte("garbage that is not a zip archive at all")...), wheelMember, "not a valid zip"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			server := serve(t, c.body)

			asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", c.member)

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("asset %+v, error %v, want an error naming %q", asset, err, c.want)
			}
			if asset != (toolchain.Asset{}) {
				t.Errorf("a refused pin returned %+v", asset)
			}
		})
	}
}

func TestAWheelMemberLargerThanTheBoundIsRefusedWithoutBeingExpandedInMemory(t *testing.T) {
	const tooBig = 129 << 20
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	member, err := writer.CreateHeader(&zip.FileHeader{Name: wheelMember, Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	zeros := make([]byte, 1<<20)
	for written := 0; written < tooBig; written += len(zeros) {
		if n, err := member.Write(zeros); err != nil || n != len(zeros) {
			t.Fatalf("setup: wrote %d, %v", n, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := serve(t, buffer.Bytes())

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", wheelMember)

	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("asset %+v, error %v; want it refused as too large", asset, err)
	}
}

// The directory of a zip is read into memory entry by entry, so what a zip may hold is bounded by
// its size and its entry count before anything is looked for in it.
func TestAZipLargerThanItsBoundIsRefusedBeforeItsDirectoryIsRead(t *testing.T) {
	const tooBig = 33 << 20
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	member, err := writer.CreateHeader(&zip.FileHeader{Name: wheelMember, Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := member.Write(make([]byte, tooBig)); err != nil || n != tooBig {
		t.Fatalf("setup: wrote %d, %v", n, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := serve(t, buffer.Bytes())

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", wheelMember)

	if err == nil || !strings.Contains(err.Error(), "zip larger than") {
		t.Errorf("asset %+v, error %v; want it refused as a zip that is too large", asset, err)
	}
}

func TestAZipWithMoreEntriesThanItsBoundIsRefused(t *testing.T) {
	entries := make([]wheelEntry, 0, 10_002)
	for i := 0; i < 10_001; i++ {
		entries = append(entries, wheelEntry{name: "f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + strings.Repeat("y", i/26%50) + string(rune('0'+i%10)), mode: 0o644})
	}
	entries = append(entries, wheelEntry{wheelMember, []byte(pinnedBinary), 0o755})
	server := serve(t, wheel(t, entries...))

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", wheelMember)

	if err == nil || !strings.Contains(err.Error(), "entries") {
		t.Errorf("asset %+v, error %v; want it refused for its entry count", asset, err)
	}
}

func TestAZipWithAsManyEntriesAsItsBoundIsRead(t *testing.T) {
	entries := make([]wheelEntry, 0, 10_000)
	for i := 0; i < 9_999; i++ {
		entries = append(entries, wheelEntry{name: "d/" + string(rune('a'+i%26)) + strings.Repeat("z", i/26), mode: 0o644})
	}
	entries = append(entries, wheelEntry{wheelMember, []byte(pinnedBinary), 0o755})
	server := serve(t, wheel(t, entries...))

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z.whl", wheelMember)

	if err != nil || asset.BinarySHA256 != sum([]byte(pinnedBinary)) {
		t.Errorf("asset %+v, error %v", asset, err)
	}
}

func TestAnArchiveThatIsNeitherGzipNorZipIsStillRefusedAsNotGzip(t *testing.T) {
	server := serve(t, []byte("plain text"))

	_, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/z", wheelMember)

	if err == nil || !strings.Contains(err.Error(), "gzip") {
		t.Errorf("got %v", err)
	}
}

func TestAWheelFetchedIntoADirectoryLeavesNothingElseBehindOnFailure(t *testing.T) {
	body := wheel(t, wheelEntry{"other", []byte("x"), 0o644})
	server := serve(t, body)
	dir := t.TempDir()
	asset := toolchain.Asset{URL: server.URL + "/z.whl", ArchiveSHA256: sum(body), Member: wheelMember, BinarySHA256: strings.Repeat("1", 64)}

	_, err := toolchain.Fetch(context.Background(), server.Client(), asset, dir, "zizmor")

	if err == nil {
		t.Fatal("want an error")
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Errorf("entries %v, %v", entries, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "zizmor")); statErr == nil {
		t.Error("a binary was left behind")
	}
}
