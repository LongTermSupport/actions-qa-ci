package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAFileLargerThanTheBoundIsRefusedNotRead(t *testing.T) {
	previous := maxFile
	maxFile = 8
	t.Cleanup(func() { maxFile = previous })

	path := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(path, []byte("123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(t.TempDir(), "small")
	if err := os.WriteFile(small, []byte("12345678"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readPlainFile(path, "the big file"); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("error %v, want a refusal that names the size", err)
	}
	if content, err := readPlainFile(small, "the small file"); err != nil || string(content) != "12345678" {
		t.Errorf("a file exactly at the bound: %q, %v", content, err)
	}
}
