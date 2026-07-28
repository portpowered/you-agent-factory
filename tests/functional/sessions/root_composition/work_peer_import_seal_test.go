package root_composition_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	modulePrefix           = "github.com/portpowered/infinite-you/"
	workServiceRoot        = modulePrefix + "pkg/services/work"
	factorySessionsRoot    = modulePrefix + "pkg/services/factory_sessions"
	factoryRuntimeRoot     = modulePrefix + "pkg/services/factory_runtime"
	workersServiceRoot     = modulePrefix + "pkg/services/workers"
	functionalSessionsRoot = "tests/functional/sessions"
)

var retiredWorkConsumerImportRoots = []string{
	modulePrefix + "pkg/work",
	modulePrefix + "pkg/workcontent",
	modulePrefix + "pkg/workgraph",
	modulePrefix + "pkg/workquery",
}

// TestFactorySessionsProductionPackagesImportWorkRootOnly proves CUT-SES-WORK
// remains closed: Factory Sessions production packages depend on Work only
// through the published service root, not nested Work implementation or legacy
// pkg/work* consumer edges.
func TestFactorySessionsProductionPackagesImportWorkRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactorySessionsPackages(t) {
		pkg := pkg
		t.Run(shortFactorySessionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseWorkRootOnly(t, pkg)
		})
	}
}

// TestFactorySessionsProductionPackagesImportWorkersOnlyThroughRoot proves
// CUT-SES-WRK remains closed under the FUN-sessions proof surface.
func TestFactorySessionsProductionPackagesImportWorkersOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactorySessionsPackages(t) {
		pkg := pkg
		t.Run(shortFactorySessionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseServiceRootOnly(t, pkg, workersServiceRoot, "Workers")
		})
	}
}

// TestFactorySessionsProductionPackagesImportFactoryRuntimeOnlyThroughRoot
// proves CUT-SES-RUN remains closed under the FUN-sessions proof surface.
func TestFactorySessionsProductionPackagesImportFactoryRuntimeOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactorySessionsPackages(t) {
		pkg := pkg
		t.Run(shortFactorySessionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseServiceRootOnly(t, pkg, factoryRuntimeRoot, "Factory Runtime")
		})
	}
}

// TestSessionsFunctionalProofsDoNotImportRetiredWorkConsumerEdges proves FUN-scoped
// functional proofs under tests/functional/sessions do not reintroduce retired
// Sessions→Work consumer edges (legacy pkg/work* paths or nested Work packages).
func TestSessionsFunctionalProofsDoNotImportRetiredWorkConsumerEdges(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	functionalRoot := filepath.Join(repoRoot, filepath.FromSlash(functionalSessionsRoot))

	err := filepath.WalkDir(functionalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		assertGoSourceDoesNotImportForbiddenWorkPaths(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", functionalRoot, err)
	}
}

func listFactorySessionsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", factorySessionsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory_sessions packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertProductionImportsUseWorkRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listPackageImports(t, packagePath) {
		if isForbiddenFactorySessionsWorkImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Work surfaces",
				packagePath,
				importPath,
				workServiceRoot,
			)
		}
	}
}

func assertProductionImportsUseServiceRootOnly(
	t *testing.T,
	packagePath string,
	serviceRoot string,
	serviceLabel string,
) {
	t.Helper()

	for _, importPath := range listPackageImports(t, packagePath) {
		if importPath == serviceRoot {
			continue
		}
		if strings.HasPrefix(importPath, serviceRoot+"/") {
			t.Fatalf(
				"%s must import %s only through %s; found direct import %s",
				packagePath,
				serviceLabel,
				serviceRoot,
				importPath,
			)
		}
	}
}

func listPackageImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(string(output))
}

func isForbiddenFactorySessionsWorkImport(importPath string) bool {
	if importPath == workServiceRoot {
		return false
	}
	if strings.HasPrefix(importPath, workServiceRoot+"/") {
		return true
	}
	for _, legacyRoot := range retiredWorkConsumerImportRoots {
		if importPath == legacyRoot || strings.HasPrefix(importPath, legacyRoot+"/") {
			return true
		}
	}
	return false
}

func assertGoSourceDoesNotImportForbiddenWorkPaths(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		if isForbiddenFactorySessionsWorkImport(importPath) {
			t.Fatalf(
				"%s imports forbidden Work consumer edge %s; use %s for Work surfaces",
				path,
				importPath,
				workServiceRoot,
			)
		}
	}
}

func shortFactorySessionsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, factorySessionsRoot) {
		rest := strings.TrimPrefix(packagePath, factorySessionsRoot)
		if rest == "" {
			return "factory_sessions"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
