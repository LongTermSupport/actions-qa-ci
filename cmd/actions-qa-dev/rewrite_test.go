package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, []byte(strings.Repeat(" ", maxManifest+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	fifoLike := filepath.Join(dir, "sub")
	if err := os.Mkdir(fifoLike, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"a large file": big, "a directory": fifoLike} {
		if _, err := readManifest(path); err == nil {
			t.Errorf("%s was read as a manifest", name)
		}
	}
}
