package check_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/LongTermSupport/actions-qa-ci/internal/check"
)

// The configuration files of the bundled tools that can suppress a finding with no record.
var nativeConfigs = []string{
	".github/actionlint.yaml",
	".github/actionlint.yml",
	".github/pinact.yaml",
	".github/pinact.yml",
	".github/zizmor.yaml",
	".github/zizmor.yml",
	".pinact.yaml",
	".pinact.yml",
	"zizmor.yaml",
	"zizmor.yml",
}

func TestDiscoverFindsTheConfigurationFilesThatCanSuppressAFinding(t *testing.T) {
	tree := fstest.MapFS{"README.md": file("# repo\n"), "sub/zizmor.yml": file("not at the root\n")}
	for _, name := range nativeConfigs {
		tree[name] = file("rules: {}\n")
	}

	got, err := check.Discover(tree)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(got, nativeConfigs) {
		t.Errorf("Discover = %v\nwant       %v", got, nativeConfigs)
	}
}

func TestDiscoverRefusesAConfigurationFileItCannotReadAsAFile(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"a directory":     {"zizmor.yml/inside.txt": file("x\n")},
		"a symbolic link": {"zizmor.yaml": {Mode: fs.ModeSymlink, Data: []byte("elsewhere")}},
		"a link in .github": {
			".github/actionlint.yaml": {Mode: fs.ModeSymlink, Data: []byte("elsewhere")},
		},
	}

	for name, tree := range cases {
		got, err := check.Discover(tree)

		if !errors.Is(err, check.ErrNotAnalysable) || got != nil {
			t.Errorf("%s: Discover = %v, %v; a configuration file that cannot be read stops the run", name, got, err)
		}
	}
}

func TestIsNativeSuppressionConfigIsTheClosedSetOfNames(t *testing.T) {
	for _, name := range nativeConfigs {
		if !check.IsNativeSuppressionConfig(name) {
			t.Errorf("%s should be a configuration file that can suppress", name)
		}
	}
	for _, name := range []string{
		"", "zizmor.json", "sub/zizmor.yml", ".github/workflows/zizmor.yml", ".github/actions-qa-ci.yml",
		".github/zizmor.yml/x", "Zizmor.yml", ".github/dependabot.yml", "pinact.yaml", "actionlint.yaml",
	} {
		if check.IsNativeSuppressionConfig(name) {
			t.Errorf("%q should not be one", name)
		}
	}
}

func TestIsDependabotFileIsTheDependabotConfigurationOfTheDefaultSet(t *testing.T) {
	for _, name := range []string{".github/dependabot.yml", ".github/dependabot.yaml"} {
		if !check.IsDependabotFile(name) {
			t.Errorf("%s should be a Dependabot file", name)
		}
	}
	for _, name := range []string{"", "dependabot.yml", ".github/dependabot.yaml.bak", ".github/workflows/dependabot.yml", "sub/.github/dependabot.yml", ".github/actions-qa-ci.yml"} {
		if check.IsDependabotFile(name) {
			t.Errorf("%q should not be one", name)
		}
	}
}

func TestIsActionFileIsAnActionOfTheDefaultSet(t *testing.T) {
	for _, name := range []string{"action.yml", "action.yaml", ".github/actions/x/action.yml", ".github/actions/x/y/action.yaml"} {
		if !check.IsActionFile(name) {
			t.Errorf("%s should be an action file", name)
		}
	}
	for _, name := range []string{"", "other/action.yml", ".github/workflows/action.yml", ".github/action.yml", ".github/actions/x/other.yml", "sub/.github/actions/x/action.yml"} {
		if check.IsActionFile(name) {
			t.Errorf("%q should not be one", name)
		}
	}
}
