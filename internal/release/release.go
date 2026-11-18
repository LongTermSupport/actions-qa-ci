// Package release reads CHANGELOG.md to say what the next release is, cuts it, and names the
// release a tag is still owed for. Nobody chooses a version: the headings under Unreleased do.
//
// Before 1.0.0 an Added or Removed entry releases the next minor and anything else the next
// patch. From 1.0.0 a Removed entry releases the next major, an Added entry the next minor and
// anything else the next patch. With no release yet, the first is 0.1.0. An Unreleased section
// with no entry releases nothing.
package release

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const unreleased = "## Unreleased"

var (
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
	headingPattern = regexp.MustCompile(`^## ((?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8})) — ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	tagPattern     = regexp.MustCompile(`^v((?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8})\.(?:0|[1-9][0-9]{0,8}))$`)

	// headings are the Keep a Changelog headings an Unreleased entry may sit under.
	headings = []string{"### Added", "### Changed", "### Deprecated", "### Removed", "### Fixed", "### Security"}
)

// section is one released version of the changelog.
type section struct {
	version string
	start   int // the line of the heading
	end     int // the line after the last line of the section
}

// parsed is a changelog read into its parts.
type parsed struct {
	lines    []string
	start    int            // the line of the Unreleased heading
	end      int            // the line after the Unreleased section
	kinds    map[string]int // the entries under each Unreleased heading
	sections []section      // newest first
}

func parse(content []byte) (parsed, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	p := parsed{lines: lines, kinds: map[string]int{}}

	p.start = slices.Index(lines, unreleased)
	if p.start < 0 {
		return parsed{}, errors.New("the changelog has no \"" + unreleased + "\" section")
	}
	p.end = len(lines)
	for i := p.start + 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "## ") {
			continue
		}
		if p.end == len(lines) {
			p.end = i
		}
		match := headingPattern.FindStringSubmatch(lines[i])
		if match == nil {
			return parsed{}, fmt.Errorf("line %d: %q is not a release heading of the form \"## 1.2.3 — 2026-01-31\"", i+1, lines[i])
		}
		if _, err := time.Parse(time.DateOnly, match[2]); err != nil {
			return parsed{}, fmt.Errorf("line %d: %q is not a date", i+1, match[2])
		}
		if n := len(p.sections); n > 0 {
			p.sections[n-1].end = i
			if compare(match[1], p.sections[n-1].version) >= 0 {
				return parsed{}, fmt.Errorf("line %d: %s is not older than %s above it; releases are listed newest first", i+1, match[1], p.sections[n-1].version)
			}
		}
		p.sections = append(p.sections, section{version: match[1], start: i, end: len(lines)})
	}

	heading := ""
	for i := p.start + 1; i < p.end; i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "### "):
			if !slices.Contains(headings, line) {
				return parsed{}, fmt.Errorf("line %d: %q is not one of %s", i+1, line, strings.Join(headings, ", "))
			}
			heading = line
		case strings.HasPrefix(line, "- "):
			if heading == "" {
				return parsed{}, fmt.Errorf("line %d: an Unreleased entry sits under no ### heading", i+1)
			}
			p.kinds[heading]++
		}
	}

	return p, nil
}

// Next is the version the Unreleased section releases as, or "" when it holds no entry.
func Next(content []byte) (string, error) {
	p, err := parse(content)
	if err != nil {
		return "", err
	}

	return p.next(), nil
}

func (p parsed) next() string {
	if len(p.kinds) == 0 {
		return ""
	}
	if len(p.sections) == 0 {
		return "0.1.0"
	}
	v := numbers(p.sections[0].version)
	added, removed := p.kinds["### Added"] > 0, p.kinds["### Removed"] > 0
	switch {
	case v[0] == 0 && (added || removed):
		v = [3]int{0, v[1] + 1, 0}
	case v[0] > 0 && removed:
		v = [3]int{v[0] + 1, 0, 0}
	case v[0] > 0 && added:
		v = [3]int{v[0], v[1] + 1, 0}
	default:
		v[2]++
	}

	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// Cut turns the Unreleased entries into the section of version, dated date, under a new empty
// Unreleased heading. The version must be the one Next gives, so a hand-chosen version is refused.
func Cut(content []byte, version, date string) ([]byte, error) {
	p, err := parse(content)
	if err != nil {
		return nil, err
	}
	next := p.next()
	if next == "" {
		return nil, errors.New("the Unreleased section has no entry, so there is nothing to release")
	}
	if version != next {
		return nil, fmt.Errorf("the changelog releases as %s, not %q", next, version)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return nil, fmt.Errorf("%q is not a date of the form 2026-01-31", date)
	}

	eol := "\n"
	if strings.Contains(string(content), "\r\n") {
		eol = "\r\n"
	}
	lines := slices.Insert(slices.Clone(p.lines), p.start+1, "", "## "+version+" — "+date)
	out := strings.Join(lines, eol)
	if strings.HasSuffix(string(content), "\n") {
		out += eol
	}

	return []byte(out), nil
}

// Pending is the newest released version that has no tag vX.Y.Z among tags, or "" when it has
// one. A version older than a tagged one with no tag of its own is an error: that release was
// skipped, and saying nothing would hide it.
func Pending(content []byte, tags []string) (string, error) {
	p, err := parse(content)
	if err != nil {
		return "", err
	}
	tagged := map[string]bool{}
	for _, tag := range tags {
		if match := tagPattern.FindStringSubmatch(strings.TrimSpace(tag)); match != nil {
			tagged[match[1]] = true
		}
	}

	seenTag := false
	for _, s := range p.sections {
		if tagged[s.version] {
			seenTag = true

			continue
		}
		if seenTag {
			return "", fmt.Errorf("%s has no tag v%s, but a newer release is tagged", s.version, s.version)
		}
	}
	if len(p.sections) == 0 || tagged[p.sections[0].version] {
		return "", nil
	}

	return p.sections[0].version, nil
}

// Notes is the body of a released version's section, without its heading.
func Notes(content []byte, version string) (string, error) {
	p, err := parse(content)
	if err != nil {
		return "", err
	}
	for _, s := range p.sections {
		if s.version != version {
			continue
		}
		body := p.lines[s.start+1 : s.end]
		for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
			body = body[1:]
		}
		for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		if len(body) == 0 {
			return "", fmt.Errorf("the section of %s is empty", version)
		}

		return strings.Join(body, "\n") + "\n", nil
	}

	return "", fmt.Errorf("the changelog has no section for %q", version)
}

// numbers splits a version the patterns above have already accepted.
func numbers(version string) [3]int {
	var v [3]int
	for i, part := range strings.SplitN(version, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil {
			panic(fmt.Sprintf("release: %q passed the version pattern but is not numbers: %v", version, err))
		}
		v[i] = n
	}

	return v
}

func compare(a, b string) int {
	x, y := numbers(a), numbers(b)
	for i := range x {
		if x[i] != y[i] {
			return x[i] - y[i]
		}
	}

	return 0
}

// Valid reports whether version is three numbers with no leading v.
func Valid(version string) bool {
	return versionPattern.MatchString(version)
}
