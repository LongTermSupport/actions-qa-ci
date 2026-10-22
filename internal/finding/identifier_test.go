package finding_test

import (
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

func TestClassifyIdentifierRecognisesEachKindOfIdentifier(t *testing.T) {
	cases := []struct {
		id   string
		want finding.Kind
	}{
		{"actionsqaci.zizmor.template-injection", finding.KindZizmor},
		{"actionsqaci.zizmor.unpinned-uses", finding.KindZizmor},
		{"actionsqaci.actionlint.syntax-check", finding.KindActionlint},
		{"actionsqaci.actionlint.expression", finding.KindActionlint},
		{"actionsqaci.shellcheck.SC2086", finding.KindShellcheck},
		{"actionsqaci.templateDrift", finding.KindFirstParty},
		{"actionsqaci.unjustifiedException", finding.KindFirstParty},
		{"phpqaci.workflowNoSelfHostedOnFork", finding.KindProject},
		{"acme.deployNeedsReview", finding.KindProject},
	}

	for _, c := range cases {
		got, err := finding.ClassifyIdentifier(c.id)
		if err != nil {
			t.Errorf("%s: %v", c.id, err)

			continue
		}
		if got != c.want {
			t.Errorf("%s: kind %q, want %q", c.id, got, c.want)
		}
	}
}

func TestClassifyIdentifierRefusesAMalformedIdentifier(t *testing.T) {
	malformed := []string{
		"",
		"templateDrift",
		"actionsqaci.",
		"actionsqaci.zizmor.",
		"actionsqaci.zizmor.TemplateInjection",
		"actionsqaci.zizmor.template_injection",
		"actionsqaci.zizmor.template-injection.extra",
		"actionsqaci.actionlint.Expression",
		"actionsqaci.shellcheck.SC208",
		"actionsqaci.shellcheck.sc2086",
		"actionsqaci.shellcheck.SC20860",
		"actionsqaci.zizmor",
		"actionsqaci.actionlint",
		"actionsqaci.shellcheck",
		"actionsqaci.template-drift",
		"actionsqaci.TemplateDrift",
		"actionsqaci.templateDrift.extra",
		"acme.deploy-needs-review",
		"acme.DeployNeedsReview",
		"acme.a.b",
		"Acme.deployNeedsReview",
		"acme.deployNeedsReview ",
	}

	for _, id := range malformed {
		if kind, err := finding.ClassifyIdentifier(id); err == nil {
			t.Errorf("%q was accepted as kind %q", id, kind)
		}
	}
}

func TestAProjectIdentifierCannotUseTheToolsOwnPrefix(t *testing.T) {
	// A project rule named actionsqaci.<something> would be indistinguishable from a
	// first-party check; the loader must refuse it, so classification must not call it a
	// project rule.
	kind, err := finding.ClassifyIdentifier("actionsqaci.projectLooking")
	if err != nil {
		t.Fatal(err)
	}
	if kind == finding.KindProject {
		t.Errorf("an actionsqaci. identifier was classified as a project rule")
	}
}
