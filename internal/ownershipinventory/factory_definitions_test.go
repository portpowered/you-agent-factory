package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

var factoryDefinitionsMappingCases = []packageMappingCase{
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
		path:        "pkg/services/factory_definitions/internal/services/validation/impl",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/distribution/goal",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/contracts",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/catalog/resource",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution",
		wantRetain:  true,
		retainOwner: "factory_definitions",
	},
	{
		path:        "pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability",
		wantRetain:  true,
		retainOwner: "factory_definitions",
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

func TestMapPackageFactoryDefinitionsMoveDestinations(t *testing.T) {
	t.Parallel()

	for _, tc := range factoryDefinitionsMappingCases {
		assertPackageMapping(t, tc)
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
