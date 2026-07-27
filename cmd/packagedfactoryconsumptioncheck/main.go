// Command packagedfactoryconsumptioncheck enforces that shipped first-party
// Factory definition bytes are consumed only through the packaged publication
// and catalog loader surfaces.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	modulePath                          = "github.com/portpowered/infinite-you"
	packagedFactoriesImport             = modulePath + "/packages/packaged-factories"
	packagedFactoryCatalogImport        = modulePath + "/internal/packagedfactorycatalog"
	diagnosticPrefix                    = "[agent-factory:packaged-factory-consumption]"
	catalogLoaderSymbol                 = "LoadPublishedDefinitionCatalog"
	packagedFactoriesPublicationSurface = "packages/packaged-factories"
)

var excludedDirectoryNames = map[string]struct{}{
	".git":         {},
	".artifacts":   {},
	"coverage":     {},
	"dist":         {},
	"examples":     {},
	"fixtures":     {},
	"node_modules": {},
	"testdata":     {},
	"tests":        {},
	"vendor":       {},
}

var allowedPackagedFactoriesImporters = map[string]struct{}{
	"packages/packaged-factories":     {},
	"internal/packagedfactorycatalog": {},
}

var allowedCatalogLoaderFiles = map[string]struct{}{
	"pkg/wire/profiles.go": {},
	"pkg/transports/http/handlers_models.go": {},
	"pkg/services/factory_definitions/packages/goal/prompt_drift.go": {},
}

type config struct {
	root string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.Parse()
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("%s resolve repository root: %w", diagnosticPrefix, err)
	}
	violations, err := inspectConsumptionBoundary(repoRoot)
	if err != nil {
		return err
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		return fmt.Errorf("%s consumption boundary failed:\n- %s", diagnosticPrefix, strings.Join(violations, "\n- "))
	}
	fmt.Fprintf(
		stdout,
		"%s packaged Factory consumption is constrained to the embedded catalog surface\n",
		diagnosticPrefix,
	)
	return nil
}

func inspectConsumptionBoundary(repoRoot string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != repoRoot && excludedDirectory(path, entry.Name(), repoRoot) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !isProductionGoFile(relative) {
			return nil
		}
		fileViolations, err := inspectGoFile(path, relative)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", relative, err)
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s scan repository: %w", diagnosticPrefix, err)
	}
	return violations, nil
}

func inspectGoFile(path, relative string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	packagePath := filepath.ToSlash(filepath.Dir(relative))
	var violations []string
	for _, importSpec := range file.Imports {
		importPath, err := strconvUnquote(importSpec.Path.Value)
		if err != nil {
			return nil, err
		}
		switch importPath {
		case packagedFactoriesImport:
			if _, allowed := allowedPackagedFactoriesImporters[packagePath]; !allowed {
				violations = append(violations, directEmbedImportViolation(relative, importPath))
			}
		case packagedFactoryCatalogImport:
			if usesCatalogLoader(file) {
				if _, allowed := allowedCatalogLoaderFiles[relative]; !allowed {
					violations = append(violations, directCatalogLoaderViolation(relative))
				}
			}
		}
	}
	return violations, nil
}

func usesCatalogLoader(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != catalogLoaderSymbol {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok || ident.Name != "packagedfactorycatalog" {
			return true
		}
		found = true
		return false
	})
	return found
}

func directEmbedImportViolation(relative, importPath string) string {
	return fmt.Sprintf(
		"%s imports %s directly; load shipped first-party Factory bytes only through %s and Factory Definitions catalog resolve/install",
		relative,
		importPath,
		packagedFactoriesPublicationSurface,
	)
}

func directCatalogLoaderViolation(relative string) string {
	return fmt.Sprintf(
		"%s calls packagedfactorycatalog.%s outside the approved catalog-consumption surface; route built-in packaged Factory list/resolve/install through Factory Definitions catalog operations",
		relative,
		catalogLoaderSymbol,
	)
}

func isProductionGoFile(relative string) bool {
	lowerName := strings.ToLower(filepath.Base(relative))
	if !strings.HasSuffix(lowerName, ".go") || strings.HasSuffix(lowerName, "_test.go") {
		return false
	}
	return true
}

func excludedDirectory(path, name, repoRoot string) bool {
	if _, excluded := excludedDirectoryNames[strings.ToLower(name)]; excluded {
		return true
	}
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(relative) == "factory"
}

func strconvUnquote(value string) (string, error) {
	if len(value) < 2 {
		return "", fmt.Errorf("invalid quoted import path %q", value)
	}
	return value[1 : len(value)-1], nil
}
