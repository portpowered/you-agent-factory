package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type functionalGoListPackage struct {
	Dir          string
	ImportPath   string
	TestGoFiles  []string
	XTestGoFiles []string
}

// discoverFunctionalTestInventory uses go list's build-selected test file
// sets as the source of truth. Parsing those files avoids compiling and
// linking every functional test binary merely to enumerate its top-level
// tests.
func discoverFunctionalTestInventory(packages []string, _ time.Duration, _ bool, _ int, repoRoot string) (functionalTestInventory, error) {
	requestedPackages := sortedUniqueStrings(packages)
	return discoverFunctionalTestInventoryWithPatterns(requestedPackages, requestedPackages, repoRoot)
}

func discoverFunctionalTestInventoryWithPatterns(listPatterns, packages []string, repoRoot string) (functionalTestInventory, error) {
	requestedPackages := sortedUniqueStrings(packages)
	if len(requestedPackages) == 0 {
		return functionalTestInventory{}, errors.New("discover functional tests: no packages were selected")
	}

	listedPackages, err := listFunctionalTestPackages(listPatterns, repoRoot)
	if err != nil {
		return functionalTestInventory{}, err
	}
	listedPackages, err = selectFunctionalPackageSet(requestedPackages, listedPackages)
	if err != nil {
		return functionalTestInventory{}, err
	}

	inventory := functionalTestInventory{
		Packages: make([]string, 0, len(listedPackages)),
		Tests:    make(map[string][]string, len(listedPackages)),
	}
	for _, pkg := range listedPackages {
		tests, err := discoverFunctionalPackageTests(pkg)
		if err != nil {
			return functionalTestInventory{}, err
		}
		inventory.Packages = append(inventory.Packages, pkg.ImportPath)
		inventory.Tests[pkg.ImportPath] = tests
	}
	sort.Strings(inventory.Packages)
	return inventory, nil
}

func listFunctionalTestPackages(packages []string, repoRoot string) ([]functionalGoListPackage, error) {
	// The inventory only needs package locations and build-selected test file
	// names. -find keeps go list from resolving imports and dependencies for
	// every functional package; the AST parser below remains responsible for
	// validating the selected source files.
	args := append([]string{"list", "-json", "-find"}, packages...)
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  repoRoot,
	})
	if err != nil {
		detail := mergeGoTestFailureDetail(stderr, stdout)
		if detail != "" {
			return nil, fmt.Errorf("discover functional tests: go list: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("discover functional tests: go list: %w", err)
	}

	decoder := json.NewDecoder(strings.NewReader(stdout))
	byImportPath := make(map[string]functionalGoListPackage)
	for {
		var pkg functionalGoListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("discover functional tests: decode go list package: %w", err)
		}
		if strings.TrimSpace(pkg.ImportPath) == "" {
			return nil, errors.New("discover functional tests: go list returned a package without an import path")
		}
		if strings.TrimSpace(pkg.Dir) == "" {
			return nil, fmt.Errorf("discover functional tests: go list package %q did not include a directory", pkg.ImportPath)
		}
		if previous, exists := byImportPath[pkg.ImportPath]; exists {
			if filepath.Clean(previous.Dir) != filepath.Clean(pkg.Dir) {
				return nil, fmt.Errorf("discover functional tests: go list returned package %q with conflicting directories %q and %q", pkg.ImportPath, previous.Dir, pkg.Dir)
			}
			previous.TestGoFiles = append(previous.TestGoFiles, pkg.TestGoFiles...)
			previous.XTestGoFiles = append(previous.XTestGoFiles, pkg.XTestGoFiles...)
			byImportPath[pkg.ImportPath] = previous
			continue
		}
		byImportPath[pkg.ImportPath] = pkg
	}

	listed := make([]functionalGoListPackage, 0, len(byImportPath))
	for _, pkg := range byImportPath {
		pkg.TestGoFiles = sortedUniqueStrings(pkg.TestGoFiles)
		pkg.XTestGoFiles = sortedUniqueStrings(pkg.XTestGoFiles)
		listed = append(listed, pkg)
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].ImportPath < listed[j].ImportPath })
	return listed, nil
}

func selectFunctionalPackageSet(requested []string, listed []functionalGoListPackage) ([]functionalGoListPackage, error) {
	requestedSet := make(map[string]struct{}, len(requested))
	for _, packagePath := range requested {
		requestedSet[packagePath] = struct{}{}
	}
	listedByImportPath := make(map[string]functionalGoListPackage, len(listed))
	for _, pkg := range listed {
		if _, requested := requestedSet[pkg.ImportPath]; requested {
			listedByImportPath[pkg.ImportPath] = pkg
		}
	}
	selected := make([]functionalGoListPackage, 0, len(requested))
	for _, packagePath := range requested {
		pkg, found := listedByImportPath[packagePath]
		if !found {
			return nil, fmt.Errorf("discover functional tests: go list did not report requested package %q", packagePath)
		}
		selected = append(selected, pkg)
	}
	return selected, nil
}

func discoverFunctionalPackageTests(pkg functionalGoListPackage) ([]string, error) {
	testFiles := append(append([]string(nil), pkg.TestGoFiles...), pkg.XTestGoFiles...)
	testFiles = sortedUniqueStrings(testFiles)
	tests := make([]string, 0)
	for _, listedFile := range testFiles {
		filePath := filepath.Join(pkg.Dir, listedFile)
		source, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("discover functional tests: read package %q file %q: %w", pkg.ImportPath, listedFile, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filePath, source, 0)
		if err != nil {
			return nil, fmt.Errorf("discover functional tests: parse package %q file %q: %w", pkg.ImportPath, listedFile, err)
		}
		tests = append(tests, functionalTestDeclarations(parsed)...)
	}
	return sortedUniqueStrings(tests), nil
}

func functionalTestDeclarations(file *ast.File) []string {
	testingNames := testingImportNames(file)
	tests := make([]string, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || !functionalTestNamePattern.MatchString(function.Name.Name) {
			continue
		}
		if functionalTestSignature(function, testingNames) {
			tests = append(tests, function.Name.Name)
		}
	}
	return tests
}

func testingImportNames(file *ast.File) map[string]struct{} {
	names := make(map[string]struct{})
	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || path != "testing" {
			continue
		}
		if importSpec.Name == nil {
			names["testing"] = struct{}{}
			continue
		}
		if importSpec.Name.Name != "_" {
			names[importSpec.Name.Name] = struct{}{}
		}
	}
	return names
}

func functionalTestSignature(function *ast.FuncDecl, testingNames map[string]struct{}) bool {
	if function.Type.TypeParams != nil || function.Type.Results != nil {
		return false
	}
	if function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) > 1 {
		return false
	}
	parameterType, ok := unwrapFunctionalParenExpr(parameter.Type).(*ast.StarExpr)
	if !ok {
		return false
	}
	testingType := unwrapFunctionalParenExpr(parameterType.X)
	if selected, ok := testingType.(*ast.SelectorExpr); ok {
		packageName, ok := selected.X.(*ast.Ident)
		if !ok || selected.Sel.Name != "T" {
			return false
		}
		_, imported := testingNames[packageName.Name]
		return imported
	}
	identifier, ok := testingType.(*ast.Ident)
	if !ok || identifier.Name != "T" {
		return false
	}
	_, dotImported := testingNames["."]
	return dotImported
}

func unwrapFunctionalParenExpr(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}
