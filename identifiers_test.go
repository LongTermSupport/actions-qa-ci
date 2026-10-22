package actionsqaci_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// literalIdentifierUses reports each place source builds a Finding whose identifier is a
// string literal. An identifier is a constant in the registry; a literal beside the finding
// is how two spellings of one identifier get printed.
func literalIdentifierUses(fset *token.FileSet, file *ast.File) []string {
	var uses []string

	report := func(at token.Pos) {
		uses = append(uses, fset.Position(at).String())
	}

	checkFinding := func(literal *ast.CompositeLit) {
		for position, element := range literal.Elts {
			pair, keyed := element.(*ast.KeyValueExpr)
			if !keyed {
				// ID is the first field, so a positional literal starts with it.
				if position == 0 && isStringLiteral(element) {
					report(element.Pos())
				}

				continue
			}
			if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "ID" && isStringLiteral(pair.Value) {
				report(pair.Value.Pos())
			}
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CompositeLit:
			if namesFinding(n.Type) {
				checkFinding(n)

				return true
			}
			// A literal of findings leaves its elements' type out: []Finding{{ID: …}}.
			for _, element := range elidedFindings(n) {
				checkFinding(element)
			}
		case *ast.AssignStmt:
			for i, target := range n.Lhs {
				selector, ok := target.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "ID" && i < len(n.Rhs) && isStringLiteral(n.Rhs[i]) {
					report(n.Rhs[i].Pos())
				}
			}
		}

		return true
	})

	return uses
}

func namesFinding(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name == "Finding"
	case *ast.SelectorExpr:
		return typed.Sel.Name == "Finding"
	case *ast.StarExpr:
		return namesFinding(typed.X)
	}

	return false
}

// elidedFindings returns the type-less element literals of a slice, array or map of
// findings; deeper nesting is reached because Inspect visits those literals in turn.
func elidedFindings(literal *ast.CompositeLit) []*ast.CompositeLit {
	var elementType ast.Expr
	switch typed := literal.Type.(type) {
	case *ast.ArrayType:
		elementType = typed.Elt
	case *ast.MapType:
		elementType = typed.Value
	default:
		return nil
	}

	var elided []*ast.CompositeLit
	for _, element := range literal.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			element = pair.Value
		}
		inner, ok := element.(*ast.CompositeLit)
		if !ok || inner.Type != nil {
			continue
		}
		if namesFinding(elementType) {
			elided = append(elided, inner)
		} else if nested := elidedElements(elementType, inner); nested != nil {
			elided = append(elided, nested...)
		}
	}

	return elided
}

// elidedElements handles a literal whose own element type is a slice, array or map of
// findings: [][]Finding{{{ID: …}}}.
func elidedElements(elementType ast.Expr, literal *ast.CompositeLit) []*ast.CompositeLit {
	typed := &ast.CompositeLit{Type: elementType, Elts: literal.Elts}

	return elidedFindings(typed)
}

// isStringLiteral is true for a string written out in full: a literal, a parenthesised
// one, a concatenation of literals, or a string(...) conversion of one. A name or a
// concatenation with a non-literal operand is how an identifier is built from a constant.
func isStringLiteral(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.BasicLit:
		return typed.Kind == token.STRING
	case *ast.ParenExpr:
		return isStringLiteral(typed.X)
	case *ast.BinaryExpr:
		return typed.Op == token.ADD && isStringLiteral(typed.X) && isStringLiteral(typed.Y)
	case *ast.CallExpr:
		conversion, ok := typed.Fun.(*ast.Ident)

		return ok && conversion.Name == "string" && len(typed.Args) == 1 && isStringLiteral(typed.Args[0])
	}

	return false
}

func usesIn(t *testing.T, source string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "case.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}

	return literalIdentifierUses(fset, file)
}

func TestTheScannerFindsAFindingBuiltWithALiteralIdentifier(t *testing.T) {
	cases := map[string]string{
		"keyed literal":     `package p; var f = finding.Finding{ID: "actionsqaci.templateDrift"}`,
		"unqualified type":  `package p; var f = Finding{ID: "actionsqaci.templateDrift"}`,
		"later assignment":  `package p; func g(f *finding.Finding) { f.ID = "actionsqaci.templateDrift" }`,
		"pointer to struct": `package p; var f = &finding.Finding{ID: "actionsqaci.templateDrift"}`,
		"slice element":     `package p; var f = []finding.Finding{{ID: "actionsqaci.templateDrift"}}`,
		"pointer element":   `package p; var f = []*finding.Finding{{ID: "actionsqaci.templateDrift"}}`,
		"map value":         `package p; var f = map[string]finding.Finding{"k": {ID: "actionsqaci.templateDrift"}}`,
		"array element":     `package p; var f = [1]finding.Finding{{ID: "actionsqaci.templateDrift"}}`,
		"positional field":  `package p; var f = finding.Finding{"actionsqaci.templateDrift", "tool"}`,
		"parenthesised":     `package p; var f = finding.Finding{ID: ("actionsqaci.templateDrift")}`,
		"concatenated":      `package p; var f = finding.Finding{ID: "actionsqaci." + "templateDrift"}`,
		"converted":         `package p; var f = finding.Finding{ID: string("actionsqaci.templateDrift")}`,
		"nested element":    `package p; var f = [][]finding.Finding{{{ID: "actionsqaci.templateDrift"}}}`,
	}

	for name, source := range cases {
		if got := usesIn(t, source); len(got) != 1 {
			t.Errorf("%s: found %d uses, want 1", name, len(got))
		}
	}
}

func TestTheScannerAcceptsAFindingBuiltFromAConstant(t *testing.T) {
	sources := []string{
		`package p; const templateDrift = "actionsqaci.templateDrift"; var f = finding.Finding{ID: templateDrift}`,
		`package p; func g(f *finding.Finding) { f.ID = registry.IDTemplateDrift }`,
		`package p; var f = finding.Finding{Message: "actionsqaci.templateDrift"}`,
		`package p; var r = registry.Rule{ID: "actionsqaci.templateDrift"}`,
		`package p; var r = []registry.Rule{{ID: "actionsqaci.templateDrift"}}`,
		`package p; var f = finding.Finding{ID: prefix + audit}`,
		`package p; var f = []finding.Finding{{ID: templateDrift}}`,
	}

	for _, source := range sources {
		if got := usesIn(t, source); len(got) != 0 {
			t.Errorf("%s: reported %v", source, got)
		}
	}
}

func TestNoProductionCodeBuildsAFindingWithALiteralIdentifier(t *testing.T) {
	var offences []string

	walkErr := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "untracked" || name == "testdata" {
				return filepath.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		offences = append(offences, literalIdentifierUses(fset, file)...)

		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}

	if len(offences) != 0 {
		t.Errorf("a Finding is built with a literal identifier at %s; declare the identifier as a constant in the registry",
			strconv.Quote(strings.Join(offences, ", ")))
	}
}
