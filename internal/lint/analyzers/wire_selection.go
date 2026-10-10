package analyzers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// WireSelection validates the compiler input for canonical production choices.
// The plugin replaces it with an invocation-local snapshot before cache lookup.
var WireSelection = &analysis.Analyzer{
	Name:       "wireselection",
	Doc:        "require compiler metadata for canonical Wire production selections",
	ResultType: reflect.TypeOf(wireChoices{}),
	Run: func(pass *analysis.Pass) (any, error) {
		if len(pass.Files) == 0 {
			return wireChoices{}, nil
		}
		return WireSelectionForDirectory(filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)).Run(pass)
	},
}

type wireChoices struct {
	selected map[string]bool
	err      error
}

// WireSelectionForDirectory binds compiler-selected files and resolved objects
// once per plugin invocation. Source contents and failures enter cache identity:
// deleting a selection must invalidate unchanged leaf-package cache entries.
func WireSelectionForDirectory(directory string) *analysis.Analyzer {
	selections, identity, err := loadWireSelections(directory)
	return wireSelectionSnapshot(selections, identity, err)
}

func loadWireSelections(directory string) (map[string]bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	configuration := compilerOwnerTags + "\x00" + os.Getenv("GOFLAGS") + "\x00" + os.Getenv("GOOS") + "\x00" + os.Getenv("GOARCH")
	loaded, err := packages.Load(&packages.Config{
		Context: ctx,
		Dir:     directory,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		BuildFlags: []string{"-tags=" + compilerOwnerTags},
	}, modulePrefix+"pkg/wire/...", modulePrefix+"pkg/root")
	if err != nil {
		return nil, configuration, fmt.Errorf("load Wire compiler metadata: %w", err)
	}
	return collectWireSelections(loaded, configuration)
}

func collectWireSelections(loaded []*packages.Package, configuration string) (map[string]bool, string, error) {
	selections := map[string]bool{}
	var identities []string
	hasWireSource := false
	for _, pkg := range loaded {
		if len(pkg.Errors) > 0 {
			return nil, configuration, fmt.Errorf("Wire compiler metadata for %s: %s", pkg.PkgPath, pkg.Errors[0])
		}
		isRoot := pkg.PkgPath == modulePrefix+"pkg/root"
		if !strings.HasPrefix(pkg.PkgPath, modulePrefix) || (!isRoot && !under(strings.TrimPrefix(pkg.PkgPath, modulePrefix), "pkg/wire")) {
			return nil, configuration, fmt.Errorf("unexpected Wire compiler owner %q", pkg.PkgPath)
		}
		if pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			return nil, configuration, fmt.Errorf("missing Wire syntax/types for %s", pkg.PkgPath)
		}
		for _, file := range pkg.Syntax {
			name := pkg.Fset.Position(file.Package).Filename
			if !wireSelectionSource(name) {
				continue
			}
			// Hash the syntax actually type-checked, rather than rereading a file
			// that could have changed since compiler loading.
			var content bytes.Buffer
			if failure := format.Node(&content, pkg.Fset, file); failure != nil {
				return nil, configuration, fmt.Errorf("encode compiler-selected Wire source: %w", failure)
			}
			digest := sha256.Sum256(content.Bytes())
			identities = append(identities, fmt.Sprintf("%s:%x", name, digest))
			var selected map[string]bool
			if isRoot {
				selected = typedRootEdgeSelections(file, pkg.TypesInfo)
			} else {
				selected = typedWireSelections(file, pkg.TypesInfo)
				hasWireSource = true
			}
			for symbol, chosen := range selected {
				if chosen {
					selections[symbol] = true
				}
			}
		}
	}
	if !hasWireSource {
		return nil, configuration, fmt.Errorf("missing canonical Wire compiler sources")
	}
	sort.Strings(identities)
	return selections, configuration + "\x00" + strings.Join(identities, "\n"), nil
}

func wireSelectionSource(name string) bool {
	return !strings.HasSuffix(name, "_test.go") && filepath.Base(name) != "wire_gen.go"
}

// Only the clock leaf lost its Wire selection in the selected-effects lane.
// Recognize its exact selection in BuildProcess, without extending other
// allowances to unrelated root helpers or package-level defaults.
func typedRootEdgeSelections(file *ast.File, info *types.Info) map[string]bool {
	selected := map[string]bool{}
	var build, normalize *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil {
			continue
		}
		switch fn.Name.Name {
		case "BuildProcess":
			build = fn
		case "normalizeProcessTime":
			normalize = fn
		}
	}
	if build == nil {
		return selected
	}
	symbol := modulePrefix + "pkg/platform/clock.Real"
	selected[symbol] = typedWireSelections(build.Body, info)[symbol]
	if normalize != nil {
		ast.Inspect(build.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if ok && info.Uses[id] == info.Defs[normalize.Name] {
				selected[symbol] = selected[symbol] || typedWireSelections(normalize.Body, info)[symbol]
			}
			return true
		})
	}
	return selected
}

func typedWireSelections(file ast.Node, info *types.Info) map[string]bool {
	selected := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		var expression ast.Expr
		switch node := node.(type) {
		case *ast.CallExpr:
			expression = node.Fun
		case *ast.CompositeLit:
			expression = node.Type
		case *ast.SelectorExpr:
			if symbol := wireQualifiedObject(node, info); symbol == modulePrefix+"pkg/platform/process.NewParentOwnedStdio" {
				selected[symbol] = true
			}
		}
		if symbol := wireQualifiedObject(expression, info); symbol != "" {
			selected[symbol] = true
		}
		return true
	})
	return selected
}

func wireQualifiedObject(expression ast.Expr, info *types.Info) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if _, ok := info.Uses[qualifier].(*types.PkgName); !ok {
		return ""
	}
	object := info.Uses[selector.Sel]
	if object == nil || object.Pkg() == nil || object.Parent() != object.Pkg().Scope() {
		return ""
	}
	return object.Pkg().Path() + "." + object.Name()
}

func wireSelectionSnapshot(selections map[string]bool, identity string, err error) *analysis.Analyzer {
	var symbols []string
	for symbol, selected := range selections {
		if selected {
			symbols = append(symbols, symbol)
		}
	}
	sort.Strings(symbols)
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v", identity, strings.Join(symbols, "\n"), err)))
	copy := analysis.Analyzer{
		Name:       fmt.Sprintf("wireselection_%x", digest[:12]),
		Doc:        "require compiler metadata for canonical Wire production selections",
		ResultType: reflect.TypeOf(wireChoices{}),
	}
	frozen := map[string]bool{}
	for _, symbol := range symbols {
		frozen[symbol] = true
	}
	copy.Run = func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); ok && unit == "internal/lint/analyzers" && err != nil {
			pass.Reportf(pass.Files[0].Package, "wire-selection-metadata: %s; restore canonical Wire compiler inputs", err)
		}
		return wireChoices{selected: frozen, err: err}, nil
	}
	return &copy
}
