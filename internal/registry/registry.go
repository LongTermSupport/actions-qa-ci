// Package registry is the one list of rules the program can print findings for. Every
// command that names a rule (rules, rule-doc, rule) answers from it, so a rule cannot exist
// in one place and not another.
package registry

import (
	"cmp"
	"fmt"
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
}

// Registry is an immutable set of rules.
type Registry struct {
	rules []Rule
	byID  map[string]Rule
}

// New builds a registry, refusing a rule that is malformed or that repeats an identifier.
func New(rules ...Rule) (*Registry, error) {
	reg := &Registry{rules: make([]Rule, 0, len(rules)), byID: make(map[string]Rule, len(rules))}

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

func validate(rule Rule) error {
	if _, err := finding.ClassifyIdentifier(rule.ID); err != nil {
		return err
	}
	if strings.TrimSpace(rule.Summary) == "" {
		return fmt.Errorf("rule %s has no summary", rule.ID)
	}
	if strings.ContainsFunc(rule.Summary, finding.IsControlOrSeparator) {
		return fmt.Errorf("rule %s has a summary with a control character or line break; it must print on one line", rule.ID)
	}

	return nil
}

// All returns every rule in identifier order.
func (r *Registry) All() []Rule {
	return slices.Clone(r.rules)
}

// Lookup returns the rule with this identifier, if there is one.
func (r *Registry) Lookup(id string) (Rule, bool) {
	rule, ok := r.byID[id]

	return rule, ok
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

	var candidates []candidate
	for _, rule := range r.rules {
		if distance := editDistance(wanted, strings.ToLower(rule.ID)); distance <= threshold {
			candidates = append(candidates, candidate{rule.ID, distance})
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

// DocPath is where the page for an identifier lives in the repository.
func DocPath(id string) string {
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
