package factory_visualization_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix             = "github.com/portpowered/infinite-you/"
	factoryVisualizationRoot = modulePrefix + "pkg/services/factory_visualization"
)

var forbiddenVisualizationConsumerRoots = []string{
	factoryVisualizationRoot + "/internal",
	factoryVisualizationRoot + "/wire",
}

// TestFunctionalProofsImportOnlyPublishedVisualizationSurfaces seals
// FUN-visualization story 005: Visualization-owned functional proofs must not
// reach owner-private internal or wire implementation packages.
func TestFunctionalProofsImportOnlyPublishedVisualizationSurfaces(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listFunctionalVisualizationProofPackages(t) {
		packagePath := packagePath
		t.Run(shortPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertVisualizationImportsPublishedOnly(t, packagePath)
		})
	}
}

// TestProductionPeersReachVisualizationThroughPublishedSurfacesOnly seals
// FUN-visualization story 005: production peers compose Visualization only
// through the published root or terminal protocol adapters, not owner-private
// internal or wire packages.
func TestProductionPeersReachVisualizationThroughPublishedSurfacesOnly(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listVisualizationConsumerPackagesOutsideOwner(t) {
		packagePath := packagePath
		t.Run(shortPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertVisualizationImportsPublishedOnly(t, packagePath)
		})
	}
}

func listFunctionalVisualizationProofPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		modulePrefix+"tests/functional/factory_visualization/...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional visualization proofs: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func listVisualizationConsumerPackagesOutsideOwner(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}}|{{range .Imports}}{{.}};{{end}}",
		modulePrefix+"...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		packagePath := parts[0]
		if packagePath == factoryVisualizationRoot ||
			strings.HasPrefix(packagePath, factoryVisualizationRoot+"/") {
			continue
		}
		imports := ""
		if len(parts) == 2 {
			imports = parts[1]
		}
		if strings.Contains(imports, factoryVisualizationRoot) {
			packages = append(packages, packagePath)
		}
	}
	if len(packages) == 0 {
		t.Fatal("expected at least one external Visualization consumer package")
	}
	return packages
}

func assertVisualizationImportsPublishedOnly(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		if !strings.HasPrefix(importPath, factoryVisualizationRoot) {
			continue
		}
		for _, forbidden := range forbiddenVisualizationConsumerRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import owner-private Visualization package %s",
					packagePath,
					importPath,
				)
			}
		}
		if importPath != factoryVisualizationRoot &&
			!strings.HasPrefix(importPath, factoryVisualizationRoot+"/transports/") {
			t.Fatalf(
				"%s must reach Visualization only through %s or terminal transports; found %s",
				packagePath,
				factoryVisualizationRoot,
				importPath,
			)
		}
	}
}

func listDirectImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func shortPackageName(packagePath string) string {
	prefix := modulePrefix
	if strings.HasPrefix(packagePath, prefix) {
		return strings.TrimPrefix(packagePath, prefix)
	}
	return packagePath
}
