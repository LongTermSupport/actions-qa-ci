// Package ghfilter matches paths and branch names against the filter patterns of a workflow's
// trigger (`paths`, `paths-ignore`, `branches`, `branches-ignore`), as GitHub reads them. It is
// not the record's pattern language (package glob): a workflow's filters are GitHub's to define,
// so this package follows the "Filter pattern cheat sheet" of GitHub's workflow syntax reference.
//
// `*` matches any run of characters but a slash; `**` any run of characters, slashes included,
// and `**/` at the start of a pattern or after a slash also matches nothing at all; `?` makes the
// character before it optional and `+` lets it repeat; `[...]` matches one character of a set of
// letters, digits and ranges; a backslash makes the next character literal. A pattern matches the
// whole name. In a list, a pattern starting with `!` takes back what earlier patterns matched,
// and the last pattern that matches a name decides.
package ghfilter

import (
	"regexp"
	"strings"
)

// Valid says whether a pattern, after any leading `!`, is one this package can read.
func Valid(pattern string) bool {
	_, ok := compile(strings.TrimPrefix(pattern, "!"))

	return ok
}

// Match says whether the name matches the pattern, which has no leading `!`. A pattern that
// cannot be read matches nothing.
func Match(pattern, name string) bool {
	re, ok := compile(pattern)

	return ok && re.MatchString(name)
}

// Included says whether a `paths` or `branches` list admits the name: the last pattern that
// matches it is not a negated one.
func Included(patterns []string, name string) bool {
	included := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if Match(strings.TrimPrefix(pattern, "!"), name) {
			included = !negated
		}
	}

	return included
}

// Ignored says whether a `paths-ignore` or `branches-ignore` list leaves the name out: one of
// its patterns matches it.
func Ignored(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if Match(pattern, name) {
			return true
		}
	}

	return false
}

// compile turns a pattern into an anchored regular expression, or says it cannot be read.
func compile(pattern string) (*regexp.Regexp, bool) {
	if pattern == "" {
		return nil, false
	}

	var out strings.Builder
	out.WriteString("^")
	atom := false // whether the last thing written can take a ? or a +
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch {
		case strings.HasPrefix(pattern[i:], "**/") && (i == 0 || pattern[i-1] == '/'):
			out.WriteString("(?:.*/)?")
			i += 3
			atom = false
		case strings.HasPrefix(pattern[i:], "**"):
			out.WriteString(".*")
			i += 2
			atom = false
		case c == '*':
			out.WriteString("[^/]*")
			i++
			atom = false
		case (c == '?' || c == '+') && atom:
			out.WriteByte(c)
			i++
			atom = false
		case c == '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 1 {
				return nil, false
			}
			set := pattern[i+1 : i+1+end]
			if !validSet(set) {
				return nil, false
			}
			out.WriteString("[" + set + "]")
			i += end + 2
			atom = true
		case c == '\\':
			if i+1 >= len(pattern) {
				return nil, false
			}
			out.WriteString(regexp.QuoteMeta(pattern[i+1 : i+2]))
			i += 2
			atom = true
		default:
			out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
			atom = true
		}
	}
	out.WriteString("$")

	re, err := regexp.Compile(out.String())
	if err != nil {
		return nil, false
	}

	return re, true
}

// validSet says whether the inside of a bracket expression holds only letters, digits and
// ranges between two of the same kind, which is all GitHub accepts there.
func validSet(set string) bool {
	for i := 0; i < len(set); i++ {
		if !alphanumeric(set[i]) {
			return false
		}
		if i+2 < len(set) && set[i+1] == '-' {
			if kind(set[i]) != kind(set[i+2]) || set[i] > set[i+2] {
				return false
			}
			i += 2
		}
	}

	return true
}

func alphanumeric(c byte) bool { return kind(c) != 0 }

func kind(c byte) int {
	switch {
	case c >= 'a' && c <= 'z':
		return 1
	case c >= 'A' && c <= 'Z':
		return 2
	case c >= '0' && c <= '9':
		return 3
	}

	return 0
}
