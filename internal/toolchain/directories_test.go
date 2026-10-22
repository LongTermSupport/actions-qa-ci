package toolchain_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

func TestDirectoriesIsTheOverrideAloneWhenItIsSet(t *testing.T) {
	override := t.TempDir()
	t.Setenv(toolchain.ToolsDirectoryVariable, override)

	dirs, err := toolchain.Directories()
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 || dirs[0] != override {
		t.Errorf("Directories = %v, want only %s", dirs, override)
	}
}

func TestDirectoriesRefusesARelativeOverride(t *testing.T) {
	t.Setenv(toolchain.ToolsDirectoryVariable, "bin")

	dirs, err := toolchain.Directories()
	if err == nil || !strings.Contains(err.Error(), toolchain.ToolsDirectoryVariable) {
		t.Errorf("a relative override must be refused by name, got %v and %v", dirs, err)
	}
}

func TestDirectoriesDefaultsToTheDirectoryOfTheRunningProgram(t *testing.T) {
	t.Setenv(toolchain.ToolsDirectoryVariable, "")

	dirs, err := toolchain.Directories()
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 || dirs[0] != filepath.Dir(resolved) || !filepath.IsAbs(dirs[0]) {
		t.Errorf("Directories = %v, want %s", dirs, filepath.Dir(resolved))
	}
}
