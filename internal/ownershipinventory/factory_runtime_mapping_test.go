package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapPackageFactoryRuntimeMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		wantRetain  bool
		retainOwner string
		wantMove    *ownershipinventory.PackageRow
	}{
		{
			path:        "pkg/services/factory_runtime",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/wire",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/transports/http",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/wire",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path: "pkg/services/factory_runtime/service",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_runtime/service",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_runtime",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_runtime/internal",
				DeletionCondition: "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes",
			},
		},
		{
			path: "pkg/services/factory_runtime/build",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_runtime/build",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_runtime",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_runtime/internal/services/instance_host",
				DeletionCondition: "delete public package after IMP-RUN-instance_host private subservice cutover proof",
			},
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/engine",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path: "pkg/services/factory_runtime/checkpointstore",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_runtime/checkpointstore",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_runtime",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_runtime/internal/services/checkpoint_recovery",
				DeletionCondition: "delete public package after IMP-RUN-checkpoint_recovery private subservice cutover proof",
			},
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/context",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/javascript",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/state/validation",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/throttle",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/token",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/token_transformer",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/definitionmapping",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/metrics",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/replayhooks",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path:        "pkg/services/factory_runtime/internal/services/orchestration/runtimecontract",
			wantRetain:  true,
			retainOwner: "factory_runtime",
		},
		{
			path: "pkg/services/factory_runtime/testdata",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/factory_runtime/testdata",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "factory_runtime",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/factory_runtime/internal",
				DeletionCondition: "delete transitional top-level package after CLN-RUN-FOLD-TOPLEVEL cutover proof",
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

func TestFactoryRuntimeInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_runtime/"
	for _, packagePath := range packages {
		if packagePath == "pkg/services/factory_runtime" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if factoryRuntimeCanonicalRetainRest(rest) {
			continue
		}

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "factory_runtime" {
			t.Fatalf("unexpected retain→factory_runtime for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}

func factoryRuntimeCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case rest == "internal" || strings.HasPrefix(rest, "internal/host"):
		return true
	case strings.HasPrefix(rest, "internal/services/orchestration"):
		return true
	case strings.HasPrefix(rest, "internal/services/instance_host"):
		return true
	case strings.HasPrefix(rest, "internal/services/dispatch_planning"):
		return true
	case strings.HasPrefix(rest, "internal/services/checkpoint_recovery"):
		return true
	default:
		return false
	}
}
