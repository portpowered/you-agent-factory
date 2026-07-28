package ownershipinventory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestFrozenInventoryMatchesCommittedPackageMappings(t *testing.T) {
	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	want, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	got, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frozen inventory drifted from committed mappings\nwant bytes=%d got bytes=%d\nregenerate with: go run ./cmd/ownershipinventoryfreeze", len(wantJSON), len(gotJSON))
	}
}

func TestMapPackageMarksProcessEdgesAsArchitectureException(t *testing.T) {
	row, err := ownershipinventory.MapPackage("pkg/services/edges")
	if err != nil {
		t.Fatalf("MapPackage() error = %v", err)
	}
	if row.Destination != ownershipinventory.DestinationEdges {
		t.Fatalf("destination = %q, want %q", row.Destination, ownershipinventory.DestinationEdges)
	}
	if row.DestinationKind != ownershipinventory.DestinationKindArchitectureException {
		t.Fatalf("destinationKind = %q, want architecture_exception", row.DestinationKind)
	}
	if row.Disposition != ownershipinventory.DispositionRetain {
		t.Fatalf("disposition = %q, want retain", row.Disposition)
	}
}

func TestMapPackageMovesProviderPackagesOutOfWorkers(t *testing.T) {
	row, err := ownershipinventory.MapPackage("pkg/services/workers/provider/codex")
	if err != nil {
		t.Fatalf("MapPackage() error = %v", err)
	}
	if row.Destination != "providers" || row.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("row = %#v, want move to providers", row)
	}
	if row.Successor == "" || row.DeletionCondition == "" {
		t.Fatalf("move row missing successor/condition: %#v", row)
	}
}

func TestMapPackageMovesLegacyServiceImplementationPackages(t *testing.T) {
	row, err := ownershipinventory.MapPackage("pkg/services/automations/service")
	if err != nil {
		t.Fatalf("MapPackage() error = %v", err)
	}
	if row.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("disposition = %q, want move", row.Disposition)
	}
	if row.Destination != "automations" {
		t.Fatalf("destination = %q, want automations", row.Destination)
	}
	if row.Successor != "pkg/services/automations/internal" {
		t.Fatalf("successor = %q, want pkg/services/automations/internal", row.Successor)
	}
	if row.DeletionCondition == "" {
		t.Fatal("expected deletion condition on move row")
	}
}

func TestInventoryArtifactExistsAtCanonicalPath(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, ownershipinventory.InventoryRelativePath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ownership inventory artifact missing at %s: %v", ownershipinventory.InventoryRelativePath, err)
	}
}
