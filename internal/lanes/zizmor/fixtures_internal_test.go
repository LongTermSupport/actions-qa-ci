package zizmor

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture is a small repository: the file the lane is pointed at, and every file in it.
type fixture struct {
	target string
	files  map[string]string
}

// pair is a red fixture the audit must fire on and a green one, the corrected construction, it
// must stay quiet on. Both are judged at the widest persona, so an audit of any persona has a
// pair.
type pair struct {
	red   fixture
	green fixture
	// alsoFires names the audits, other than the one the pair is for, that the red fixture
	// cannot avoid firing. It is empty for most: a red fixture differs from a clean one by the
	// defect alone.
	alsoFires []string
	// atDefault says whether the lane's own persona (regular) reports the red fixture. An audit
	// of the pedantic or auditor persona is false here, and shows only in the rule's harness.
	atDefault bool
}

// fixtures has one pair for every audit this build runs. Files of the package add theirs in an
// init function with addFixtures, so that each can be written without touching another.
var fixtures = map[string]pair{}

func addFixtures(more map[string]pair) {
	for audit, p := range more {
		if _, taken := fixtures[audit]; taken {
			panic("two fixture pairs for the audit " + audit)
		}
		fixtures[audit] = p
	}
}

const (
	workflowPath = ".github/workflows/w.yml"
	actionPath   = "action.yml"
	// checkoutPin is a full commit SHA with the version comment zizmor looks for beside it.
	checkoutPin = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0"
	// pinnedImage is a container image named by digest.
	pinnedImage = "ghcr.io/x/y@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// header is the top of a workflow with nothing for any audit to find: a name, a trigger that
// carries no untrusted input to a privileged token, no permissions at the top and a concurrency
// limit.
const header = "name: x\non: push\npermissions: {}\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n"

// cleanSteps is a checkout that keeps no credentials, then a step that does nothing.
const cleanSteps = "" +
	"      - name: checkout\n" +
	"        uses: " + checkoutPin + "\n" +
	"        with:\n" +
	"          persist-credentials: false\n" +
	"      - name: hi\n" +
	"        run: echo hi\n"

// jobWith is the one job `a` of a workflow with nothing for any audit to find, with extra lines
// for the job (already indented four spaces) and the given steps (already indented six).
func jobWith(extras, steps string) string {
	return "jobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n" + extras + "    steps:\n" + steps
}

// clean is a workflow with nothing for any audit to find.
func clean() fixture {
	return single(header + jobWith("", cleanSteps))
}

// single is a fixture of one workflow.
func single(source string) fixture {
	return fixture{target: workflowPath, files: map[string]string{workflowPath: source}}
}

// workflowOf is a one-job fixture: the header, then the job with these extras and steps.
func workflowOf(extras, steps string) fixture {
	return single(header + jobWith(extras, steps))
}

// inProject writes the fixture as a repository in a temporary directory, makes it the working
// directory, and returns the repository-relative path of the file to lint.
func inProject(t *testing.T, fx fixture) string {
	t.Helper()

	root := t.TempDir()
	files := map[string]string{".git/HEAD": "ref: refs/heads/main\n"}
	for path, content := range fx.files {
		files[path] = content
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	return fx.target
}
