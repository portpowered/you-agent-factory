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
			path: "pkg/services/factory_definitions/definition",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/definition",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal",
				DeletionCondition: "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path:        "pkg/services/factory_definitions/internal/services/snapshots_portability/wire",
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
			path:        "pkg/services/factory_definitions/internal",
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
			path:        "pkg/services/factory_definitions/internal/services/catalog/namedfactories",
			wantRetain:  true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/internal/services/validation/impl",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/validation/impl",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/distribution/goal",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/distribution/goal",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/contracts",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/snapshots_portability",
				DeletionCondition: "delete public package after IMP-DEF-snapshots_portability private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/catalog/resource",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/catalog/resource",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/validation",
				DeletionCondition: "delete public package after IMP-DEF-validation private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_definitions",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_definitions/internal/services/invocation_policy",
				DeletionCondition: "delete public package after IMP-DEF-invocation_policy private subservice cutover proof",
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
