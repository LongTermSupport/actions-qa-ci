package actionlint_test

import (
	"slices"
	"strings"
	"testing"

	upstream "github.com/rhysd/actionlint"

	"github.com/LongTermSupport/actions-qa-ci/internal/lanes/actionlint"
)

const (
	credentialsOwner = "actionsqaci.zizmor.hardcoded-container-credentials"
	injectionOwner   = "actionsqaci.zizmor.template-injection"
)

// hardCodedPassword is a workflow actionlint's credentials kind reports. zizmor owns the hazard,
// so the lane reports nothing of it.
var hardCodedPassword = workflow(pushTrigger,
	"    container:\n      image: ghcr.io/x/y:1\n      credentials:\n        username: user\n        password: hunter2\n", runHi)

// untrustedInputs are the ways actionlint words "this expression puts untrusted input into a
// script". zizmor owns the hazard; the type errors of the same kind stay actionlint's.
var untrustedInputs = map[string]fixture{
	"a run step": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ github.event.issue.title }}\n"),
	"a github-script step": workflow(pushTrigger, "",
		"    steps:\n      - uses: actions/github-script@60a0d83039c74a4aee543508d2ffcb1c3799cdea\n        with:\n          script: console.log('${{ github.event.pull_request.body }}')\n"),
	"an object filter": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ join(github.event.commits.*.message) }}\n"),
	"head_ref": workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ github.head_ref }}\n"),
}

func TestWhatZizmorOwnsIsNotReportedByActionlint(t *testing.T) {
	cases := map[string]fixture{"hard-coded container credentials": hardCodedPassword}
	for name, fx := range untrustedInputs {
		cases["untrusted input in "+name] = fx
	}

	for _, name := range sortedKeys(cases) {
		target := inProject(t, cases[name])

		found, err := actionlint.Lint(target)

		if err != nil || len(found) != 0 {
			t.Errorf("%s: got %+v, %v; zizmor owns the hazard and actionlint reports none of it", name, found, err)
		}
	}
}

func TestATypeErrorOfTheExpressionKindStaysWhereAnUntrustedInputIsDropped(t *testing.T) {
	target := inProject(t, workflow(pushTrigger, "",
		"    steps:\n      - run: echo ${{ github.event.issue.title }}\n      - run: echo ${{ github.evnt }}\n"))

	found, err := actionlint.Lint(target)

	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != "actionsqaci.actionlint.expression" || !strings.Contains(found[0].Message, "evnt") {
		t.Errorf("got %+v; the type error is actionlint's alone", found)
	}
}

func TestTheDropsAreTheDuplicatesAndEachNamesItsOwner(t *testing.T) {
	var owners []string
	for _, d := range actionlint.Duplicates() {
		owners = append(owners, d.Owner)
		if strings.TrimSpace(d.Report) == "" {
			t.Errorf("a duplicate owned by %s does not say what is dropped", d.Owner)
		}
	}
	slices.Sort(owners)

	if got, want := strings.Join(owners, ","), credentialsOwner+","+injectionOwner; got != want {
		t.Errorf("the duplicates are owned by %s, want %s", got, want)
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
