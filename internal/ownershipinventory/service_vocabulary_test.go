package ownershipinventory_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// TestDiscoverProductOwnersReadsTheServicesDirectory proves the service-name
// vocabulary is derived from the pkg/services directory at check time rather than
// from a closed Go list.
//
// The fixture root holds two service names that appear nowhere in this
// repository's Go source, so they can only be reported by reading the tree. This
// is the retirement of the registration tax: adding a service is a directory
// creation, not an edit to a checker literal.
func TestDiscoverProductOwnersReadsTheServicesDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, dir := range []string{
		"pkg/services/throwaway_probe_service",
		"pkg/services/second_throwaway_service/internal",
		"pkg/services/edges",
		"pkg/services/testdata",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	owners, err := ownershipinventory.DiscoverProductOwners(root)
	if err != nil {
		t.Fatalf("DiscoverProductOwners() error = %v", err)
	}
	want := []string{"second_throwaway_service", "throwaway_probe_service"}
	if !slices.Equal(owners, want) {
		t.Fatalf("DiscoverProductOwners() = %v, want %v (edges is the architecture exception, testdata is ignored)", owners, want)
	}

	vocabulary, err := ownershipinventory.DiscoverDestinationVocabulary(root)
	if err != nil {
		t.Fatalf("DiscoverDestinationVocabulary() error = %v", err)
	}
	if !vocabulary.IsOwner("throwaway_probe_service") {
		t.Fatalf("vocabulary.IsOwner(throwaway_probe_service) = false; owners=%v", vocabulary.Owners)
	}
	if kind, ok := vocabulary.KindOf("edges"); !ok || kind != ownershipinventory.DestinationKindArchitectureException {
		t.Fatalf("KindOf(edges) = %q, %v; want architecture_exception, true", kind, ok)
	}
}

// TestUnfinishedMoveDestinationsFollowTheLiveServicesDirectory proves the derived
// roster is what the ledger validator enforces: a row may target a service that
// exists only as a directory, and may not target a service with no directory.
func TestUnfinishedMoveDestinationsFollowTheLiveServicesDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash("pkg/services/throwaway_probe_service")), 0o755); err != nil {
		t.Fatalf("mkdir throwaway service: %v", err)
	}
	vocabulary, err := ownershipinventory.DiscoverDestinationVocabulary(root)
	if err != nil {
		t.Fatalf("DiscoverDestinationVocabulary() error = %v", err)
	}

	ledger := func(destination, successor string) ownershipinventory.UnfinishedMoves {
		return ownershipinventory.UnfinishedMoves{
			Version:  1,
			Stage:    ownershipinventory.UnfinishedMovesStage,
			SortKey:  ownershipinventory.SortKeyDescription,
			EndState: "shrinks to zero, then the file is deleted",
			Moves: []ownershipinventory.UnfinishedMoveRow{{
				PackagePath:       "pkg/services/throwaway_probe_service/legacyfacade",
				Destination:       destination,
				Successor:         successor,
				DeletionCondition: "delete after cutover proof",
			}},
		}
	}

	accepted := ownershipinventory.ValidateUnfinishedMoves(
		ledger("throwaway_probe_service", "pkg/services/throwaway_probe_service/internal"),
		vocabulary,
	)
	if len(accepted) != 0 {
		t.Fatalf("ValidateUnfinishedMoves() rejected a destination whose service directory exists: %v", accepted)
	}

	rejected := ownershipinventory.ValidateUnfinishedMoves(
		ledger("ghost_service", "pkg/services/ghost_service/internal"),
		vocabulary,
	)
	if len(rejected) == 0 {
		t.Fatal("ValidateUnfinishedMoves() accepted a destination naming a service with no directory")
	}
	if !strings.Contains(strings.Join(rejected, "\n"), "ghost_service") {
		t.Fatalf("ValidateUnfinishedMoves() problems = %v, want the unknown service named", rejected)
	}
}

// TestOwnerForPackageDerivesOwnerForAnUnknownServiceName proves path→owner
// resolution consults the path rather than a roster, which is what lets a package
// inside a brand-new service map without any checker edit.
func TestOwnerForPackageDerivesOwnerForAnUnknownServiceName(t *testing.T) {
	t.Parallel()

	const added = "pkg/services/throwaway_probe_service/internal/thing"
	owner, ok := ownershipinventory.OwnerForPackage(added)
	if !ok || owner != "throwaway_probe_service" {
		t.Fatalf("OwnerForPackage(%q) = %q, %v; want throwaway_probe_service, true", added, owner, ok)
	}

	row, mapped := ownershipinventory.MapPackage(added)
	if mapped != nil {
		t.Fatalf("MapPackage(%q) error = %v; a package in a new service must resolve without a checker edit", added, mapped)
	}
	if row.Disposition != ownershipinventory.DispositionRetain || row.Destination != "throwaway_probe_service" {
		t.Fatalf("MapPackage(%q) = %#v, want retain to throwaway_probe_service", added, row)
	}
	if row.DestinationKind != ownershipinventory.DestinationKindOwner {
		t.Fatalf("MapPackage(%q) destinationKind = %q, want owner", added, row.DestinationKind)
	}
}

// TestLoadDerivesDestinationVocabularyFromTheTree ties the derivation to the
// checker's real entry path: the vocabulary the gate validates against comes off
// the live tree, not out of the committed artifact.
func TestLoadDerivesDestinationVocabularyFromTheTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	derived, err := ownershipinventory.DiscoverProductOwners(root)
	if err != nil {
		t.Fatalf("DiscoverProductOwners() error = %v", err)
	}
	if !slices.Equal(inventory.Destinations.Owners, derived) {
		t.Fatalf("Load().Destinations.Owners = %v, want the derived roster %v", inventory.Destinations.Owners, derived)
	}

	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ownershipinventory.InventoryRelativePath)))
	if err != nil {
		t.Fatalf("read committed inventory: %v", err)
	}
	if strings.Contains(string(payload), "\"destinations\"") {
		t.Fatal("the committed inventory still carries a destinations block; the vocabulary is derived and must not be mirrored into the artifact")
	}
}
