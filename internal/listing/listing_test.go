package listing_test

import (
	"slices"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/catalogue"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
)

func TestActiveListsEveryRuleAndFamilyOnceInIdentifierOrder(t *testing.T) {
	reg, err := catalogue.Registry()
	if err != nil {
		t.Fatal(err)
	}

	got := listing.Active(reg)

	ids := make([]string, len(got))
	for i, entry := range got {
		ids[i] = entry.ID
		if entry.Summary == "" || entry.DocPath == "" {
			t.Errorf("%s has no summary or page: %+v", entry.ID, entry)
		}
	}
	if !slices.IsSorted(ids) {
		t.Errorf("identifiers are not in order: %q", ids)
	}
	if len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Errorf("an identifier is listed twice: %q", ids)
	}
	if want := len(reg.All()) + len(reg.Families()); len(got) != want {
		t.Errorf("listed %d entries, the registry holds %d rules and families", len(got), want)
	}
}
