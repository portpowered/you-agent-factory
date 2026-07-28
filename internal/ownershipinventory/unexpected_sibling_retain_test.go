package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// packageRowIsRetainToOwnerRoot reports the false-retain anti-pattern rejected
// for inventoried unexpected product-owner top-level siblings.
func packageRowIsRetainToOwnerRoot(row ownershipinventory.PackageRow, owner string) bool {
	return row.Disposition == ownershipinventory.DispositionRetain && row.Destination == owner
}

func TestRetainToOwnerRootGuardDetectsDeliberateViolations(t *testing.T) {
	t.Parallel()

	deliberate := ownershipinventory.PackageRow{
		PackagePath: "pkg/services/factory_runtime/engine",
		Disposition: ownershipinventory.DispositionRetain,
		Destination: "factory_runtime",
	}
	if !packageRowIsRetainToOwnerRoot(deliberate, "factory_runtime") {
		t.Fatal("guard failed to detect deliberate retain→factory_runtime")
	}

	valid := ownershipinventory.PackageRow{
		PackagePath: "pkg/services/factory_runtime/engine",
		Disposition: ownershipinventory.DispositionMove,
		Destination: "factory_runtime",
		Successor:   "pkg/services/factory_runtime/internal/services/orchestration",
	}
	if packageRowIsRetainToOwnerRoot(valid, "factory_runtime") {
		t.Fatal("guard falsely flagged valid move mapping as retain→factory_runtime")
	}
}

func TestDeliberateRetainToOwnerMappingRejectedByInventorySweepGuard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		owner string
		child string
	}{
		{owner: "factory_runtime", child: "engine"},
		{owner: "workers", child: "execution"},
		{owner: "operator_settings", child: "identityinventory"},
		{owner: "work", child: "stateaccessrecordings"},
		{owner: "factory_definitions", child: "clonetests"},
		{owner: "recordings", child: "artifacts"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.owner+"/"+tc.child, func(t *testing.T) {
			t.Parallel()

			path := "pkg/services/" + tc.owner + "/" + tc.child
			got, err := ownershipinventory.MapPackage(path)
			if err != nil {
				t.Fatalf("MapPackage(%q) error = %v", path, err)
			}
			if packageRowIsRetainToOwnerRoot(got, tc.owner) {
				t.Fatalf("live mapping for %q is already retain→%s: %#v", path, tc.owner, got)
			}

			deliberate := got
			deliberate.Disposition = ownershipinventory.DispositionRetain
			deliberate.Destination = tc.owner
			deliberate.Successor = ""
			deliberate.DeletionCondition = ""
			if !packageRowIsRetainToOwnerRoot(deliberate, tc.owner) {
				t.Fatalf("guard failed to detect deliberate retain→%s for %q", tc.owner, path)
			}
		})
	}
}

func TestLiveProductionUnexpectedSiblingPackagesRejectRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	for _, packagePath := range packages {
		owner, rest, ok := strings.Cut(strings.TrimPrefix(packagePath, "pkg/services/"), "/")
		if !ok || owner == "" || rest == "" {
			continue
		}
		if !ownershipinventory.IsOwnerUnexpectedTopLevelRest(owner, rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if packageRowIsRetainToOwnerRoot(got, owner) {
			t.Fatalf("unexpected sibling path %q maps retain→%s: %#v", packagePath, owner, got)
		}
		if got.Disposition != ownershipinventory.DispositionMove && got.Disposition != ownershipinventory.DispositionDelete {
			t.Fatalf("unexpected sibling path %q disposition = %q, want move or delete", packagePath, got.Disposition)
		}
	}
}
