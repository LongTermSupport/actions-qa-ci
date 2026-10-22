package gitindex_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/gitindex"
)

const (
	regular = 0o100644
	link    = 0o120000
	gitlink = 0o160000
	sparse  = 0o040000
)

type entry struct {
	path     string
	mode     uint32
	stage    int
	extended bool
}

func putUint32(buffer *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	buffer.Write(raw[:])
}

// buildIndex writes an index of the given version the way git does, with the entries in the
// order given.
func buildIndex(version uint32, entries []entry, extensions ...[]byte) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("DIRC")
	putUint32(&buffer, version)
	putUint32(&buffer, uint32(len(entries)))

	previous := ""
	for _, e := range entries {
		start := buffer.Len()
		for i := 0; i < 5; i++ {
			putUint32(&buffer, 0) // ctime and mtime (seconds, nanoseconds), dev
		}
		putUint32(&buffer, 0) // ino
		putUint32(&buffer, e.mode)
		putUint32(&buffer, 0) // uid
		putUint32(&buffer, 0) // gid
		putUint32(&buffer, 0) // size
		buffer.Write(make([]byte, 20))

		flags := uint16(min(len(e.path), 0xfff)) | uint16(e.stage)<<12
		if e.extended {
			flags |= 0x4000
		}
		buffer.WriteByte(byte(flags >> 8))
		buffer.WriteByte(byte(flags))
		if e.extended {
			buffer.Write([]byte{0, 0})
		}

		if version == 4 {
			common := 0
			for common < len(previous) && common < len(e.path) && previous[common] == e.path[common] {
				common++
			}
			strip := len(previous) - common
			buffer.Write(offsetVarint(strip))
			buffer.WriteString(e.path[common:])
			buffer.WriteByte(0)
			previous = e.path

			continue
		}
		buffer.WriteString(e.path)
		buffer.WriteByte(0)
		for (buffer.Len()-start)%8 != 0 {
			buffer.WriteByte(0)
		}
	}

	for _, extension := range extensions {
		buffer.Write(extension)
	}
	sum := sha1.Sum(buffer.Bytes())
	buffer.Write(sum[:])

	return buffer.Bytes()
}

// offsetVarint is git's offset encoding of a number, as used for the prefix length of a version
// 4 index.
func offsetVarint(value int) []byte {
	encoded := []byte{byte(value & 127)}
	for value >>= 7; value != 0; value >>= 7 {
		value--
		encoded = append([]byte{byte(128 | value&127)}, encoded...)
	}

	return encoded
}

func extension(signature string, body string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(signature)
	putUint32(&buffer, uint32(len(body)))
	buffer.WriteString(body)

	return buffer.Bytes()
}

func paths(entries []gitindex.Entry) []string {
	var list []string
	for _, e := range entries {
		list = append(list, e.Path)
	}

	return list
}

func TestParseReadsTheEntriesOfEveryIndexVersion(t *testing.T) {
	long := strings.Repeat("d/", 2500) + "x.yml"
	entries := []entry{
		{path: ".github/workflows/ci.yml", mode: regular},
		{path: ".github/workflows/qa.yml", mode: regular},
		{path: "a b/ünï.yml", mode: regular},
		{path: long, mode: regular},
		{path: "link.yml", mode: link},
		{path: "sub", mode: gitlink},
	}

	for _, version := range []uint32{2, 3, 4} {
		got, err := gitindex.Parse(buildIndex(version, entries))

		if err != nil {
			t.Errorf("version %d: %v", version, err)

			continue
		}
		want := []string{".github/workflows/ci.yml", ".github/workflows/qa.yml", "a b/ünï.yml", long, "link.yml", "sub"}
		if !slices.Equal(paths(got), want) {
			t.Errorf("version %d: %d paths, the first of them %q", version, len(got), paths(got)[:min(3, len(got))])

			continue
		}
		if !got[0].IsRegular() || got[4].IsRegular() || got[5].IsRegular() {
			t.Errorf("version %d: a regular file, a link and a gitlink were not told apart: %+v", version, got)
		}
	}
}

func TestParseReadsAnEntryWithExtendedFlagsInVersion3(t *testing.T) {
	got, err := gitindex.Parse(buildIndex(3, []entry{{path: "a.yml", mode: regular, extended: true}, {path: "b.yml", mode: regular}}))

	if err != nil || !slices.Equal(paths(got), []string{"a.yml", "b.yml"}) {
		t.Errorf("paths %q, error %v", paths(got), err)
	}
}

func TestAPathInSeveralStagesIsListedOnce(t *testing.T) {
	got, err := gitindex.Parse(buildIndex(2, []entry{{path: "a.yml", mode: regular, stage: 1}, {path: "a.yml", mode: regular, stage: 2}, {path: "a.yml", mode: regular, stage: 3}}))

	if err != nil || !slices.Equal(paths(got), []string{"a.yml"}) {
		t.Errorf("paths %q, error %v", paths(got), err)
	}
}

func TestAnOptionalExtensionIsSkippedAndARequiredOneIsRefused(t *testing.T) {
	entries := []entry{{path: "a.yml", mode: regular}}

	got, err := gitindex.Parse(buildIndex(2, entries, extension("TREE", "ignored cache"), extension("FSMN", "x")))
	if err != nil || !slices.Equal(paths(got), []string{"a.yml"}) {
		t.Errorf("an optional extension: paths %q, error %v", paths(got), err)
	}

	for _, signature := range []string{"link", "sdir", "abcd"} {
		if _, err := gitindex.Parse(buildIndex(2, entries, extension(signature, "body"))); err == nil {
			t.Errorf("the required extension %q was accepted: the index would be read incompletely", signature)
		}
	}
}

func TestASparseDirectoryEntryIsRefusedBecauseTheFilesBehindItAreNotListed(t *testing.T) {
	_, err := gitindex.Parse(buildIndex(2, []entry{{path: "tests/", mode: sparse}}))

	if err == nil || !strings.Contains(err.Error(), "sparse") {
		t.Errorf("error %v; want a refusal that names the sparse index", err)
	}
}

func TestParseRefusesWhatIsNotAnIntactIndex(t *testing.T) {
	good := buildIndex(2, []entry{{path: "a.yml", mode: regular}, {path: "b.yml", mode: regular}})
	flipped := slices.Clone(good)
	flipped[len(flipped)/2] ^= 0xff
	badSignature := slices.Clone(good)
	badSignature[0] = 'X'
	badVersion := buildIndex(5, nil)
	overstated := slices.Clone(good)
	overstated[11] = 200

	cases := map[string][]byte{
		"empty":                   nil,
		"too short":               good[:10],
		"truncated":               good[:len(good)-30],
		"a changed byte":          flipped,
		"a wrong signature":       badSignature,
		"an unknown version":      badVersion,
		"more entries than exist": overstated,
	}
	for name, data := range cases {
		if _, err := gitindex.Parse(data); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// resealed is the index with its body replaced and the checksum made right again, so that what
// is wrong with it is the body and not the checksum.
func resealed(body []byte) []byte {
	sum := sha1.Sum(body)

	return append(slices.Clone(body), sum[:]...)
}

func TestParseRefusesAPathPrefixLengthThatOverflowsInsteadOfPanicking(t *testing.T) {
	good := buildIndex(4, []entry{{path: "a.yml", mode: regular}, {path: "b.yml", mode: regular}})
	body := good[:len(good)-20]
	// Entry 1 is 62 bytes, a one-byte prefix length and "a.yml" with its NUL; entry 2's prefix
	// length follows its own 62 bytes.
	at := 12 + 62 + 1 + 6 + 62
	hostile := slices.Concat(body[:at], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, body[at+1:])

	for _, overflow := range [][]byte{
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00},
	} {
		sealed := resealed(slices.Concat(body[:at], overflow, body[at+1:]))
		if _, err := gitindex.Parse(sealed); err == nil {
			t.Errorf("a prefix length of %x was accepted", overflow)
		}
	}
	if _, err := gitindex.Parse(resealed(hostile)); err == nil {
		t.Error("the overflowing prefix length was accepted")
	}
}

func TestASplitIndexIsRefusedWithAnExplanationNotAsAnEmptyPath(t *testing.T) {
	data := buildIndex(2, []entry{{path: "", mode: regular}}, extension("link", "shared index"))

	_, err := gitindex.Parse(data)

	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Errorf("error %v; want a refusal that says a split index is not read", err)
	}
}

func TestParseRefusesAPathThatIsNotInsideTheRepository(t *testing.T) {
	for _, path := range []string{"/abs.yml", "a/../b.yml", "../b.yml", "a//b.yml", "./a.yml", "a/", ".git/config", "x/.git/config"} {
		if _, err := gitindex.Parse(buildIndex(2, []entry{{path: path, mode: regular}})); err == nil {
			t.Errorf("%q was accepted", path)
		}
	}
}

func TestReadFindsTheIndexOfARepositoryAndOfALinkedWorktree(t *testing.T) {
	index := buildIndex(2, []entry{{path: "a.yml", mode: regular}})

	plain := t.TempDir()
	if err := os.MkdirAll(filepath.Join(plain, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, ".git", "index"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := gitindex.Read(plain)
	if err != nil || !slices.Equal(paths(got), []string{"a.yml"}) {
		t.Errorf("a repository: paths %q, error %v", paths(got), err)
	}

	worktree := t.TempDir()
	gitDir := filepath.Join(t.TempDir(), "worktrees", "w")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = gitindex.Read(worktree)
	if err != nil || !slices.Equal(paths(got), []string{"a.yml"}) {
		t.Errorf("a linked worktree: paths %q, error %v", paths(got), err)
	}
}

func TestReadTreatsAMissingIndexAsNothingTracked(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := gitindex.Read(root)

	if err != nil || len(got) != 0 {
		t.Errorf("entries %v, error %v; a repository with nothing added has no index, as git sees it", got, err)
	}
}

func TestReadRefusesWhatItCannotTrust(t *testing.T) {
	cases := map[string]func(t *testing.T, root string){
		"no .git": func(t *testing.T, root string) {},
		"a .git that is neither": func(t *testing.T, root string) {
			if err := os.Symlink("/", filepath.Join(root, ".git")); err != nil {
				t.Fatal(err)
			}
		},
		"a .git file that is not a gitdir line": func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, ".git"), []byte("nonsense\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"an index that is a link": func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/etc/hostname", filepath.Join(root, ".git", "index")); err != nil {
				t.Fatal(err)
			}
		},
		"an index that is a directory": func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".git", "index"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, arrange := range cases {
		root := t.TempDir()
		arrange(t, root)

		if _, err := gitindex.Read(root); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseAgreesWithGitOnARealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed, so there is nothing to compare with")
	}

	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()

		command := exec.Command("git", args...)
		command.Dir = root
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}

		return string(out)
	}

	run("init", "-q")
	files := []string{".github/workflows/ci.yml", "a b/c d.yml", "ünï/ç.yml", "deep/" + strings.Repeat("x/", 40) + "f.yml", "zz.txt", "A.yml", "a.yml"}
	for _, file := range files {
		full := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("zz.txt", filepath.Join(root, "link.yml")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("add", "-N", "--", "a.yml")

	for _, version := range []string{"2", "4"} {
		run("update-index", "--index-version", version)

		got, err := gitindex.Read(root)
		if err != nil {
			t.Fatalf("index version %s: %v", version, err)
		}
		want := strings.Split(strings.TrimSuffix(run("ls-files", "-z", "--cached"), "\x00"), "\x00")
		slices.Sort(want)
		if !slices.Equal(paths(got), want) {
			t.Errorf("index version %s:\n got %q\nwant %q", version, paths(got), want)
		}
	}
}
