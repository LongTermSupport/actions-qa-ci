// Package gitindextest writes the git index of a test repository without running git, so a test
// can say which files the repository tracks.
package gitindextest

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The modes an entry can have.
const (
	RegularMode = 0o100644
	LinkMode    = 0o120000
)

// Entry is a tracked path and its mode.
type Entry struct {
	Path string
	Mode uint32
}

// File is a tracked regular file.
func File(path string) Entry { return Entry{Path: path, Mode: RegularMode} }

// Link is a tracked symbolic link.
func Link(path string) Entry { return Entry{Path: path, Mode: LinkMode} }

// Files are tracked regular files.
func Files(paths ...string) []Entry {
	entries := make([]Entry, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, File(path))
	}

	return entries
}

// Write makes root a repository whose index holds exactly these entries: it creates .git if
// there is none and writes .git/index as version 2.
func Write(t testing.TB, root string, entries ...Entry) {
	t.Helper()

	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })

	var buffer bytes.Buffer
	buffer.WriteString("DIRC")
	word(&buffer, 2)
	word(&buffer, uint32(len(sorted)))
	for _, e := range sorted {
		start := buffer.Len()
		for i := 0; i < 6; i++ {
			word(&buffer, 0) // ctime and mtime (seconds, nanoseconds), dev, ino
		}
		word(&buffer, e.Mode)
		for i := 0; i < 3; i++ {
			word(&buffer, 0) // uid, gid, size
		}
		buffer.Write(make([]byte, 20))
		flags := uint16(min(len(e.Path), 0xfff))
		buffer.Write([]byte{byte(flags >> 8), byte(flags)})
		buffer.WriteString(e.Path)
		buffer.WriteByte(0)
		for (buffer.Len()-start)%8 != 0 {
			buffer.WriteByte(0)
		}
	}
	sum := sha1.Sum(buffer.Bytes())
	buffer.Write(sum[:])

	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "index"), buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func word(buffer *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	buffer.Write(raw[:])
}
