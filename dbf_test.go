package actionsqaci_test

import (
	"testing"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
)

// The versions of the Defence Before Fix method and toolchain specifications this
// repository is audited against. A change to either is a deliberate act: the vendored
// specifications move first, then these.
const (
	methodVersion    = "1.1.0"
	toolchainVersion = "0.2.0"
)

func TestDeclarationIsPinnedToTheAuditedSpecificationVersions(t *testing.T) {
	declaration, err := actionsqaci.LoadDeclaration()
	if err != nil {
		t.Fatalf("the embedded dbf.json does not load: %v", err)
	}

	if declaration.Method != methodVersion {
		t.Errorf("method = %q, want %q", declaration.Method, methodVersion)
	}
	if declaration.Toolchain != toolchainVersion {
		t.Errorf("toolchain = %q, want %q", declaration.Toolchain, toolchainVersion)
	}
	if declaration.Project.Method != methodVersion || declaration.Project.Toolchain != toolchainVersion {
		t.Errorf("project declaration = %q/%q, want it to match the artefact's %q/%q",
			declaration.Project.Method, declaration.Project.Toolchain, methodVersion, toolchainVersion)
	}
}

func TestEveryKnownGapNamesItselfAndSaysWhy(t *testing.T) {
	declaration, err := actionsqaci.LoadDeclaration()
	if err != nil {
		t.Fatalf("the embedded dbf.json does not load: %v", err)
	}

	seen := map[string]bool{}
	for _, gap := range append(append([]actionsqaci.KnownGap{}, declaration.KnownGaps...), declaration.Project.KnownGaps...) {
		if gap.ID == "" || gap.Description == "" {
			t.Errorf("known gap %+v needs an id and a description", gap)
		}
		if seen[gap.ID] {
			t.Errorf("known gap id %q is declared twice", gap.ID)
		}
		seen[gap.ID] = true
	}
}

func TestADeclarationWithAnUnknownFieldIsRefused(t *testing.T) {
	_, err := actionsqaci.ParseDeclaration([]byte(`{"method":"1.1.0","toolchain":"0.2.0","known-gaps":[],"project":{"method":"1.1.0","toolchain":"0.2.0","known-gaps":[]},"extra":true}`))
	if err == nil {
		t.Fatal("an unknown field was accepted; a misspelt key would silently declare nothing")
	}
}

func TestADeclarationMissingAFieldIsRefused(t *testing.T) {
	_, err := actionsqaci.ParseDeclaration([]byte(`{"method":"1.1.0","known-gaps":[],"project":{"method":"1.1.0","toolchain":"0.2.0","known-gaps":[]}}`))
	if err == nil {
		t.Fatal("a declaration with no toolchain version was accepted")
	}
}
