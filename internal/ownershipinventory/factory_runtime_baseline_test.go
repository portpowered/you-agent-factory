package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestFrozenInventoryFactoryRuntimeRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_runtime/"
	for _, row := range inventory.Packages {
		if row.PackagePath == "pkg/services/factory_runtime" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if factoryRuntimeCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == ownershipinventory.DispositionRetain && row.Destination == "factory_runtime" {
			t.Fatalf("frozen inventory row retain→factory_runtime for %q", row.PackagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("frozen inventory row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Successor == "" || row.DeletionCondition == "" {
			t.Fatalf("frozen inventory row %q missing successor/deletionCondition: %#v", row.PackagePath, row)
		}
	}
}

func TestFactoryRuntimeCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest, err := loadPackageTargetManifest(root)
	if err != nil {
		t.Fatalf("loadPackageTargetManifest() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(inventory.Packages))
	for _, row := range inventory.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/factory_runtime/"
	for _, row := range manifest.Packages {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			continue
		}
		ownershipRow, ok := ownershipByPath[row.PackagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", row.PackagePath)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				row.PackagePath, row.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}

func TestFactoryRuntimeEnginePipelineMoveDestinationsLocked(t *testing.T) {
	t.Parallel()

	orchestrationSuccessor := "pkg/services/factory_runtime/internal/services/orchestration"
	instanceHostSuccessor := "pkg/services/factory_runtime/internal/services/instance_host"
	checkpointRecoverySuccessor := "pkg/services/factory_runtime/internal/services/checkpoint_recovery"

	cases := []struct {
		path            string
		wantSuccessor   string
		wantDestination string
	}{
		{path: "pkg/services/factory_runtime/build", wantSuccessor: instanceHostSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/checkpointstore", wantSuccessor: checkpointRecoverySuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/checkpointsummary", wantSuccessor: checkpointRecoverySuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/context", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/definitionmapping", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/javascript", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/metrics", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/orchestrationowner", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/orchestratorcontract", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/replayhooks", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/runtimecontract", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/throttle", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/token", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/token_transformer", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
		{path: "pkg/services/factory_runtime/tooling/javascript/catalog", wantSuccessor: orchestrationSuccessor, wantDestination: "factory_runtime"},
	}

	for _, tc := range cases {
		got, err := ownershipinventory.MapPackage(tc.path)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", tc.path, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "factory_runtime" {
			t.Fatalf("MapPackage(%q) regressed to retain→factory_runtime", tc.path)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("MapPackage(%q) disposition = %q, want move", tc.path, got.Disposition)
		}
		if got.Destination != tc.wantDestination {
			t.Fatalf("MapPackage(%q) destination = %q, want %q", tc.path, got.Destination, tc.wantDestination)
		}
		if got.Successor != tc.wantSuccessor {
			t.Fatalf("MapPackage(%q) successor = %q, want %q", tc.path, got.Successor, tc.wantSuccessor)
		}
		if got.DeletionCondition == "" {
			t.Fatalf("MapPackage(%q) missing deletionCondition", tc.path)
		}
	}
}
