package petri

import (
	"github.com/portpowered/agent-factory/internal/contractguard"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var retiredTransitionRuntimeFields = map[string]struct{}{
	"WorkstationKind": {},
	"Limits":          {},
	"StopWords":       {},
}

func TestTransitionContractGuard_RuntimeOwnedFieldsStayDeleted(t *testing.T) {
	t.Parallel()

	transitionType := reflect.TypeOf(Transition{})
	for fieldName := range retiredTransitionRuntimeFields {
		if _, ok := transitionType.FieldByName(fieldName); ok {
			t.Fatalf("petri.Transition must not expose retired runtime-owned field %s", fieldName)
		}
	}
}

func TestTransitionContractGuard_ProductionTransitionLiteralsStayTopologyOnly(t *testing.T) {
	t.Parallel()

	moduleRoot := filepath.Clean(filepath.Join("..", ".."))
	fset := token.NewFileSet()
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if contractguard.ShouldSkipDir(moduleRoot, path, "pkg/api/generated", "ui/dist", "ui/node_modules", "ui/storybook-static") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "transition_contract_guard_test.go" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		petriAliases := transitionImportAliases(file)
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !isTransitionLiteral(file.Name.Name, lit.Type, petriAliases) {
				return true
			}
			for _, elt := range lit.Elts {
				keyValue, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := keyValue.Key.(*ast.Ident)
				if !ok {
					continue
				}
				if _, blocked := retiredTransitionRuntimeFields[key.Name]; blocked {
					t.Fatalf("%s reintroduces retired petri.Transition field %s in a production literal", path, key.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan production transition literals: %v", err)
	}
}

func transitionImportAliases(file *ast.File) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"github.com/portpowered/agent-factory/pkg/petri"` {
			continue
		}
		name := "petri"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = struct{}{}
	}
	return aliases
}

func isTransitionLiteral(packageName string, expr ast.Expr, petriAliases map[string]struct{}) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return packageName == "petri" && typed.Name == "Transition"
	case *ast.SelectorExpr:
		ident, ok := typed.X.(*ast.Ident)
		if !ok || typed.Sel == nil || typed.Sel.Name != "Transition" {
			return false
		}
		_, ok = petriAliases[ident.Name]
		return ok
	default:
		return false
	}
}
