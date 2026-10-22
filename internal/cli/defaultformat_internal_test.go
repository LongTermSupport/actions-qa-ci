package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTheDefaultFormatIsTheEnvironmentsThenTextForATerminalThenAgent(t *testing.T) {
	terminal, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer closeInTest(t, terminal)

	plain, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeInTest(t, plain)

	t.Setenv("ACTIONS_QA_FORMAT", "")
	if got := defaultFormat(terminal); got != "text" {
		t.Errorf("a terminal's default = %q, want text", got)
	}
	if got := defaultFormat(plain); got != "agent" {
		t.Errorf("a file's default = %q, want agent", got)
	}
	if got := defaultFormat(&bytes.Buffer{}); got != "agent" {
		t.Errorf("a pipe's default = %q, want agent", got)
	}

	t.Setenv("ACTIONS_QA_FORMAT", "json")
	for name, out := range map[string]*os.File{"a terminal": terminal, "a file": plain} {
		if got := defaultFormat(out); got != "json" {
			t.Errorf("%s with ACTIONS_QA_FORMAT=json = %q; the environment wins", name, got)
		}
	}
}

func closeInTest(t *testing.T, file *os.File) {
	t.Helper()

	if err := file.Close(); err != nil {
		t.Error(err)
	}
}
