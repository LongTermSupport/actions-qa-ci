// Package registry is the one list of rules the program can print findings for. Every
// command that names a rule (rules, rule-doc, rule) answers from it, so a rule cannot exist
// in one place and not another.
package registry

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Rule is one defence: an identifier and the standing instruction it enforces.
type Rule struct {
	ID string
	// Summary is one line stating what the rule requires, as an instruction a reader can follow.
	Summary string
	// Upstream names the tool and version a bundled rule comes from; empty for first-party rules.
	Upstream string
	// Absorbs says which reports of another tool this rule stands for, because the hazard has one
	// owner and the other tool's report of it is dropped. Each prints on one line.
	Absorbs []string
	// Check runs the rule on one file, given its repository-relative path, and returns only
	// findings carrying this rule's identifier. It is nil for a rule that cannot run on its
	// own, such as one a bundled backend reports only as part of its lane.
	Check func(path string) ([]finding.Finding, error)
}

// Family is a set of rules too many to list one by one, such as ShellCheck's codes. They share
// one statement, one page and one way of running, and a member is any identifier Member accepts.
type Family struct {
	// Name is how the family is listed, with a placeholder for the part that varies.
	Name string
	// Page is the stem of the page the members share, under docs/rules.
	Page string
	// Summary is the standing instruction every member states.
	Summary string
	// Upstream names the tool and version the members come from.
	Upstream string
	// Member says whether an identifier belongs to the family.
	Member func(id string) bool
	// Check returns the check for one member, given its identifier.
	Check func(id string) func(path string) ([]finding.Finding, error)
}

// Registry is an immutable set of rules and families of rules.
type Registry struct {
	rules    []Rule
	byID     map[string]Rule
	families []Family
}

// New builds a registry, refusing a rule that is malformed or that repeats an identifier.
func New(rules ...Rule) (*Registry, error) {
	return NewWithFamilies(nil, rules...)
}

// NewWithFamilies builds a registry of individual rules and of families, refusing anything
// malformed or repeated. A family's members are not individually registered.
func NewWithFamilies(families []Family, rules ...Rule) (*Registry, error) {
	reg := &Registry{rules: make([]Rule, 0, len(rules)), byID: make(map[string]Rule, len(rules))}

	for _, family := range families {
		if err := validateFamily(family, reg.families); err != nil {
			return nil, err
		}
		reg.families = append(reg.families, family)
	}
	slices.SortFunc(reg.families, func(a, b Family) int { return cmp.Compare(a.Name, b.Name) })

	for _, rule := range rules {
		if err := validate(rule); err != nil {
			return nil, err
		}
		if _, repeated := reg.byID[rule.ID]; repeated {
			return nil, fmt.Errorf("rule %s is registered twice", rule.ID)
		}
		reg.byID[rule.ID] = rule
		reg.rules = append(reg.rules, rule)
	}

	slices.SortFunc(reg.rules, func(a, b Rule) int { return cmp.Compare(a.ID, b.ID) })

	return reg, nil
}

func validateFamily(family Family, earlier []Family) error {
	switch {
	case strings.TrimSpace(family.Name) == "":
		return errors.New("a family has no name")
	case strings.TrimSpace(family.Page) == "" || family.Page != path.Base(family.Page) || strings.Contains(family.Page, "\\"):
		return fmt.Errorf("family %s has no page stem, or one that is a path", family.Name)
	case strings.TrimSpace(family.Summary) == "":
		return fmt.Errorf("family %s has no summary", family.Name)
	case strings.ContainsFunc(family.Summary, finding.IsControlOrSeparator):
		return fmt.Errorf("family %s has a summary with a control character or line break; it must print on one line", family.Name)
	case family.Member == nil:
		return fmt.Errorf("family %s cannot say which identifiers belong to it", family.Name)
	case family.Check == nil:
		return fmt.Errorf("family %s cannot run", family.Name)
	}

	for _, other := range earlier {
		if other.Name == family.Name || other.Page == family.Page {
			return fmt.Errorf("family %s repeats the name or page of family %s", family.Name, other.Name)
		}
	}

	return nil
}

func validate(rule Rule) error {
	kind, err := finding.ClassifyIdentifier(rule.ID)
	if err != nil {
		return err
	}
	if kind == finding.KindShellcheck {
		return fmt.Errorf("rule %s is a ShellCheck code; register the family, not the code", rule.ID)
	}
	if strings.TrimSpace(rule.Summary) == "" {
		return fmt.Errorf("rule %s has no summary", rule.ID)
	}
	if strings.ContainsFunc(rule.Summary, finding.IsControlOrSeparator) {
		return fmt.Errorf("rule %s has a summary with a control character or line break; it must print on one line", rule.ID)
	}
	for _, absorbed := range rule.Absorbs {
		if strings.TrimSpace(absorbed) == "" || strings.ContainsFunc(absorbed, finding.IsControlOrSeparator) {
			return fmt.Errorf("rule %s absorbs a report that is blank or has a control character or line break; it must print on one line", rule.ID)
		}
	}

	return nil
}

// All returns every rule in identifier order.
func (r *Registry) All() []Rule {
	return slices.Clone(r.rules)
}

// FamilyNamed returns the family listed under this name, such as the name `rules` prints.
func (r *Registry) FamilyNamed(name string) (Family, bool) {
	for _, family := range r.families {
		if family.Name == name {
			return family, true
		}
	}

	return Family{}, false
}

// Families returns every family in name order.
func (r *Registry) Families() []Family {
	return slices.Clone(r.families)
}

// Lookup returns the rule with this identifier, if there is one. A member of a family is
// returned as a rule carrying the family's statement and a check for that member.
func (r *Registry) Lookup(id string) (Rule, bool) {
	if rule, ok := r.byID[id]; ok {
		return rule, true
	}

	if _, err := finding.ClassifyIdentifier(id); err != nil {
		return Rule{}, false
	}
	for _, family := range r.families {
		if family.Member(id) {
			return Rule{ID: id, Summary: family.Summary, Upstream: family.Upstream, Check: family.Check(id)}, true
		}
	}

	return Rule{}, false
}

// Nearest returns up to limit registered identifiers that resemble id, closest first. It
// returns nothing when none is close: a wild guess is worse than no suggestion.
func (r *Registry) Nearest(id string, limit int) []string {
	if limit <= 0 {
		return nil
	}

	type candidate struct {
		id       string
		distance int
	}

	wanted := strings.ToLower(id)
	threshold := len(id) / 3

	names := make([]string, 0, len(r.rules)+len(r.families))
	for _, rule := range r.rules {
		names = append(names, rule.ID)
	}
	for _, family := range r.families {
		names = append(names, family.Name)
	}

	var candidates []candidate
	for _, name := range names {
		if distance := editDistance(wanted, strings.ToLower(name)); distance <= threshold {
			candidates = append(candidates, candidate{name, distance})
		}
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.distance, b.distance), cmp.Compare(a.id, b.id))
	})

	suggestions := make([]string, 0, min(limit, len(candidates)))
	for _, c := range candidates[:min(limit, len(candidates))] {
		suggestions = append(suggestions, c.id)
	}

	return suggestions
}

// DocPath is where the page for an identifier lives in the repository. Every ShellCheck code
// shares the page of the family.
func DocPath(id string) string {
	if kind, err := finding.ClassifyIdentifier(id); err == nil && kind == finding.KindShellcheck {
		return "docs/rules/" + finding.ToolPrefix + ".shellcheck.md"
	}

	return "docs/rules/" + id + ".md"
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			substitution := previous[j-1]
			if a[i-1] != b[j-1] {
				substitution++
			}
			current[j] = min(previous[j]+1, current[j-1]+1, substitution)
		}
		previous = current
	}

	return previous[len(b)]
}
