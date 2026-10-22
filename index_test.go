package actionsqaci_test

import (
	"flag"
	"os"
	"testing"

	actionsqaci "github.com/LongTermSupport/actions-qa-ci"
	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/pages"
)

var update = flag.Bool("update", false, "rewrite the generated identifier index")

const indexPath = pages.Directory + "/" + pages.IndexFile

func TestTheIdentifierIndexIsCurrent(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}
	want := pages.Index(reg)

	if *update {
		if err := os.WriteFile(indexPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; regenerate it with: go test . -run TestTheIdentifierIndexIsCurrent -update", indexPath)
	}
}

func TestTheEmbeddedPagesPassTheSelfAudit(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	for _, problem := range pages.Audit(reg, actionsqaci.Pages) {
		t.Error(problem)
	}
}
