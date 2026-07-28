package factory_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const (
	factoryRuntimeOwnerPrefix = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryRuntimeWireImport  = factoryRuntimeOwnerPrefix + "/wire"
)

// pss-cln-run-fold-engine-pipeline-007: production peers must construct Runtime
// only through factory_runtime root contracts and factory_runtime/wire, not former
// public engine-pipeline paths or owner-private internal packages.
func TestProductionPackagesOutsideOwnerDoNotImportMovedEnginePipelinePackages(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range listPackagesOutsideFactoryRuntimeOwner(t) {
		for _, dep := range listTransitiveDeps(t, packagePath) {
			if reason, forbidden := forbiddenMovedEnginePipelineImport(dep); forbidden {
				violations = append(
					violations,
					fmt.Sprintf(
						"%s must not depend on %s (%s); construct through factory_runtime/wire or the factory_runtime root only",
						packagePath,
						dep,
						reason,
					),
				)
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden moved engine-pipeline imports:\n%s", strings.Join(violations, "\n"))
	}
}

func listPackagesOutsideFactoryRuntimeOwner(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasPrefix(packagePath, factoryRuntimeOwnerPrefix) {
			continue
		}
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func listTransitiveDeps(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(string(output))
}

func forbiddenMovedEnginePipelineImport(importPath string) (reason string, forbidden bool) {
	if importPath == factoryRuntimeWireImport || strings.HasPrefix(importPath, factoryRuntimeWireImport+"/") {
		return "", false
	}
	if importPath == factoryRuntimeOwnerPrefix {
		return "", false
	}

	ownerPrivatePrefix := factoryRuntimeOwnerPrefix + "/internal"
	if importPath == ownerPrivatePrefix || strings.HasPrefix(importPath, ownerPrivatePrefix+"/") {
		return "owner-private internal package", true
	}

	transitionalServicePrefix := factoryRuntimeOwnerPrefix + "/service"
	if importPath == transitionalServicePrefix || strings.HasPrefix(importPath, transitionalServicePrefix+"/") {
		return "transitional service shim", true
	}

	for _, moved := range foldedEnginePipelineTopLevelChildren() {
		prefix := factoryRuntimeOwnerPrefix + "/" + moved
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return "former public engine-pipeline package", true
		}
	}

	return "", false
}
