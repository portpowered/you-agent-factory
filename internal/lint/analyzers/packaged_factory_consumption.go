package analyzers

import (
	"go/ast"
	"go/parser"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// PackagedFactoryConsumption keeps shipped Factory bytes behind publication
// and Factory Definitions catalog operations, without a debt allowance.
var PackagedFactoryConsumption = &analysis.Analyzer{
	Name: "packagedfactoryconsumption",
	Doc:  "enforce packaged Factory publication and catalog consumption boundaries",
	Run:  runPackagedFactoryConsumption,
}

const packagedPublication = "github.com/portpowered/infinite-you/packages/packaged-factories"
const packagedCatalog = "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
const publishedLoader = "LoadPublishedDefinitionCatalog"

var packagedLoaderFiles = setOf(
	"pkg/wire/profiles.go",
	"pkg/transports/http/handlers_models.go",
	"pkg/services/factory_definitions/internal/services/distribution/goal/prompt_drift.go",
)

func runPackagedFactoryConsumption(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	unit = strings.TrimSuffix(unit, "_test")
	visited := map[string]bool{}
	for _, file := range pass.Files {
		name := filepath.Clean(pass.Fset.Position(file.Pos()).Filename)
		visited[name] = true
		path := sourceName(unit, name)
		if !packagedConsumptionSource(path) || ast.IsGenerated(file) {
			continue
		}
		packagedPublicationImports(pass, file, unit, path)
		if packagedLoaderFiles[path] {
			continue
		}
		packagedCatalogCalls(pass, file, path)
	}
	// Only compiler-owned ignored/other Go filenames are read. ImportsOnly
	// preserves the import guard across tags without parsing excluded bodies.
	for _, name := range slices.Concat(pass.IgnoredFiles, pass.OtherFiles) {
		name = filepath.Clean(name)
		path := sourceName(unit, name)
		if visited[name] || !packagedConsumptionSource(path) {
			continue
		}
		visited[name] = true
		contents, err := pass.ReadFile(name)
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(pass.Fset, name, contents, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if !ast.IsGenerated(file) {
			packagedPublicationImports(pass, file, unit, path)
		}
	}
	return nil, nil
}

func packagedCatalogCalls(pass *analysis.Pass, file *ast.File, path string) {
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun, ok := packagedCallObject(pass, ast.Unparen(call.Fun)).(*types.Func)
		if ok && fun.Pkg() != nil && fun.Pkg().Path() == packagedCatalog &&
			fun.Parent() == fun.Pkg().Scope() && fun.Name() == publishedLoader {
			pass.Reportf(call.Pos(), "packaged-factory-catalog-loader: %s calls the published catalog loader outside the approved catalog-consumption surface; route built-in packaged Factory list/resolve/install through Factory Definitions catalog operations", path)
		}
		return true
	})
}

func packagedPublicationImports(pass *analysis.Pass, file *ast.File, unit, path string) {
	if unit == "packages/packaged-factories" || unit == "internal/packagedfactorycatalog" {
		return
	}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err == nil && imported == packagedPublication {
			pass.Reportf(spec.Pos(), "packaged-factory-direct-publication: %s imports %s directly; load shipped first-party Factory bytes only through packages/packaged-factories and Factory Definitions catalog resolve/install", path, imported)
		}
	}
}

func packagedConsumptionSource(path string) bool {
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, "_test.go") || under(path, "factory") {
		return false
	}
	for _, segment := range strings.Split(lower, "/") {
		switch segment {
		case ".git", ".artifacts", "coverage", "dist", "examples", "fixtures", "node_modules", "testdata", "tests", "vendor":
			return false
		}
	}
	return true
}

func packagedCallObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch expr := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[expr]
	case *ast.SelectorExpr:
		return pass.TypesInfo.Uses[expr.Sel]
	}
	return nil
}
