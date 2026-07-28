package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapPackageFactoryDefinitionsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		wantRetain  bool
		retainOwner string
		wantMove    *ownershipinventory.PackageRow
	}{
		{
			path:        "pkg/services/factory_definitions",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path:        "pkg/services/factory_definitions/wire/defaultscaffold",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path:        "pkg/services/factory_definitions/transports/http",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path:        "pkg/services/factory_definitions/internal/services/catalog/wire",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path:        "pkg/services/factory_definitions/internal/services/validation/internal/topology",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/service",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/service",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
			},
		},
		{
			path: "pkg/services/factory_definitions/authoredlayout",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/authoredlayout",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/authoring_layout",
				DeletionCondition: "delete public package after IMP-DEF-authoring_layout private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/namedfactories",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/namedfactories",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/catalog",
				DeletionCondition: "delete public package after IMP-DEF-catalog private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/namedfactories",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/namedfactories",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/catalog",
				DeletionCondition: "delete public package after IMP-DEF-catalog private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/loading",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/loading",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/compilation",
				DeletionCondition: "delete public package after IMP-DEF-compilation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/validation",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/validation",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/portableconfig",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/portableconfig",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/snapshots_portability",
				DeletionCondition: "delete public package after IMP-DEF-snapshots_portability private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/packages/goal",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/packages/goal",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/distribution",
				DeletionCondition: "delete public package after IMP-DEF-distribution private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/contracts",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/namevalue",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/workers/taxonomy",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/workers/taxonomy",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/decisionenvelope",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/decisionenvelope",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/testcomposition",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/testcomposition",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/clonetests",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/clonetests",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof",
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

func TestFactoryDefinitionsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, packagePath := range packages {
		if packagePath == "pkg/services/factory_definitions" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if factoryDefinitionsCanonicalRetainRest(rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "factory_definitions" {
			t.Fatalf("unexpected retain→factory_definitions for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}
