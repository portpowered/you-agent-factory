package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapPackageRecordingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		wantRetain  bool
		retainOwner string
		wantMove    *ownershipinventory.PackageRow
	}{
		{
			path:        "pkg/services/recordings",
			wantRetain:  true,
			retainOwner: "recordings",
		},
		{
			path:        "pkg/services/recordings/wire",
			wantRetain:  true,
			retainOwner: "recordings",
		},
		{
			path:        "pkg/services/recordings/transports/http",
			wantRetain:  true,
			retainOwner: "recordings",
		},
		{
			path:        "pkg/services/recordings/internal/services/replay/wire",
			wantRetain:  true,
			retainOwner: "recordings",
		},
		{
			path:        "pkg/services/recordings/internal/canonical",
			wantRetain:  true,
			retainOwner: "recordings",
		},
		{
			path: "pkg/services/recordings/service",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/recordings/service",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "recordings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/recordings/internal",
				DeletionCondition: "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
			},
		},
		{
			path: "pkg/services/recordings/artifacts",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/recordings/artifacts",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "recordings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/recordings/internal/services/artifacts_export",
				DeletionCondition: "delete public package after IMP-REC-artifacts_export private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/recordings/events/kinds",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/recordings/events/kinds",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "recordings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/recordings/internal/services/canonical_ledger",
				DeletionCondition: "delete public package after IMP-REC-canonical_ledger private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/recordings/projections/dashboard",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/recordings/projections/dashboard",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "recordings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/recordings/internal/services/projection_query",
				DeletionCondition: "delete public package after IMP-REC-projection_query private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/recordings/replay/clocktests",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/recordings/replay/clocktests",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "recordings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/recordings/internal/services/replay",
				DeletionCondition: "delete public package after IMP-REC-replay private subservice cutover proof",
			},
		},
	}

	for _, tc := range cases {
		got, err := ownershipinventory.MapPackage(tc.path)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", tc.path, err)
		}
		if tc.wantRetain {
			if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != tc.retainOwner {
				t.Fatalf("MapPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
			}
			continue
		}
		if got != *tc.wantMove {
			t.Fatalf("MapPackage(%q) = %#v, want %#v", tc.path, got, *tc.wantMove)
		}
	}
}

func TestRecordingsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, packagePath := range packages {
		if packagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if ownershipinventory.IsRecordingsCanonicalRetainRest(rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "recordings" {
			t.Fatalf("unexpected retain→recordings for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}
