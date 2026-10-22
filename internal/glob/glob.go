// Package glob matches repository-relative paths against the patterns a project record holds.
// The patterns are small on purpose: a pattern that could be read two ways would let a record
// say one thing to its author and another to the program.
//
// A pattern is slash-separated and anchored at the repository root, like the paths it matches.
// In a segment, `*` matches any run of characters and `?` any one character, neither crossing a
// slash. A segment that is exactly `**` matches any number of whole segments: none included in
// the middle of a pattern (`a/**/b` matches `a/b`), at least one at the end (`a/**` matches what
// is inside `a`, not `a`).
package glob

import (
	"errors"
	"fmt"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Check says why a pattern cannot be used, or returns nil.
func Check(pattern string) error {
	switch {
	case pattern == "":
		return errors.New("is empty")
	case strings.HasPrefix(pattern, "/"):
		return errors.New("is absolute; use a path relative to the repository root")
	case strings.HasSuffix(pattern, "/"):
		return errors.New("must not end with a slash; name the files, for example dir/**")
	case strings.ContainsFunc(pattern, finding.IsControlOrSeparator):
		return errors.New("contains a control character")
	case strings.ContainsRune(pattern, '\\'):
		return errors.New("contains a backslash; use forward slashes")
	case strings.ContainsAny(pattern, "[]{}"):
		return errors.New("contains a character class or brace set, which is not supported; use * and ?")
	case strings.HasPrefix(pattern, "!"):
		return errors.New("starts with !; negation is not supported")
	}

	for _, segment := range strings.Split(pattern, "/") {
		switch {
		case segment == "":
			return errors.New("has an empty segment (two slashes in a row)")
		case segment == "." || segment == "..":
			return fmt.Errorf("has a %q segment; use a clean path inside the repository", segment)
		case strings.Contains(segment, "**") && segment != "**":
			return errors.New("uses ** inside a segment; ** must be a whole segment")
		}
	}

	return nil
}

// Match says whether the path, slash-separated and relative to the repository root, matches the
// pattern. A pattern that Check refuses matches nothing.
func Match(pattern, name string) bool {
	if Check(pattern) != nil {
		return false
	}

	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// matchSegments matches whole segments, with ** standing for any number of them. It fills a
// table of whether the first i pattern segments match the first j name segments, so the cost
// is the product of the two lengths whatever the pattern holds.
func matchSegments(pattern, name []string) bool {
	matches := make([][]bool, len(pattern)+1)
	for i := range matches {
		matches[i] = make([]bool, len(name)+1)
	}
	matches[0][0] = true

	for i := 1; i <= len(pattern); i++ {
		if pattern[i-1] == "**" {
			for j := 0; j <= len(name); j++ {
				// ** matches one more segment than it matched at the previous name segment. In
				// the middle of a pattern it may also match none; at the end it matches what is
				// inside a directory, so at least one.
				more := j > 0 && (matches[i][j-1] || matches[i-1][j-1])
				none := i < len(pattern) && matches[i-1][j]
				matches[i][j] = more || none
			}

			continue
		}
		for j := 1; j <= len(name); j++ {
			matches[i][j] = matches[i-1][j-1] && matchSegment([]rune(pattern[i-1]), []rune(name[j-1]))
		}
	}

	return matches[len(pattern)][len(name)]
}

// matchSegment matches one segment: * for any run, ? for any one character. It remembers the
// last * to retry from, so a pattern full of them is not exponential.
func matchSegment(pattern, name []rune) bool {
	p, n := 0, 0
	star, retry := -1, 0

	for n < len(name) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, retry = p, n
			p++
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == name[n]):
			p++
			n++
		case star >= 0:
			retry++
			p, n = star+1, retry
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}

	return p == len(pattern)
}
