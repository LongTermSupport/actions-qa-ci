// Package actionsqaci is the Defence Before Fix toolchain for GitHub Actions code.
//
// The root package holds what the module declares about itself; the program is in
// cmd/actions-qa.
package actionsqaci

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

//go:embed dbf.json
var declarationJSON []byte

// KnownGap is a place where this toolchain or project does not meet the specification,
// stated rather than hidden.
type KnownGap struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// ProjectDeclaration is the conformance this repository claims for its own working method.
type ProjectDeclaration struct {
	Method    string     `json:"method"`
	Toolchain string     `json:"toolchain"`
	KnownGaps []KnownGap `json:"known-gaps"`
}

// Declaration is the Defence Before Fix conformance declaration at both levels: the artefact
// (what actions-qa-ci is) and the project (how it is worked on).
type Declaration struct {
	Method    string             `json:"method"`
	Toolchain string             `json:"toolchain"`
	KnownGaps []KnownGap         `json:"known-gaps"`
	Project   ProjectDeclaration `json:"project"`
}

// LoadDeclaration reads the declaration embedded from dbf.json.
func LoadDeclaration() (Declaration, error) {
	return ParseDeclaration(declarationJSON)
}

// ParseDeclaration parses a declaration and refuses one with an unknown field or a missing
// one: a misspelt key would otherwise declare nothing without saying so.
func ParseDeclaration(data []byte) (Declaration, error) {
	var declaration Declaration

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declaration); err != nil {
		return Declaration{}, fmt.Errorf("declaration: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Declaration{}, errors.New("declaration: data after the declaration object")
	}

	if declaration.Method == "" || declaration.Toolchain == "" || declaration.KnownGaps == nil {
		return Declaration{}, errors.New("declaration: method, toolchain and known-gaps are all required")
	}
	if declaration.Project.Method == "" || declaration.Project.Toolchain == "" || declaration.Project.KnownGaps == nil {
		return Declaration{}, errors.New("declaration: project method, toolchain and known-gaps are all required")
	}

	return declaration, nil
}
