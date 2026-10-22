package firstparty_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
)

// inTree makes a directory holding these files the working directory.
func inTree(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
}

const workflow = ".github/workflows/ci.yml"

func TestInlineSuppressionFindsEveryCommentThatSuppressesWithNoRecord(t *testing.T) {
	cases := map[string]struct {
		text      string
		line, col int
		mentions  string
	}{
		"a zizmor ignore":                            {"on: push\njobs:\n  a:\n    steps:\n      - uses: actions/checkout@v4 # zizmor: ignore[unpinned-uses]\n", 5, 35, "zizmor"},
		"a zizmor ignore with a reason":              {"# zizmor: ignore[template-injection] it is fine\non: push\n", 1, 1, "zizmor"},
		"a zizmor ignore of every audit":             {"on: push\n# zizmor: ignore\n", 2, 1, "zizmor"},
		"a ratchet exclusion":                        {"on: push\njobs:\n  a:\n    steps:\n      - uses: x/y@v1 # ratchet:exclude\n", 5, 22, "ratchet"},
		"a shellcheck disable":                       {"on: push\njobs:\n  a:\n    steps:\n      - run: |\n          # shellcheck disable=SC2086\n          echo $X\n", 6, 11, "ShellCheck"},
		"a shellcheck directive with another option": {"run: |\n  # shellcheck source=/dev/null disable=SC1090,SC2034\n  . \"$F\"\n", 2, 3, "ShellCheck"},
		"capitals":                                  {"# ZIZMOR: IGNORE\n", 1, 1, "zizmor"},
		"capitals for ratchet":                      {"x: 1 # RATCHET:EXCLUDE\n", 1, 6, "ratchet"},
		"capitals for ShellCheck":                   {"run: |\n  # SHELLCHECK DISABLE=SC2086\n", 2, 3, "ShellCheck"},
		"a space before the colon":                  {"# zizmor : ignore\n", 1, 1, "zizmor"},
		"a space before the colon for ratchet":      {"x: 1 # ratchet : exclude\n", 1, 6, "ratchet"},
		"a tab after the hash":                      {"#\tzizmor: ignore\n", 1, 1, "zizmor"},
		"carriage return and line feed":             {"on: push\r\nx: 1\r\n# zizmor: ignore\r\n", 3, 1, "zizmor"},
		"no spaces":                                 {"#zizmor:ignore[x]\n", 1, 1, "zizmor"},
		"a carriage return as the only line ending": {"on: push\r# zizmor: ignore\r", 2, 1, "zizmor"},
		"a next line character":                     {"on: push\u0085# zizmor: ignore\n", 2, 1, "zizmor"},
		"a column after wide characters":            {"name: \"é€\" # zizmor: ignore\n", 1, 12, "zizmor"},
	}

	for name, c := range cases {
		inTree(t, map[string]string{workflow: c.text})

		got, err := firstparty.CheckInlineSuppression(workflow)

		if err != nil || len(got) != 1 {
			t.Errorf("%s: findings %+v, error %v; want exactly one", name, got, err)

			continue
		}
		f := got[0]
		if f.ID != firstparty.InlineSuppression || f.Path != workflow || f.Line != c.line || f.Col != c.col ||
			f.Severity != finding.SeverityError || f.Tool != firstparty.Tool || !strings.Contains(f.Message, c.mentions) {
			t.Errorf("%s: finding %+v; want line %d col %d, severity error, mentioning %q", name, f, c.line, c.col, c.mentions)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestInlineSuppressionLeavesAlonePlainCommentsAboutTheTools(t *testing.T) {
	inTree(t, map[string]string{workflow: `# zizmor is run by the gate
# shellcheck passes on every script here
# disable the cache when debugging
# ratchet pins by hash
on: push
jobs:
  a:
    steps:
      - run: |
          # shellcheck is fine with this
          echo "disable=SC2086"
`})

	got, err := firstparty.CheckInlineSuppression(workflow)

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none", got, err)
	}
}

func TestInlineSuppressionReportsEachCommentInOrder(t *testing.T) {
	inTree(t, map[string]string{workflow: "# shellcheck disable=SC2086\nx: 1 # zizmor: ignore\ny: 2 # ratchet:exclude\n"})

	got, err := firstparty.CheckInlineSuppression(workflow)

	if err != nil || len(got) != 3 || got[0].Line != 1 || got[1].Line != 2 || got[2].Line != 3 {
		t.Errorf("findings %+v, error %v; want three, on lines 1, 2 and 3", got, err)
	}
}

func TestInlineSuppressionCannotReadAFileItIsGivenIsAnError(t *testing.T) {
	inTree(t, map[string]string{})

	got, err := firstparty.CheckInlineSuppression(workflow)

	if err == nil || got != nil {
		t.Errorf("findings %+v, error %v; a file that cannot be read has no verdict", got, err)
	}
}

func TestNativeSuppressionConfigForbidsAZizmorConfigurationFile(t *testing.T) {
	for _, name := range []string{"zizmor.yml", "zizmor.yaml", ".github/zizmor.yml", ".github/zizmor.yaml"} {
		inTree(t, map[string]string{name: "rules: {}\n"})

		got, err := firstparty.CheckNativeSuppressionConfig(name)

		if err != nil || len(got) != 1 {
			t.Errorf("%s: findings %+v, error %v; want one", name, got, err)

			continue
		}
		f := got[0]
		if f.ID != firstparty.NativeSuppressionConfig || f.Path != name || f.Line != 1 || f.Col != 1 || !strings.Contains(f.Message, "zizmor") {
			t.Errorf("%s: finding %+v", name, f)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNativeSuppressionConfigForbidsAnEmptyZizmorConfigurationFileToo(t *testing.T) {
	inTree(t, map[string]string{"zizmor.yml": ""})

	got, err := firstparty.CheckNativeSuppressionConfig("zizmor.yml")

	if err != nil || len(got) != 1 {
		t.Errorf("findings %+v, error %v; the file's existence is what is forbidden", got, err)
	}
}

func TestNativeSuppressionConfigForbidsAnActionlintIgnoreAndAllowsItsCalibrations(t *testing.T) {
	inTree(t, map[string]string{
		".github/actionlint.yaml": "self-hosted-runner:\n  labels: [gpu]\nconfig-variables: [A]\npaths:\n  \".github/workflows/**/*.yml\":\n    ignore:\n      - 'shellcheck reported'\n",
		".github/actionlint.yml":  "self-hosted-runner:\n  labels:\n    - gpu\nconfig-variables:\n  - A\n",
	})

	forbidden, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yaml")
	if err != nil || len(forbidden) != 1 || forbidden[0].Line != 6 || forbidden[0].Col != 5 || !strings.Contains(forbidden[0].Message, "actionlint") {
		t.Errorf("findings %+v, error %v; want one at the ignore key, line 6 column 5", forbidden, err)
	}

	allowed, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yml")
	if err != nil || len(allowed) != 0 {
		t.Errorf("findings %+v, error %v; the calibrations are not a suppression", allowed, err)
	}
}

func TestNativeSuppressionConfigForbidsAPinactIgnoreAndAllowsItsOtherKeys(t *testing.T) {
	inTree(t, map[string]string{
		".pinact.yaml": "version: 3\nfiles:\n  - pattern: .github/workflows/*.yml\nignore_actions:\n  - name: actions/checkout\n    ref: v4\n",
		".pinact.yml":  "version: 3\nfiles:\n  - pattern: .github/workflows/*.yml\n",
	})

	forbidden, err := firstparty.CheckNativeSuppressionConfig(".pinact.yaml")
	if err != nil || len(forbidden) != 1 || forbidden[0].Line != 4 || !strings.Contains(forbidden[0].Message, "pinact") {
		t.Errorf("findings %+v, error %v; want one at ignore_actions, line 4", forbidden, err)
	}

	allowed, err := firstparty.CheckNativeSuppressionConfig(".pinact.yml")
	if err != nil || len(allowed) != 0 {
		t.Errorf("findings %+v, error %v", allowed, err)
	}
}

func TestNativeSuppressionConfigFindsAnIgnoreKeyAtAnyDepthAndInAnyDocument(t *testing.T) {
	inTree(t, map[string]string{".github/actionlint.yaml": "self-hosted-runner:\n  labels: [a]\n---\npaths:\n  x:\n    ignore: [y]\n"})

	got, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yaml")

	if err != nil || len(got) != 1 || got[0].Line != 6 {
		t.Errorf("findings %+v, error %v; want one on line 6", got, err)
	}
}

func TestNativeSuppressionConfigFindsAnIgnoreKeyWrittenAsAnAlias(t *testing.T) {
	inTree(t, map[string]string{
		".github/actionlint.yaml": "names:\n  - &k ignore\npaths:\n  x:\n    *k : [foo]\n",
		".pinact.yaml":            "names:\n  - &k ignore_actions\n*k : [foo]\n",
		".github/actionlint.yml":  "names:\n  - &k labels\nself-hosted-runner:\n  *k : [gpu]\n",
	})

	forbidden, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yaml")
	if err != nil || len(forbidden) != 1 || forbidden[0].Line != 5 || forbidden[0].Col != 5 {
		t.Errorf("actionlint: findings %+v, error %v; want one, at the alias on line 5 column 5", forbidden, err)
	}

	forbidden, err = firstparty.CheckNativeSuppressionConfig(".pinact.yaml")
	if err != nil || len(forbidden) != 1 || forbidden[0].Line != 3 {
		t.Errorf("pinact: findings %+v, error %v; want one, at the alias on line 3", forbidden, err)
	}

	allowed, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yml")
	if err != nil || len(allowed) != 0 {
		t.Errorf("an alias for another key: findings %+v, error %v", allowed, err)
	}
}

func TestNativeSuppressionConfigThatIsNotYAMLIsAnErrorNotACleanResult(t *testing.T) {
	inTree(t, map[string]string{".github/actionlint.yaml": "paths: [unclosed\n", "unreadable/.keep": ""})

	got, err := firstparty.CheckNativeSuppressionConfig(".github/actionlint.yaml")
	if err == nil || got != nil {
		t.Errorf("findings %+v, error %v; a file that cannot be read has no verdict", got, err)
	}

	got, err = firstparty.CheckNativeSuppressionConfig(".pinact.yaml")
	if err == nil || got != nil {
		t.Errorf("a missing file: findings %+v, error %v", got, err)
	}
}

func TestTheSuppressionGuardsAreRegisteredRulesWithSummaries(t *testing.T) {
	registered := map[string]bool{}
	for _, rule := range firstparty.Rules(everythingIsKnown) {
		registered[rule.ID] = rule.Summary != ""
	}

	for _, id := range []string{firstparty.InlineSuppression, firstparty.NativeSuppressionConfig} {
		if !registered[id] {
			t.Errorf("%s is not a registered first-party rule with a summary", id)
		}
	}
}
