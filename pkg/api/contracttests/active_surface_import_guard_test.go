package apicontract_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

var activeSurfaceScanRoots = []string{
	"pkg/api",
	"pkg/apisurface",
	"pkg/cli",
	"pkg/mcp",
	"pkg/factorysessionexecution",
	"pkg/factorysessions",
}

var deprecatedRootWorkflowImportPrefixes = []string{
	"github.com/portpowered/infinite-you/pkg/workflowpreview",
	"github.com/portpowered/infinite-you/pkg/workflowsource",
	"github.com/portpowered/infinite-you/pkg/workflowvalidation",
	"github.com/portpowered/infinite-you/pkg/workflowpolicy",
	"github.com/portpowered/infinite-you/pkg/workflowresult",
}

var requiredOrchestratorOwnershipPackages = []string{
	"source",
	"validation",
	"policy",
	"preview",
	"result",
	"store",
}

func TestActiveSurfaceImportGuard_NoRootWorkflowImportsInScopedPackages(t *testing.T) {
	t.Parallel()

	moduleRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	fset := token.NewFileSet()

	for _, scanRoot := range activeSurfaceScanRoots {
		scanRoot := scanRoot
		t.Run(scanRoot, func(t *testing.T) {
			t.Parallel()

			absRoot := filepath.Join(moduleRoot, scanRoot)
			err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					if contractguard.ShouldSkipDir(absRoot, path, "generated") {
						return filepath.SkipDir
					}
					return nil
				}
				if filepath.Ext(path) != ".go" {
					return nil
				}
				if isDeprecatedWorkflowShimCompatibilityTest(path) {
					return nil
				}

				file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imp := range file.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)
					if isDeprecatedRootWorkflowImport(importPath) {
						rel, _ := filepath.Rel(moduleRoot, path)
						return fmt.Errorf(
							"%s imports deprecated root workflow shim %s; use pkg/orchestrators/javascript/* instead",
							rel,
							importPath,
						)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestActiveSurfaceImportGuard_OrchestratorOwnershipPackagesRemainPresent(t *testing.T) {
	t.Parallel()

	moduleRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	orchestratorRoot := filepath.Join(moduleRoot, "pkg", "orchestrators", "javascript")

	for _, pkg := range requiredOrchestratorOwnershipPackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			pkgDir := filepath.Join(orchestratorRoot, pkg)
			info, err := os.Stat(pkgDir)
			if err != nil || !info.IsDir() {
				t.Fatalf("pkg/orchestrators/javascript/%s is missing", pkg)
			}

			entries, err := os.ReadDir(pkgDir)
			if err != nil {
				t.Fatalf("read pkg/orchestrators/javascript/%s: %v", pkg, err)
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
					return
				}
			}
			t.Fatalf("pkg/orchestrators/javascript/%s has no Go source files", pkg)
		})
	}
}

func isDeprecatedRootWorkflowImport(importPath string) bool {
	for _, prefix := range deprecatedRootWorkflowImportPrefixes {
		if importPath == prefix {
			return true
		}
	}
	return false
}

func isDeprecatedWorkflowShimCompatibilityTest(path string) bool {
	base := filepath.Base(path)
	if !strings.HasSuffix(base, "_test.go") {
		return false
	}
	if strings.Contains(base, "compat") || strings.Contains(base, "compatibility") {
		return true
	}

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false
	}
	for _, commentGroup := range file.Comments {
		for _, comment := range commentGroup.List {
			lower := strings.ToLower(comment.Text)
			if strings.Contains(lower, "compatibility shim") || strings.Contains(lower, "compatibility-only") {
				return true
			}
		}
	}
	return false
}
