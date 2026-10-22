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

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CompositeLit:
			if !namesFinding(n.Type) {
				return true
			}
			for _, element := range n.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "ID" && isStringLiteral(pair.Value) {
					report(pair.Value.Pos())
				}
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
	}

	return false
}

func isStringLiteral(expression ast.Expr) bool {
	literal, ok := expression.(*ast.BasicLit)

	return ok && literal.Kind == token.STRING
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
