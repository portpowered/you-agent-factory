package analyzers

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"gopkg.in/yaml.v3"
)

const packagedSourceBoundary = "packages/packaged-factories/factories"

// PackagedFactorySource checks compiler-selected production literals. Shipped
// catalog inputs are bound separately at plugin construction, before caching.
var PackagedFactorySource = &analysis.Analyzer{
	Name: "packagedfactorysource",
	Doc:  "keep shipped first-party Factory definitions in their authored boundary",
	Run:  runPackagedFactorySource,
}

func runPackagedFactorySource(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	for _, file := range pass.Files {
		name := filepath.Base(pass.Fset.Position(file.Package).Filename)
		if ast.IsGenerated(file) || strings.HasSuffix(strings.ToLower(name), "_test.go") || packagedSourceExcluded(unit) {
			continue
		}
		positions := map[string]token.Pos{}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			var identity struct {
				Name string `yaml:"name"`
			}
			if yaml.Unmarshal([]byte(value), &identity) != nil {
				return true
			}
			name := strings.TrimSpace(identity.Name)
			if strings.HasPrefix(name, "@you/") && positions[name] == token.NoPos {
				positions[name] = literal.Pos()
			}
			return true
		})
		var names []string
		for name := range positions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			pass.Reportf(positions[name], "packaged-factory-source: declares shipped first-party Factory %q outside %s; move the authored definition and owned assets under the required source boundary", name, packagedSourceBoundary)
		}
	}
	return nil, nil
}

func packagedSourceExcluded(unit string) bool {
	unit = strings.ReplaceAll(unit, "\\", "/")
	if under(unit, "factory") || under(unit, packagedSourceBoundary) {
		return true
	}
	for _, part := range strings.Split(unit, "/") {
		switch strings.ToLower(part) {
		case ".git", ".artifacts", "coverage", "dist", "examples", "fixtures", "node_modules", "testdata", "tests", "vendor":
			return true
		}
	}
	return false
}
