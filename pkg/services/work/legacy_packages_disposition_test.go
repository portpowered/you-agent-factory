package work_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// workProductionPublicSurfacePackages are owner production packages outside
// INV-recorded private destinations. They must depend only on the committed
// public surface (thin root, wire/, and transports/), not unexpected public siblings.
var workProductionPublicSurfacePackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/work",
	"github.com/portpowered/infinite-you/pkg/services/work/wire",
	"github.com/portpowered/infinite-you/pkg/services/work/transports/http",
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli",
	"github.com/portpowered/infinite-you/pkg/services/work/transports/mcp",
}

// workRootBehaviorProofFiles are committed fold-target behavioral proofs that
// exercise wire-constructed list/get/state-access and published Service slices.
var workRootBehaviorProofFiles = []string{
	"wire_behavioral_proof_test.go",
	"service_root_contract_seal_test.go",
	"service_root_contract_test.go",
	"recordings_import_boundary_test.go",
	"recordings_request_boundary_test.go",
}

// workLegitimateTestFixtureDirsRetained lists test-only fixture directories
// that remain at the Work root after DEL-WORK transitional public deletion.
var workLegitimateTestFixtureDirsRetained = []string{
	"pkg/services/work/testdata",
}

var workUnexpectedPublicSiblingImportPrefixes = []string{
	workOwnerPrefix + "/testdata",
}

// TestProductionPackagesDoNotImportUnexpectedPublicSiblings seals
// pss-cln-work-legacy-packages-004: no production importer outside INV-recorded
// private destinations may depend on packet-scoped transitional public siblings.
func TestProductionPackagesDoNotImportUnexpectedPublicSiblings(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range listProductionPackagesSubjectToUnexpectedPublicSiblingImportGuard(t) {
		violations = append(violations, unexpectedPublicSiblingImportViolations(t, packagePath, workUnexpectedPublicSiblingImportPrefixes)...)
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden unexpected public sibling imports:\n%s", strings.Join(violations, "\n"))
	}
}

// TestOwnerProductionPublicSurfaceDoesNotImportUnexpectedPublicSiblings seals
// pss-cln-work-legacy-packages-004: work/wire and other owner public-surface
// production packages must not retain imports of transitional public siblings.
func TestOwnerProductionPublicSurfaceDoesNotImportUnexpectedPublicSiblings(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range workProductionPublicSurfacePackages {
		violations = append(violations, unexpectedPublicSiblingImportViolations(t, packagePath, workUnexpectedPublicSiblingImportPrefixes)...)
	}
	if len(violations) > 0 {
		t.Fatalf("owner public-surface unexpected public sibling imports:\n%s", strings.Join(violations, "\n"))
	}
}

// TestWorkRootBehaviorPreserved seals pss-cln-work-legacy-packages-004: focused
// wire-constructed behavioral proofs and Work root contract characterization
// tests remain committed so legacy-sibling cleanup cannot silently drop list/get
// observability coverage.
func TestWorkRootBehaviorPreserved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	workRoot := filepath.Join(root, "pkg", "services", "work")
	for _, name := range workRootBehaviorProofFiles {
		path := filepath.Join(workRoot, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("root behavior proof file %q missing from work owner: %v", name, err)
		}
	}

	functionalProof := filepath.Join(root, "tests", "functional", "work", "recordings", "recordings_read_test.go")
	if _, err := os.Stat(functionalProof); err != nil {
		t.Fatalf("functional recordings-backed Work root proof missing: %v", err)
	}
}

// TestWorkLegitimateTestFixtureDirsRetained seals DEL-WORK story 002:
// legitimate test-only fixture directories remain after transitional public deletion.
func TestWorkLegitimateTestFixtureDirsRetained(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, dir := range workLegitimateTestFixtureDirsRetained {
		path := filepath.Join(root, filepath.FromSlash(dir))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("legitimate test fixture dir %q must remain: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("test fixture path %q is not a directory", dir)
		}
	}
}

func listProductionPackagesSubjectToUnexpectedPublicSiblingImportGuard(t *testing.T) []string {
	t.Helper()

	packages := append([]string(nil), listPackagesOutsideWorkOwner(t)...)
	packages = append(packages, workProductionPublicSurfacePackages...)
	packages = append(packages, listWorkOwnerProductionPackagesOutsideTransitionalDestinations(t)...)
	return packages
}

func listWorkOwnerProductionPackagesOutsideTransitionalDestinations(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", workOwnerPrefix+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list work owner packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		if isWorkTransitionalPrivateDestination(packagePath) {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func isWorkTransitionalPrivateDestination(packagePath string) bool {
	return strings.HasPrefix(packagePath, workOwnerPrefix+"/internal/services/state_access")
}

func unexpectedPublicSiblingImportViolations(t *testing.T, packagePath string, forbiddenImportPrefixes []string) []string {
	t.Helper()

	var violations []string
	for _, dep := range listTransitiveWorkServiceDeps(t, packagePath) {
		for _, forbidden := range forbiddenImportPrefixes {
			if !matchesForbiddenUnexpectedPublicSiblingImport(dep, forbidden) {
				continue
			}
			violations = append(
				violations,
				fmt.Sprintf(
					"%s must not depend on transitional public sibling %s outside INV private destinations (found %s)",
					packagePath,
					forbidden,
					dep,
				),
			)
		}
	}
	return violations
}

func matchesForbiddenUnexpectedPublicSiblingImport(dep, forbidden string) bool {
	return dep == forbidden || strings.HasPrefix(dep, forbidden+"/")
}

func TestMatchesForbiddenUnexpectedPublicSiblingImport(t *testing.T) {
	t.Parallel()

	forbidden := workOwnerPrefix + "/testdata"
	tests := []struct {
		dep  string
		want bool
	}{
		{forbidden, true},
		{forbidden + "/nested", true},
		{workOwnerPrefix + "/wire", false},
		{workOwnerPrefix, false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.dep, func(t *testing.T) {
			t.Parallel()
			if got := matchesForbiddenUnexpectedPublicSiblingImport(test.dep, forbidden); got != test.want {
				t.Fatalf("matchesForbiddenUnexpectedPublicSiblingImport(%q, %q) = %t, want %t", test.dep, forbidden, got, test.want)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return dir
}
