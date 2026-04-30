package config

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/internal/contractguard"
)

var retiredAuthoredExhaustionIdentifiers = map[string]struct{}{
	"ExhaustionRules":      {},
	"ExhaustionRuleConfig": {},
}

var approvedTransitionExhaustionSites = map[string]map[string]struct{}{
	filepath.Clean("config/config_mapper.go"): {
		"addDefaultTimeExpiryTransition": {},
	},
	filepath.Clean("factory/subsystems/circuitbreaker.go"): {
		"Execute": {},
	},
	filepath.Clean("petri/transition.go"): {
		"": {},
	},
}

func TestExhaustionRuleContractGuard_RetiredIdentifiersStayOutOfProductionGoFiles(t *testing.T) {
	t.Parallel()

	err := walkProductionPkgFiles(func(path string, _ string, file *ast.File, fset *token.FileSet) error {
		var scanErr error
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, blocked := retiredAuthoredExhaustionIdentifiers[ident.Name]; !blocked {
				return true
			}
			pos := fset.Position(ident.Pos())
			scanErr = fmt.Errorf("%s:%d reintroduces retired authored exhaustion identifier %s", path, pos.Line, ident.Name)
			return false
		})
		return scanErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExhaustionRuleContractGuard_TransitionExhaustionStaysInApprovedProductionSites(t *testing.T) {
	t.Parallel()

	err := walkProductionPkgFiles(func(path, rel string, file *ast.File, fset *token.FileSet) error {
		allowedFunctions := approvedTransitionExhaustionSites[rel]
		return scanTransitionExhaustionUses(file, fset, transitionImportAliases(file), func(funcName string, line int) error {
			if _, ok := allowedFunctions[funcName]; ok {
				return nil
			}
			if len(allowedFunctions) == 0 {
				return fmt.Errorf("%s:%d introduces petri.TransitionExhaustion outside the approved expiry/circuit-breaker sites", path, line)
			}
			if funcName == "" {
				return fmt.Errorf("%s:%d uses petri.TransitionExhaustion outside the approved file-level declaration site", path, line)
			}
			return fmt.Errorf("%s:%d uses petri.TransitionExhaustion in %s; only approved expiry/circuit-breaker sites may keep it", path, line, funcName)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func walkProductionPkgFiles(visit func(path, rel string, file *ast.File, fset *token.FileSet) error) error {
	pkgRoot := filepath.Clean("..")
	fset := token.NewFileSet()

	return filepath.WalkDir(pkgRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if contractguard.ShouldSkipDir(pkgRoot, path, "api/generated") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(pkgRoot, path)
		if err != nil {
			return err
		}
		return visit(path, filepath.Clean(rel), file, fset)
	})
}

func scanTransitionExhaustionUses(file *ast.File, fset *token.FileSet, petriAliases map[string]struct{}, visit func(funcName string, line int) error) error {
	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name != nil {
			funcName = fn.Name.Name
		}
		var scanErr error
		ast.Inspect(decl, func(node ast.Node) bool {
			if scanErr != nil {
				return false
			}
			switch typed := node.(type) {
			case *ast.SelectorExpr:
				ident, ok := typed.X.(*ast.Ident)
				if !ok || typed.Sel == nil || typed.Sel.Name != "TransitionExhaustion" {
					return true
				}
				if _, ok := petriAliases[ident.Name]; !ok {
					return true
				}
				scanErr = visit(funcName, fset.Position(typed.Sel.Pos()).Line)
				return false
			case *ast.Ident:
				if file.Name.Name != "petri" || typed.Name != "TransitionExhaustion" {
					return true
				}
				parentIsSelector := false
				ast.Inspect(decl, func(inner ast.Node) bool {
					selector, ok := inner.(*ast.SelectorExpr)
					if !ok || selector.Sel == nil {
						return true
					}
					if selector.Sel == typed {
						parentIsSelector = true
						return false
					}
					return true
				})
				if parentIsSelector {
					return true
				}
				scanErr = visit(funcName, fset.Position(typed.Pos()).Line)
				return false
			default:
				return true
			}
		})
		if scanErr != nil {
			return scanErr
		}
	}
	return nil
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
