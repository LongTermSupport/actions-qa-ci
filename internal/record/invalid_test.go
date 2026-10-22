package record_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

func TestAnInvalidRecordCarriesItsReasonsOneByOne(t *testing.T) {
	cases := map[string]struct {
		data    string
		reasons int
		mention string
	}{
		"an unknown field":   {"version: 1\nsurprise: 1\n", 1, "surprise"},
		"a second document":  {"version: 1\n---\nversion: 2\n", 1, "more than one document"},
		"too large":          {"version: 1\n# " + strings.Repeat("x", 2<<20) + "\n", 1, "larger"},
		"an alias":           {"version: &v 1\n", 1, "anchor"},
		"two faulty entries": {"version: 1\nexceptions:\n  - id: actionsqaci.nonsense\n  - id: actionsqaci.alsoNonsense\n", 2, "actionsqaci.alsoNonsense"},
		"not yaml":           {"version: [\n", 1, "yaml"},
	}

	for name, c := range cases {
		_, err := record.Parse([]byte(c.data), func(string) bool { return false })

		var invalid *record.InvalidError
		if !errors.As(err, &invalid) || !errors.Is(err, record.ErrInvalid) {
			t.Errorf("%s: error %v is not an InvalidError that is also ErrInvalid", name, err)

			continue
		}
		if len(invalid.Reasons) != c.reasons {
			t.Errorf("%s: %d reasons %q, want %d", name, len(invalid.Reasons), invalid.Reasons, c.reasons)
		}
		if !strings.Contains(strings.Join(invalid.Reasons, "\n"), c.mention) || !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: reasons %q and error %q must mention %q", name, invalid.Reasons, err.Error(), c.mention)
		}
		if invalid.Path != "" {
			t.Errorf("%s: Parse knows no file, but the path is %q", name, invalid.Path)
		}
	}
}

func TestLoadNamesTheFileOnTheInvalidError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.yml")
	if err := os.WriteFile(path, []byte("version: 1\nsurprise: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rec, err := record.Load(path, everythingIsKnown)

	var invalid *record.InvalidError
	if rec != nil || !errors.As(err, &invalid) || invalid.Path != path {
		t.Fatalf("record = %v, error = %v", rec, err)
	}
	if strings.Count(err.Error(), path) != 1 {
		t.Errorf("error %q must name the file once", err.Error())
	}
}
