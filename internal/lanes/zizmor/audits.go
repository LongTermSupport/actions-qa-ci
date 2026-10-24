package zizmor

import (
	"fmt"
	"slices"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// Audit is one audit zizmor runs, with the standing instruction its rule states.
type Audit struct {
	// Name is zizmor's identifier for the audit, which the rule's identifier ends with.
	Name string
	// Summary is the standing instruction, in the form the other rules state theirs.
	Summary string
	// Held is why this build does not run the audit, or empty when it does. The audit still has
	// its identifier and its page, so that a finding of it is never an unknown one.
	Held string
	// WiderPersonaOnly is true when zizmor reports the audit only at its pedantic or auditor
	// persona, which the lane's own persona is not, so `check` never shows it and only the rule
	// does.
	WiderPersonaOnly bool
}

// widerPersonaNote is what a rule's summary says of an audit `check` does not report.
const widerPersonaNote = "reported by the rule only: zizmor's regular persona, which check runs, leaves it out"

// widerPersonaOnly names the audits the regular persona does not report at all.
var widerPersonaOnly = []string{
	"anonymous-definition",
	"concurrency-limits",
	"secrets-outside-env",
	"self-hosted-runner",
	"superfluous-actions",
	"undocumented-permissions",
}

const (
	heldOnline = "it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline"
	heldConfig = "it does nothing until it is given a policy, and the project record cannot yet give it one"
	// heldPreCommit is for an audit zizmor limits to a pre-commit configuration, which the files
	// this build analyses do not include.
	heldPreCommit = "it reads only a pre-commit configuration, and this build does not collect those"
)

// audits is every audit zizmor reports a finding of, in alphabetical order: the files of
// crates/zizmor/src/audit at the pinned release.
var audits = withPersonas(listed)

// listedAudit is an audit as the list below writes it.
type listedAudit struct{ name, summary, held string }

func withPersonas(list []listedAudit) []Audit {
	built := make([]Audit, 0, len(list))
	for _, e := range list {
		built = append(built, Audit{
			Name:             e.name,
			Summary:          e.summary,
			Held:             e.held,
			WiderPersonaOnly: slices.Contains(widerPersonaOnly, e.name),
		})
	}

	return built
}

var listed = []listedAudit{
	{"adhoc-packages", "No run: step installs a package by name; dependencies come from a lock file", ""},
	{"anonymous-definition", "Every workflow and action definition has a name:", ""},
	{"archived-uses", "No uses: refers to an archived repository", ""},
	{"artipacked", "Checkout does not leave credentials where an uploaded artifact could carry them out", ""},
	{"bot-conditions", "A bot is recognised by a check an attacker cannot spoof, not by github.actor", ""},
	{"cache-poisoning", "A release workflow does not read a cache that untrusted code could have written, and a privileged-trigger job does not write one", ""},
	{"concurrency-limits", "A workflow states a concurrency limit", ""},
	{"dangerous-triggers", "No workflow runs on a trigger that gives untrusted input a privileged token", ""},
	{"dependabot-cooldown", "Dependabot waits out a cooldown before it proposes a new release", ""},
	{"dependabot-execution", "Dependabot does not allow insecure external code execution", ""},
	{"excessive-permissions", "A workflow's token permissions are the least it needs", ""},
	{"forbidden-uses", "uses: stays inside the allow or deny list the project sets", heldConfig},
	{"github-app", "A GitHub App token is scoped to what the job needs and revoked when it ends", ""},
	{"github-env", "No step of a privileged-trigger workflow writes a non-literal value to GITHUB_ENV or GITHUB_PATH", ""},
	{"hardcoded-container-credentials", "Container and service credentials come from secrets, never as literals", ""},
	{"impostor-commit", "A pinned commit belongs to the repository it is pinned in, not to a fork of it", heldOnline},
	{"insecure-commands", "ACTIONS_ALLOW_UNSECURE_COMMANDS is never set", ""},
	{"insecure-url-scheme", "A pre-commit repo: URL uses https or ssh, not http or git", heldPreCommit},
	{"known-vulnerable-actions", "No uses: names an action version that has a published advisory", heldOnline},
	{"misfeature", "No pip-install input, cmd shell or non-standard shell", ""},
	{"obfuscation", "uses: paths and expressions are written plainly, not obfuscated", ""},
	{"overprovisioned-secrets", "A step does not expose the whole secrets context", ""},
	{"ref-confusion", "A uses: ref is not a name that is both a branch and a tag", heldOnline},
	{"ref-version-mismatch", "The version comment beside a pinned commit names that commit's release", heldOnline},
	{"secrets-inherit", "A reusable workflow is given the secrets it needs, not secrets: inherit", ""},
	{"secrets-outside-env", "A secret is used only in a job that has a dedicated environment", ""},
	{"self-hosted-runner", "A job runs on a GitHub-hosted runner unless self-hosting is a recorded decision", ""},
	{"self-repository", "A step that uses an action of the repository's own checkout is written uses: $/path, not uses: ./path", ""},
	{"stale-action-refs", "A pinned commit is the commit of a release tag", heldOnline},
	{"superfluous-actions", "No action does what the runner image already provides", ""},
	{"template-injection", "No ${{ }} expansion puts attacker-controllable data into code", ""},
	{"typosquat-uses", "No uses: names a repository that imitates a popular one", ""},
	{"undocumented-permissions", "Every entry of a permissions: block says why it is needed", ""},
	{"unpinned-images", "A container image is named by a digest, not by no tag, latest or a mutable tag", ""},
	{"unpinned-tools", "The known tool-downloading actions (setup-trivy, load-secrets-action) are given a version, not none or latest", ""},
	{"unpinned-uses", "uses: is pinned to a full commit SHA", ""},
	{"unredacted-secrets", "A secret is used whole, not as a field of structured data the runner would not redact", ""},
	{"unsound-condition", "An if: condition does not hold a fenced expression in a block scalar that keeps its newline, which makes it always true", ""},
	{"unsound-contains", "contains() searches a list, not a string a partial match could satisfy", ""},
	{"unsound-ternary", "A && / || pseudo-ternary does not have a statically falsy true value", ""},
	{"use-trusted-publishing", "A packaging workflow publishes with trusted publishing, not a long-lived token", ""},
}

// Audits returns every audit the lane can report a finding of.
func Audits() []Audit {
	return append([]Audit(nil), audits...)
}

func audit(name string) (Audit, bool) {
	for _, a := range audits {
		if a.Name == name {
			return a, true
		}
	}

	return Audit{}, false
}

// ID is the stable identifier of a zizmor audit.
func ID(name string) string {
	return finding.ToolPrefix + ".zizmor." + name
}

// Rules returns one rule per audit. Each runs zizmor at its widest persona over a file and keeps
// only its own audit, so the harness shows exactly what that rule prints even for an audit the
// default persona leaves out.
func Rules() []registry.Rule {
	rules := make([]registry.Rule, 0, len(audits))
	for _, a := range audits {
		summary := a.Summary
		if a.Held != "" {
			// The listing of defences shows the summary, and must not present as standing an
			// audit this build does not run.
			summary += " (not run by this build: " + a.Held + ")"
		}
		if a.WiderPersonaOnly {
			summary += " (" + widerPersonaNote + ")"
		}
		rules = append(rules, registry.Rule{
			ID:       ID(a.Name),
			Summary:  summary,
			Upstream: Tool(),
			Check:    checkAudit(a),
		})
	}

	return rules
}

func checkAudit(a Audit) func(string) ([]finding.Finding, error) {
	return func(path string) ([]finding.Finding, error) {
		if a.Held != "" {
			return nil, fmt.Errorf("%s is not run by this build: %s", ID(a.Name), a.Held)
		}
		all, err := lintAs(path, personaAuditor)
		if err != nil {
			return nil, err
		}

		var own []finding.Finding
		for _, f := range all {
			if f.UpstreamID == a.Name {
				own = append(own, f)
			}
		}

		return own, nil
	}
}
