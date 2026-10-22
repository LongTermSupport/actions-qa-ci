package firstparty_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/firstparty"
	"github.com/LongTermSupport/actions-qa-ci/internal/record"
)

const recordPath = ".github/actions-qa-ci.yml"

func templateRecord(pairs ...string) string {
	var text strings.Builder
	text.WriteString("version: 1\ntemplates:\n")
	for i := 0; i+1 < len(pairs); i += 2 {
		text.WriteString("  - source: " + pairs[i] + "\n    copies: [" + pairs[i+1] + "]\n")
	}

	return text.String()
}

func driftIn(t *testing.T, files map[string]string) ([]finding.Finding, error) {
	t.Helper()

	inTree(t, files)

	return firstparty.CheckTemplateDrift(everythingIsKnown)(recordPath)
}

func TestIdenticalCopiesOfATemplateAreClean(t *testing.T) {
	got, err := driftIn(t, map[string]string{
		recordPath:                 templateRecord("templates/qa.yml", ".github/workflows/qa.yml"),
		"templates/qa.yml":         "on: push\r\njobs: {}\n",
		".github/workflows/qa.yml": "on: push\r\njobs: {}\n",
	})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; want none", got, err)
	}
}

func TestACopyThatDiffersByAByteIsAFindingAtTheCopyNamingTheFirstDifferingLine(t *testing.T) {
	cases := map[string]struct {
		source, copyText string
		line             int
	}{
		"a changed line":          {"a\nb\nc\n", "a\nX\nc\n", 2},
		"an added line":           {"a\n", "a\nb\n", 2},
		"a removed line":          {"a\nb\n", "a\n", 2},
		"no final line feed":      {"a\nb\n", "a\nb", 2},
		"line endings":            {"a\nb\n", "a\r\nb\r\n", 1},
		"a trailing space":        {"a: 1\n", "a: 1 \n", 1},
		"empty against something": {"", "a\n", 1},
		"something against empty": {"a\n", "", 1},
	}

	for name, c := range cases {
		got, err := driftIn(t, map[string]string{
			recordPath: templateRecord("t.yml", "c.yml"),
			"t.yml":    c.source,
			"c.yml":    c.copyText,
		})

		if err != nil || len(got) != 1 {
			t.Errorf("%s: findings %+v, error %v; want exactly one", name, got, err)

			continue
		}
		f := got[0]
		if f.ID != firstparty.TemplateDrift || f.Path != "c.yml" || f.Line != 1 || f.Col != 1 || f.Severity != finding.SeverityError || f.Tool != firstparty.Tool {
			t.Errorf("%s: finding %+v; want an error at c.yml line 1 column 1", name, f)
		}
		if !strings.Contains(f.Message, "t.yml") || !strings.Contains(f.Message, "line "+strconv.Itoa(c.line)) {
			t.Errorf("%s: message %q should name the source and say line %d", name, f.Message, c.line)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAMissingCopyOrSourceIsAFindingAtTheRecordNotSilence(t *testing.T) {
	copyMissing, err := driftIn(t, map[string]string{
		recordPath: templateRecord("t.yml", "c.yml"),
		"t.yml":    "a\n",
	})
	if err != nil || len(copyMissing) != 1 || copyMissing[0].Path != recordPath || copyMissing[0].Line != 3 || !strings.Contains(copyMissing[0].Message, "c.yml") || !strings.Contains(copyMissing[0].Message, "missing") {
		t.Errorf("a missing copy: findings %+v, error %v; want one at the record's entry, line 3", copyMissing, err)
	}

	sourceMissing, err := driftIn(t, map[string]string{
		recordPath: templateRecord("t.yml", "c.yml"),
		"c.yml":    "a\n",
	})
	if err != nil || len(sourceMissing) != 1 || sourceMissing[0].Path != recordPath || !strings.Contains(sourceMissing[0].Message, "t.yml") || !strings.Contains(sourceMissing[0].Message, "missing") {
		t.Errorf("a missing source: findings %+v, error %v; want exactly one, naming the source", sourceMissing, err)
	}

	bothMissing, err := driftIn(t, map[string]string{recordPath: templateRecord("t.yml", "c.yml")})
	if err != nil || len(bothMissing) != 1 {
		t.Errorf("both missing: findings %+v, error %v; the source is what is reported", bothMissing, err)
	}
}

func TestACopyOrSourceThatIsNotARegularFileIsAFindingAndIsNeverRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "t.yml"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, recordPath), []byte(templateRecord("t.yml", "pipe.yml")+"  - source: t.yml\n    copies: [dir.yml, link.yml]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.yml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir.yml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("t.yml", filepath.Join(root, "link.yml")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	type outcome struct {
		found []finding.Finding
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		found, err := firstparty.CheckTemplateDrift(everythingIsKnown)(recordPath)
		done <- outcome{found, err}
	}()

	select {
	case got := <-done:
		if got.err != nil || len(got.found) != 3 {
			t.Fatalf("findings %+v, error %v; want one for each of the pipe, the directory and the link", got.found, got.err)
		}
		for _, f := range got.found {
			if !strings.Contains(f.Message, "regular file") {
				t.Errorf("finding %+v should say it is not a regular file", f)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the check blocked on a file that is not a regular file")
	}
}

func TestACopyOverTheSizeLimitIsAFindingNotAnUnboundedRead(t *testing.T) {
	big := strings.Repeat("x", (1<<20)+1)

	got, err := driftIn(t, map[string]string{
		recordPath: templateRecord("t.yml", "c.yml"),
		"t.yml":    big,
		"c.yml":    big,
	})

	if err != nil || len(got) == 0 || !strings.Contains(got[0].Message, "larger than") {
		t.Errorf("findings %+v, error %v; two identical files over the limit cannot be compared and say so", got, err)
	}
}

func TestACopyExactlyAtTheSizeLimitIsComparedAndOneByteOverIsNot(t *testing.T) {
	atLimit := strings.Repeat("x", 1<<20)

	got, err := driftIn(t, map[string]string{
		recordPath: templateRecord("t.yml", "c.yml"),
		"t.yml":    atLimit,
		"c.yml":    atLimit,
	})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v; a file of exactly the limit is compared", got, err)
	}
}

func TestAPathThroughALinkedDirectoryIsFollowedOnlyInsideTheRepository(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, recordPath), templateRecord("t.yml", "linked-out/g.yml")+"  - source: t.yml\n    copies: [linked-in/g.yml]\n")
	write(filepath.Join(root, "t.yml"), "same\n")
	write(filepath.Join(outside, "g.yml"), "same\n")
	write(filepath.Join(root, "real", "g.yml"), "same\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked-out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "linked-in")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	got, err := firstparty.CheckTemplateDrift(everythingIsKnown)(recordPath)

	if err != nil || len(got) != 1 || got[0].Path != recordPath || !strings.Contains(got[0].Message, "linked-out/g.yml") || !strings.Contains(got[0].Message, "outside the repository") {
		t.Errorf("findings %+v, error %v; want one: the copy through the link that leaves the repository, which is not read (the one through a link that stays inside is compared and identical)", got, err)
	}
}

func TestTemplateDriftReportsEveryPairInOrder(t *testing.T) {
	got, err := driftIn(t, map[string]string{
		recordPath: templateRecord("a.yml", "a-copy.yml", "b.yml", "b-copy.yml"),
		"a.yml":    "1\n", "a-copy.yml": "2\n",
		"b.yml": "1\n", "b-copy.yml": "2\n",
	})

	if err != nil || len(got) != 2 || got[0].Path != "a-copy.yml" || got[1].Path != "b-copy.yml" {
		t.Errorf("findings %+v, error %v; want one for each copy, in order", got, err)
	}
}

func TestTemplateDriftOfARecordWithNoTemplatesIsClean(t *testing.T) {
	got, err := driftIn(t, map[string]string{recordPath: "version: 1\n"})

	if err != nil || len(got) != 0 {
		t.Errorf("findings %+v, error %v", got, err)
	}
}

func TestTemplateDriftOnAnInvalidRecordIsTheRecordsError(t *testing.T) {
	_, err := driftIn(t, map[string]string{recordPath: "version: 1\ntemplates:\n  - source: a.yml\n"})

	if !errors.Is(err, record.ErrInvalid) {
		t.Errorf("error = %v; want the record refused as invalid", err)
	}
}

func TestTemplateDriftIsARegisteredRuleWithASummary(t *testing.T) {
	for _, rule := range firstparty.Rules(everythingIsKnown) {
		if rule.ID == firstparty.TemplateDrift {
			if rule.Summary == "" || rule.Check == nil {
				t.Errorf("rule %+v needs a summary and a check", rule)
			}

			return
		}
	}
	t.Errorf("%s is not a registered first-party rule", firstparty.TemplateDrift)
}
