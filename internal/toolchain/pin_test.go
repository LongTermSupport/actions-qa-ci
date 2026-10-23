package toolchain_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

func pinServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestPinHashesTheArchiveAndTheMemberTakenOutOfIt(t *testing.T) {
	tarball := archive(t, "tool-1/tool", []byte(pinnedBinary))
	server := pinServer(t, http.StatusOK, tarball)

	asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/tool.tar.gz", "tool-1/tool")
	if err != nil {
		t.Fatal(err)
	}

	want := toolchain.Asset{
		URL:           server.URL + "/tool.tar.gz",
		ArchiveSHA256: sum(tarball),
		Member:        "tool-1/tool",
		BinarySHA256:  sum([]byte(pinnedBinary)),
	}
	if asset != want {
		t.Errorf("asset = %+v, want %+v", asset, want)
	}
}

func TestPinRefusesWhatCannotBePinned(t *testing.T) {
	tarball := archive(t, "tool-1/tool", []byte(pinnedBinary))
	cases := map[string]struct {
		status int
		body   []byte
		member string
		want   string
	}{
		"a failing response":         {http.StatusNotFound, tarball, "tool-1/tool", "404"},
		"a member that is not there": {http.StatusOK, tarball, "tool-1/other", "no member"},
		"a body that is not gzip":    {http.StatusOK, []byte("plain text"), "tool-1/tool", "gzip"},
		"an unsafe member":           {http.StatusOK, tarball, "../tool", "relative path"},
		"an absolute member":         {http.StatusOK, tarball, "/tool", "relative path"},
		"an empty member":            {http.StatusOK, tarball, "", "relative path"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			server := pinServer(t, c.status, c.body)

			asset, err := toolchain.Pin(context.Background(), server.Client(), server.URL+"/tool.tar.gz", c.member)

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("asset %+v, error %v, want an error naming %q", asset, err, c.want)
			}
			if asset != (toolchain.Asset{}) {
				t.Errorf("a refused pin returned %+v", asset)
			}
		})
	}
}
