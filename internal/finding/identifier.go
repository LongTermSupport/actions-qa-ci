// Package finding is the model every lane reports in: one Finding type, the identifier
// scheme that names what a finding is about, and a stable order for printing them.
package finding

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind says which family an identifier belongs to, and so which owner answers for it.
type Kind string

// The kinds of identifier the scheme defines.
const (
	KindZizmor     Kind = "zizmor"
	KindActionlint Kind = "actionlint"
	KindShellcheck Kind = "shellcheck"
	KindFirstParty Kind = "first-party"
	KindProject    Kind = "project"
)

// ToolPrefix is the leading segment of every identifier this toolchain owns. A project rule
// may not use it.
const ToolPrefix = "actionsqaci"

const (
	kebab = `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`
	camel = `[a-z][a-zA-Z0-9]*`
)

var (
	zizmorPattern     = regexp.MustCompile(`^` + ToolPrefix + `\.zizmor\.` + kebab + `$`)
	actionlintPattern = regexp.MustCompile(`^` + ToolPrefix + `\.actionlint\.` + kebab + `$`)
	shellcheckPattern = regexp.MustCompile(`^` + ToolPrefix + `\.shellcheck\.SC[0-9]{4}$`)
	firstPartyPattern = regexp.MustCompile(`^` + ToolPrefix + `\.` + camel + `$`)
	projectPattern    = regexp.MustCompile(`^([a-z][a-z0-9]*)\.` + camel + `$`)
)

// reservedSecondSegments are the second segments the three bundled backends own; a first-party
// check cannot be named after one of them.
var reservedSecondSegments = []string{"zizmor", "actionlint", "shellcheck"}

// ClassifyIdentifier names the kind of an identifier, or says why it is not one.
func ClassifyIdentifier(id string) (Kind, error) {
	switch {
	case zizmorPattern.MatchString(id):
		return KindZizmor, nil
	case actionlintPattern.MatchString(id):
		return KindActionlint, nil
	case shellcheckPattern.MatchString(id):
		return KindShellcheck, nil
	case firstPartyPattern.MatchString(id):
		return firstPartyKind(id)
	}

	if match := projectPattern.FindStringSubmatch(id); match != nil && match[1] != ToolPrefix {
		return KindProject, nil
	}

	return "", fmt.Errorf("identifier %q is not of a form the scheme defines", id)
}

func firstPartyKind(id string) (Kind, error) {
	name := strings.TrimPrefix(id, ToolPrefix+".")
	for _, reserved := range reservedSecondSegments {
		if name == reserved {
			return "", fmt.Errorf("identifier %q uses %q, which names a bundled backend", id, reserved)
		}
	}

	return KindFirstParty, nil
}
