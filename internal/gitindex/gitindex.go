// Package gitindex lists the files a repository tracks by reading its index, so no other
// program has to run to say which files are the project's. The index is read whole and
// checked against its own checksum; anything this reader does not understand completely (a
// split or sparse index, an index of another hash) is refused, because a list that left files
// out would read as a project with fewer files.
package gitindex

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// maxSize bounds an index: the largest repositories have indexes of a few hundred megabytes.
const maxSize = 1 << 29

// Entry is one tracked path and the mode it is tracked with.
type Entry struct {
	Path string
	Mode uint32
}

const (
	modeType    = 0o170000
	modeFile    = 0o100000
	modeSparse  = 0o040000
	checksumLen = 20
	fixedLen    = 62
)

// IsRegular says whether the entry is a regular file, as opposed to a link or a submodule.
func (e Entry) IsRegular() bool { return e.Mode&modeType == modeFile }

// Read lists the paths the repository at root tracks, sorted, each once. A repository with no
// index has nothing tracked, as git sees it. root holds .git, a directory or the file of a
// linked worktree.
func Read(root string) ([]Entry, error) {
	gitDir, err := findGitDir(root)
	if err != nil {
		return nil, err
	}

	index := filepath.Join(gitDir, "index")
	info, err := os.Lstat(index)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("cannot look at the git index: %w", err)
	case !info.Mode().IsRegular():
		return nil, errors.New("the git index is not a regular file")
	case info.Size() > maxSize:
		return nil, fmt.Errorf("the git index is larger than %d bytes", maxSize)
	}

	file, err := os.OpenFile(index, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot read the git index: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("cannot read the git index: %w", err)
	}

	return Parse(data)
}

// findGitDir is the directory that holds the index: .git itself, or the directory a linked
// worktree's .git file points to.
func findGitDir(root string) (string, error) {
	dotGit := filepath.Join(root, ".git")
	info, err := os.Lstat(dotGit)
	switch {
	case err != nil:
		return "", fmt.Errorf("the working directory must be the repository root, which holds .git: %w", err)
	case info.IsDir():
		return dotGit, nil
	case !info.Mode().IsRegular():
		return "", errors.New(".git is neither a directory nor a file")
	}

	file, err := os.Open(dotGit)
	if err != nil {
		return "", fmt.Errorf("cannot read .git: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 4096))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return "", fmt.Errorf("cannot read .git: %w", err)
	}

	target, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !found || target == "" || strings.ContainsRune(target, '\n') {
		return "", errors.New(".git is a file that does not say `gitdir: <directory>`")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	if directory, err := os.Stat(target); err != nil || !directory.IsDir() {
		return "", fmt.Errorf(".git points to %s, which is not a directory", filepath.Clean(target))
	}

	return filepath.Clean(target), nil
}

// Parse reads the bytes of an index of version 2, 3 or 4.
func Parse(data []byte) ([]Entry, error) {
	if len(data) < 12+checksumLen {
		return nil, errors.New("the git index is too short")
	}
	if string(data[:4]) != "DIRC" {
		return nil, errors.New("the git index does not start with DIRC")
	}
	version := binary.BigEndian.Uint32(data[4:8])
	if version < 2 || version > 4 {
		return nil, fmt.Errorf("the git index is version %d; versions 2 to 4 are read", version)
	}

	body := data[:len(data)-checksumLen]
	if sum := sha1.Sum(body); !bytes.Equal(sum[:], data[len(body):]) {
		return nil, errors.New("the git index does not match its checksum (it is damaged, or it is of a repository that does not use SHA-1)")
	}

	count := int(binary.BigEndian.Uint32(data[8:12]))
	if count > len(body)/fixedLen {
		return nil, fmt.Errorf("the git index claims %d entries, more than it can hold", count)
	}

	entries, position, err := parseEntries(body, 12, count, version)
	if err != nil {
		return nil, err
	}
	if err := skipExtensions(body, position); err != nil {
		return nil, err
	}

	slices.SortStableFunc(entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })

	return slices.CompactFunc(entries, func(a, b Entry) bool { return a.Path == b.Path }), nil
}

func parseEntries(body []byte, position, count int, version uint32) ([]Entry, int, error) {
	entries := make([]Entry, 0, count)
	previous := ""

	for i := 0; i < count; i++ {
		start := position
		if position+fixedLen > len(body) {
			return nil, 0, fmt.Errorf("the git index ends inside entry %d", i+1)
		}
		mode := binary.BigEndian.Uint32(body[position+24 : position+28])
		flags := binary.BigEndian.Uint16(body[position+60 : position+62])
		position += fixedLen
		fixed := fixedLen

		if version >= 3 && flags&0x4000 != 0 {
			if position+2 > len(body) {
				return nil, 0, fmt.Errorf("the git index ends inside entry %d", i+1)
			}
			position += 2
			fixed += 2
		}

		if mode&modeType == modeSparse {
			return nil, 0, errors.New("the git index is a sparse index, which lists directories in place of the files in them; it is not read")
		}

		var path string
		if version == 4 {
			strip, used := offsetVarint(body[position:])
			if used == 0 || strip > int64(len(previous)) {
				return nil, 0, fmt.Errorf("entry %d of the git index has a path prefix that cannot be read", i+1)
			}
			position += used
			end := bytes.IndexByte(body[position:], 0)
			if end < 0 {
				return nil, 0, fmt.Errorf("the git index ends inside the path of entry %d", i+1)
			}
			path = previous[:len(previous)-int(strip)] + string(body[position:position+end])
			position += end + 1
			previous = path
		} else {
			end := bytes.IndexByte(body[position:], 0)
			if end < 0 {
				return nil, 0, fmt.Errorf("the git index ends inside the path of entry %d", i+1)
			}
			if named := int(flags & 0xfff); named != 0xfff && named != end {
				return nil, 0, fmt.Errorf("entry %d of the git index names a path of %d bytes and holds %d", i+1, named, end)
			}
			path = string(body[position : position+end])
			position = start + (fixed+end+8)&^7
			if position > len(body) || !allZero(body[start+fixed+end:position]) {
				return nil, 0, fmt.Errorf("the git index ends inside the padding of entry %d", i+1)
			}
		}

		if problem := pathProblem(path); problem != "" {
			return nil, 0, fmt.Errorf("entry %d of the git index has a path %q that %s", i+1, path, problem)
		}
		entries = append(entries, Entry{Path: path, Mode: mode})
	}

	return entries, position, nil
}

// skipExtensions steps over the optional extensions that follow the entries and refuses a
// required one, which changes what the entries mean (a split index keeps most of them in
// another file, a sparse one lists directories).
func skipExtensions(body []byte, position int) error {
	for position < len(body) {
		if position+8 > len(body) {
			return errors.New("the git index ends inside an extension")
		}
		signature := string(body[position : position+4])
		size := int(binary.BigEndian.Uint32(body[position+4 : position+8]))
		if signature[0] < 'A' || signature[0] > 'Z' {
			return fmt.Errorf("the git index needs its extension %q, which this reader does not support (a split or sparse index)", signature)
		}
		if size < 0 || position+8+size > len(body) {
			return fmt.Errorf("the git index ends inside its extension %q", signature)
		}
		position += 8 + size
	}

	return nil
}

// maxPrefix bounds a prefix length before it is shifted again, far above any path and far below
// what could overflow a number.
const maxPrefix = 1 << 32

// offsetVarint reads the number format a version 4 index uses for the length of the prefix to
// drop from the previous path, and says how many bytes it took (0 when it cannot be read).
func offsetVarint(data []byte) (value int64, used int) {
	for i, c := range data {
		if i >= 9 {
			return 0, 0
		}
		if i == 0 {
			value = int64(c & 127)
		} else {
			if value > maxPrefix {
				return 0, 0
			}
			value = (value+1)<<7 + int64(c&127)
		}
		if c&128 == 0 {
			return value, i + 1
		}
	}

	return 0, 0
}

func allZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}

	return true
}

// pathProblem says why a path cannot be a tracked file's, or returns "". Git itself refuses the
// same: a path outside the work tree, or one inside .git.
func pathProblem(path string) string {
	if path == "" {
		return "is empty, as the entries of a split index are (it keeps its paths in another file); a split or sparse index is not read"
	}
	if strings.HasPrefix(path, "/") {
		return "is absolute"
	}
	for _, segment := range strings.Split(path, "/") {
		switch {
		case segment == "":
			return "has an empty segment"
		case segment == "." || segment == "..":
			return "is not a clean path"
		case strings.EqualFold(segment, ".git"):
			return "is inside .git"
		}
	}

	return ""
}
