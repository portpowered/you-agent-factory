package petri

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/handwrittensourceguard"
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
	err := walkTransitionGuardProductionFiles(moduleRoot, func(path string, file *ast.File) error {
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

func TestTransitionContractGuard_SkipsHiddenMetadataDirs(t *testing.T) {
	t.Parallel()

	moduleRoot := t.TempDir()
	for path, contents := range map[string]string{
		"pkg/feature/kept.go":                "package feature\n",
		".claude/worktrees/stale/ignored.go": "package stale\n",
		".git/hooks/ignored.go":              "package hooks\n",
		".worktrees/nested/ignored.go":       "package nested\n",
	} {
		fullPath := filepath.Join(moduleRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create parent dir for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	var visited []string
	if err := walkTransitionGuardProductionFiles(moduleRoot, func(path string, _ *ast.File) error {
		rel, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		visited = append(visited, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("walk transition guard files: %v", err)
	}

	if !slices.Contains(visited, "pkg/feature/kept.go") {
		t.Fatalf("expected handwritten source file to be visited, got %v", visited)
	}
	for _, skipped := range []string{
		".claude/worktrees/stale/ignored.go",
		".git/hooks/ignored.go",
		".worktrees/nested/ignored.go",
	} {
		if slices.Contains(visited, skipped) {
			t.Fatalf("expected hidden metadata path %s to be skipped, got %v", skipped, visited)
		}
	}
}

func walkTransitionGuardProductionFiles(moduleRoot string, visit func(path string, file *ast.File) error) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if handwrittensourceguard.ShouldSkipDir("pkg/petri/transition_contract_guard_test.go", moduleRoot, path) {
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
		return visit(path, file)
	})
}

func transitionImportAliases(file *ast.File) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"github.com/portpowered/infinite-you/pkg/petri"` {
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
