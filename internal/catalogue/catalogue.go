// Package catalogue holds the rules this build ships. It is the one place a rule is added to
// the program: every command that names a rule reads the registry built here.
package catalogue

import "github.com/LongTermSupport/actions-qa-ci/internal/registry"

// Rules returns the rules this build ships, in no particular order. The bundled backends and
// the first-party checks add theirs here as they arrive.
func Rules() []registry.Rule {
	return nil
}

// Registry builds the registry of the rules this build ships.
func Registry() (*registry.Registry, error) {
	return registry.New(Rules()...)
}
