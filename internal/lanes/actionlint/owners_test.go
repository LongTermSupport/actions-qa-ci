package actionlint_test

import (
	"strings"
	"testing"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
)

const credentialsOwner = "actionsqaci.zizmor.hardcoded-container-credentials"

// hardCodedPassword is a workflow actionlint's credentials kind reports. zizmor owns the hazard,
// so the lane reports nothing of it.
var hardCodedPassword = workflow(pushTrigger,
	"    container:\n      image: ghcr.io/x/y:1\n      credentials:\n        username: user\n        password: hunter2\n", runHi)

// untrustedInputs are workflows whose expression actionlint calls potentially untrusted. zizmor's
// regular persona does not report every one of them (a fromJSON of the input is only a
// pedantic-persona finding), so the lane keeps actionlint's report of all of them.
var untrustedInputs = map[string]fixture{
	"a run step": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ github.event.issue.title }}\n"),
	"a github-script step": workflow(pushTrigger, "",
		"    steps:\n      - uses: actions/github-script@60a0d83039c74a4aee543508d2ffcb1c3799cdea\n        with:\n          script: console.log('${{ github.event.pull_request.body }}')\n"),
	"an object filter": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ join(github.event.commits.*.message) }}\n"),
	"a fromJSON of the input": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ fromJSON(github.event.issue.title) }}\n"),
	"a branch name": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ github.head_ref }}\n"),
	"an object filter over every property": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ toJSON(github.event.*.body) }}\n"),
}

func TestTheObjectFilterWordingIsOneOfTheUntrustedMessagesKept(t *testing.T) {
	target := inProject(t, untrustedInputs["an object filter over every property"])

	found, err := actionlint.Lint(target)

	if err != nil || len(found) != 1 || !strings.Contains(found[0].Message, "object filter extracts potentially untrusted properties") {
		t.Errorf("got %+v, %v; this fixture is the object-filter wording of the message", found, err)
	}
}

func TestHardCodedContainerCredentialsAreNotReportedByActionlint(t *testing.T) {
	target := inProject(t, hardCodedPassword)

	found, err := actionlint.Lint(target)

	if err != nil || len(found) != 0 {
		t.Errorf("got %+v, %v; zizmor owns the hazard and actionlint reports none of it", found, err)
	}
}

func TestAnUntrustedInputIsStillReportedByActionlintWhereZizmorsRegularPersonaMissesIt(t *testing.T) {
	for _, name := range sortedKeys(untrustedInputs) {
		target := inProject(t, untrustedInputs[name])

		found, err := actionlint.Lint(target)

		if err != nil || len(found) != 1 || found[0].ID != "actionsqaci.actionlint.expression" || !strings.Contains(found[0].Message, "potentially untrusted") {
			t.Errorf("%s: got %+v, %v; dropping this report would leave a hazard nobody reports", name, found, err)
		}
	}
}

func TestTheOnlyDuplicateDroppedIsTheCredentialsKindAndItNamesItsOwner(t *testing.T) {
	duplicates := actionlint.Duplicates()

	if len(duplicates) != 1 || duplicates[0].Owner != credentialsOwner || duplicates[0].Kind != "credentials" || !duplicates[0].WholeKind || strings.TrimSpace(duplicates[0].Report) == "" {
		t.Errorf("duplicates = %+v", duplicates)
	}
}

func TestADroppedKindHasNoRuleAndIsStillKnown(t *testing.T) {
	for _, kind := range actionlint.Kinds() {
		if kind.Name == "credentials" {
			t.Errorf("credentials is still a rule of its own; the hazard has one owner")
		}
	}
	found, err := actionlint.Convert(workflowPath, []*upstream.Error{{Message: "m", Line: 1, Column: 1, Kind: "credentials"}})

	if err != nil || len(found) != 0 {
		t.Errorf("got %+v, %v; the kind is dropped, not refused as unknown", found, err)
	}
}
