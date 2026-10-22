package actionlint

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// ruleSets creates actionlint's rules with and without a ShellCheck, and returns what each had.
func ruleSets(t *testing.T) (without, with []upstream.Rule) {
	t.Helper()

	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	pinned := filepath.Join(bin, "shellcheck")
	if _, err := os.Stat(pinned); err != nil {
		t.Fatalf("the pinned ShellCheck is not fetched (go run ./cmd/actions-qa-dev fetch bin): %v", err)
	}
	t.Setenv(toolchain.ToolsDirectoryVariable, bin)

	create := func(shellcheck string) []upstream.Rule {
		var created []upstream.Rule
		linter, err := upstream.NewLinter(io.Discard, &upstream.LinterOptions{
			Shellcheck: shellcheck,
			OnRulesCreated: func(rules []upstream.Rule) []upstream.Rule {
				created = rules

				return rules
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := linter.Lint("w.yml", []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"), nil); err != nil {
			t.Fatal(err)
		}

		return created
	}

	return create(""), create(pinned)
}

func TestShellcheckRuleIsRecognisedAmongActionlintsRules(t *testing.T) {
	without, with := ruleSets(t)

	if hasShellcheckRule(without) {
		t.Error("actionlint created a ShellCheck rule although it was given no ShellCheck")
	}
	if !hasShellcheckRule(with) {
		t.Error("actionlint did not create its ShellCheck rule although it was given the pinned binary")
	}
}
