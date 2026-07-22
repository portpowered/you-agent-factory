package testlanes

import (
	"slices"
	"strings"
	"testing"
)

func TestForImportPathAssignsPrimaryLanes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importPath string
		want       Lane
		wantOK     bool
	}{
		{name: "ordinary package", importPath: ModulePath + "/pkg/services/work", want: LaneUnit, wantOK: true},
		{name: "nested contract", importPath: ModulePath + "/pkg/transports/http/contracttests", want: LaneContract, wantOK: true},
		{name: "provider compatibility", importPath: ModulePath + "/pkg/services/workers/provider/functionaltests", want: LaneContract, wantOK: true},
		{name: "nested integration", importPath: ModulePath + "/pkg/example/integrationtests/case", want: LaneIntegration, wantOK: true},
		{name: "server integration", importPath: ModulePath + "/pkg/transports/http/servertests/factorysessionsse", want: LaneIntegration, wantOK: true},
		{name: "repository guard", importPath: ModulePath + "/pkg/services/factory_runtime/exhaustiontests", want: LaneMaintenance, wantOK: true},
		{name: "command", importPath: ModulePath + "/cmd/factory", want: LaneMaintenance, wantOK: true},
		{name: "internal", importPath: ModulePath + "/internal/contractstaging", want: LaneMaintenance, wantOK: true},
		{name: "root contracts", importPath: ModulePath + "/contracts", want: LaneContract, wantOK: true},
		{name: "CLI production baseline", importPath: ModulePath + "/pkg/transports/cli/baseline", want: LaneFunctional, wantOK: true},
		{name: "CLI generated drift", importPath: ModulePath + "/pkg/transports/cli/climanifestgen", want: LaneContract, wantOK: true},
		{name: "runtime execution fixtures", importPath: ModulePath + "/pkg/services/factory_sessions/execution/fixtures", want: LaneIntegration, wantOK: true},
		{name: "functional", importPath: ModulePath + "/tests/functional/runtime_api", want: LaneFunctional, wantOK: true},
		{name: "functional support", importPath: ModulePath + "/tests/functional/internal/support", want: LaneMaintenance, wantOK: true},
		{name: "stress", importPath: ModulePath + "/tests/stress/runtime", want: LaneStress, wantOK: true},
		{name: "release", importPath: ModulePath + "/tests/release", want: LaneRelease, wantOK: true},
		{name: "ui", importPath: ModulePath + "/ui", want: LaneMaintenance, wantOK: true},
		{name: "docs", importPath: ModulePath + "/docs/reference", wantOK: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ForImportPath(test.importPath)
			if got != test.want || ok != test.wantOK {
				t.Fatalf("ForImportPath(%q) = (%q, %t), want (%q, %t)", test.importPath, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestRunnableFunctionalPackagePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importPath string
		want       bool
	}{
		{name: "provider package", importPath: ModulePath + "/tests/functional/providers/codex", want: true},
		{name: "existing package", importPath: ModulePath + "/tests/functional/workflow", want: true},
		{name: "shared support", importPath: ModulePath + "/tests/functional/internal/support", want: false},
		{name: "backend package", importPath: ModulePath + "/pkg/root", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRunnableFunctionalPackage(test.importPath); got != test.want {
				t.Fatalf("IsRunnableFunctionalPackage(%q) = %t, want %t", test.importPath, got, test.want)
			}
		})
	}
}

func TestValidateProviderFunctionalPackages(t *testing.T) {
	t.Parallel()

	required := RequiredProviderFunctionalPackages()
	if err := ValidateProviderFunctionalPackages(required); err != nil {
		t.Fatalf("ValidateProviderFunctionalPackages() error = %v", err)
	}

	missing := required[2]
	withoutOne := slices.Delete(slices.Clone(required), 2, 3)
	err := ValidateProviderFunctionalPackages(withoutOne)
	if err == nil {
		t.Fatal("ValidateProviderFunctionalPackages() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("ValidateProviderFunctionalPackages() error = %q, want missing package %q", err, missing)
	}
}

func TestRequiredProviderFunctionalPackagesReturnsCopy(t *testing.T) {
	t.Parallel()

	first := RequiredProviderFunctionalPackages()
	first[0] = "changed"
	second := RequiredProviderFunctionalPackages()
	if second[0] == "changed" {
		t.Fatal("RequiredProviderFunctionalPackages() exposed mutable policy state")
	}
}
