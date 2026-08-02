package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

var workersMappingCases = []packageMappingCase{
	{
		path:        "pkg/services/workers",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path:        "pkg/services/workers/wire",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path:        "pkg/services/workers/internal/services/runners",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path:        "pkg/services/workers/internal/services/runners/internal/services/agent",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path:        "pkg/services/workers/internal/services/runtime_assembly/wire",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path:        "pkg/services/workers/internal/services/workstations/internal/service",
		wantRetain:  true,
		retainOwner: "workers",
	},
	{
		path: "pkg/services/workers/service",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/service",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal",
			DeletionCondition: "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
		},
	},
	{
		path: "pkg/services/workers/construction",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/construction",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/runtime_assembly",
			DeletionCondition: "delete public package after IMP-WRK-runtime_assembly private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/diagnostics",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/diagnostics",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal",
			DeletionCondition: "delete transitional top-level package after CLN-WRK-FOLD-TOPLEVEL cutover proof",
		},
	},
	{
		path: "pkg/services/workers/interface",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/interface",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal",
			DeletionCondition: "delete transitional top-level package after CLN-WRK-FOLD-TOPLEVEL cutover proof",
		},
	},
	{
		path: "pkg/services/workers/prompting",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/prompting",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/workstations",
			DeletionCondition: "delete public package after IMP-WRK-workstations private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/execution/recording",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/execution/recording",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/workstations",
			DeletionCondition: "delete public package after IMP-WRK-workstations private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/execution",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/execution",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/workstations",
			DeletionCondition: "delete public package after IMP-WRK-workstations private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/executor/agentrun",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/executor/agentrun",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/workstations",
			DeletionCondition: "delete public package after IMP-WRK-workstations private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/process",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/process",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/runners",
			DeletionCondition: "delete public package after IMP-WRK-runners private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/workers/services/inference",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/workers/services/inference",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "workers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/workers/internal/services/runners",
			DeletionCondition: "delete public package after IMP-WRK-runners private subservice cutover proof",
		},
	},
	{
		path: "pkg/services/providers/internal/services/execution/internal/provider/codex",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/providers/internal/services/execution/internal/provider/codex",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "providers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/providers",
			DeletionCondition: "delete Workers provider packages after Providers root cutover proof (IMP-providers-*)",
		},
	},
	{
		path: "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "providers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/providers",
			DeletionCondition: "delete Workers provider packages after Providers root cutover proof (IMP-providers-*)",
		},
	},
	{
		path: "pkg/services/providers/internal/services/execution/internal/provider_test",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/providers/internal/services/execution/internal/provider_test",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "providers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/providers",
			DeletionCondition: "delete Workers provider packages after Providers root cutover proof (IMP-providers-*)",
		},
	},
	{
		path: "pkg/services/providers/internal/services/execution/internal/provider/registry",
		wantMove: &ownershipinventory.PackageRow{
			PackagePath:       "pkg/services/providers/internal/services/execution/internal/provider/registry",
			Disposition:       ownershipinventory.DispositionMove,
			Destination:       "providers",
			DestinationKind:   ownershipinventory.DestinationKindOwner,
			Successor:         "pkg/services/providers",
			DeletionCondition: "delete Workers provider packages after Providers root cutover proof (IMP-providers-*)",
		},
	},
}

func TestMapPackageWorkersMoveDestinations(t *testing.T) {
	t.Parallel()

	for _, tc := range workersMappingCases {
		assertPackageMapping(t, tc)
	}
}

func TestWorkersInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/workers/"
	for _, packagePath := range packages {
		if packagePath == "pkg/services/workers" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if workersCanonicalRetainRest(rest) {
			continue
		}
		if isWorkersProvidersExtractionRest(rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "workers" {
			t.Fatalf("unexpected retain→workers for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}

func workersCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case strings.HasPrefix(rest, "internal/services/runtime_assembly"):
		return true
	case strings.HasPrefix(rest, "internal/services/workstations"):
		return true
	case strings.HasPrefix(rest, "internal/services/runners"):
		return true
	default:
		return false
	}
}

func isWorkersProvidersExtractionRest(rest string) bool {
	switch {
	case rest == "agypty" || strings.HasPrefix(rest, "agypty/"):
		return true
	case rest == "provider" || strings.HasPrefix(rest, "provider/"):
		return true
	case rest == "provider_test" || strings.HasPrefix(rest, "provider_test/"):
		return true
	default:
		return false
	}
}
