package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapPackageWorkMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		wantRetain  bool
		retainOwner string
		wantMove    *ownershipinventory.PackageRow
	}{
		{
			path:        "pkg/services/work",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path:        "pkg/services/work/wire",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path:        "pkg/services/work/transports/http",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path:        "pkg/services/work/internal/services/state_access/wire",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path: "pkg/services/work/service",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/work/service",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "work",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/work/internal",
				DeletionCondition: "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
			},
		},
		{
			path: "pkg/services/work/stateaccessrecordings",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/work/stateaccessrecordings",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "work",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/work/internal/services/state_access",
				DeletionCondition: "delete public package after IMP-WORK-state_access private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/work/testdata/primary_result_regression",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/work/testdata/primary_result_regression",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "work",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/work/internal",
				DeletionCondition: "delete transitional top-level package after CLN-WORK-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/work/materialize",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/work/materialize",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "work",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/work/internal/services/content_materialization",
				DeletionCondition: "delete public package after IMP-WORK-content_materialization private subservice cutover proof",
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

func TestWorkInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/work/"
	for _, packagePath := range packages {
		if packagePath == "pkg/services/work" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if workCanonicalRetainRest(rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "work" {
			t.Fatalf("unexpected retain→work for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}

func workCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/admission"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_staging"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_materialization"):
		return true
	case strings.HasPrefix(rest, "internal/services/state_access"):
		return true
	default:
		return false
	}
}
