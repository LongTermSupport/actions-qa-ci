package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/bump"
	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

func validPin() toolchain.Manifest {
	return toolchain.Manifest{Tools: map[string]toolchain.Tool{
		"shellcheck": {Version: "v0.12.0", Assets: map[string]toolchain.Asset{"linux_amd64": {
			URL: "https://example.com/koalaman/shellcheck/releases/download/v0.12.0/shellcheck-v0.12.0.tar.gz", ArchiveSHA256: strings.Repeat("a", 64),
			Member: "shellcheck-v0.12.0/shellcheck", BinarySHA256: strings.Repeat("b", 64),
		}}},
	}}
}

func TestAManifestTheToolchainWouldRefuseIsNeverWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := validPin()
	tool := broken.Tools["shellcheck"]
	asset := tool.Assets["linux_amd64"]
	asset.URL = "http://example.com/plain-http"
	tool.Assets["linux_amd64"] = asset
	broken.Tools["shellcheck"] = tool

	err := rewriteManifest(path, broken)

	if err == nil || !strings.Contains(err.Error(), "would not be accepted") {
		t.Errorf("error %v", err)
	}
	if kept, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(kept, []byte("original")) {
		t.Errorf("the file was changed: %q, %v", kept, readErr)
	}
}

func TestAManifestThatCannotBePlacedLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "manifest.json")
	if err := os.MkdirAll(filepath.Join(target, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := rewriteManifest(target, validPin())

	if err == nil {
		t.Fatal("a directory was replaced by a file")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Errorf("entries %v, %v; the staged file must be removed when it cannot be placed", entries, readErr)
	}
}

func TestOnlyARegularManifestOfReasonableSizeIsRead(t *testing.T) {
	dir := t.TempDir()
	valid, err := bump.Marshal(validPin())
	if err != nil {
		t.Fatal(err)
	}
	// A manifest that would parse, padded past the bound: only the size check refuses it.
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, append(valid, bytes.Repeat([]byte(" "), maxManifest)...), 0o644); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(dir, "sub")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"a large file": big, "a directory": directory, "a FIFO": fifo} {
		answered := make(chan error, 1)
		go func() {
			_, err := readManifest(path)
			answered <- err
		}()

		select {
		case err := <-answered:
			if err == nil {
				t.Errorf("%s was read as a manifest", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: reading it blocked; it must be refused without being opened", name)
		}
	}
	if _, err := readManifest(filepath.Join(dir, "none.json")); err == nil {
		t.Error("a missing file was read")
	}
}
