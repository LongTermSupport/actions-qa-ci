package main

import (
	"bytes"
	"strings"
	"testing"
)

type result struct {
	code           int
	stdout, stderr string
}

func invoke(args ...string) result {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestNoCommandPrintsUsageAndExitsTwo(t *testing.T) {
	got := invoke()

	if got.code != 2 || !strings.Contains(got.stderr, "fetch <directory>") {
		t.Errorf("got %+v, want exit 2 and the usage on stderr", got)
	}
}

func TestAnUnknownCommandIsNamedAndExitsTwo(t *testing.T) {
	got := invoke("bump")

	if got.code != 2 || !strings.Contains(got.stderr, "bump") {
		t.Errorf("got %+v, want exit 2 naming the command", got)
	}
}

func TestFetchNeedsExactlyOneDirectory(t *testing.T) {
	for _, args := range [][]string{{"fetch"}, {"fetch", "a", "b"}} {
		got := invoke(args...)

		if got.code != 2 || !strings.Contains(got.stderr, "fetch <directory>") {
			t.Errorf("%v: got %+v, want exit 2 and the usage on stderr", args, got)
		}
	}
}
