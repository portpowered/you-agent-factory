package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageFactoryRuntimeMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
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
			path: "pkg/services/factory_runtime/internal/services/orchestration/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/wire",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/instance_host/internal/service",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/instance_host/internal/service",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/instance_host",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/instance_host/build",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/instance_host/build",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/instance_host",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/checkpoint_recovery",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/checkpoint_recovery",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/javascript",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/javascript",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/orchestrators/petri",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/orchestrators/petri",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/engine",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/engine",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/runtime",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/runtime",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/runtimecontract",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/runtimecontract",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/scheduler",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/scheduler",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/state",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/state",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/subsystems",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/subsystems",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/token",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/token",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/context",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/context",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/definitionmapping",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/definitionmapping",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/metrics",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/metrics",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/replayhooks",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/replayhooks",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/state/validation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/state/validation",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/throttle",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/throttle",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/services/orchestration/token_transformer",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/token_transformer",
				Disposition: DispositionRetain,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/rootobservation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/rootobservation",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal",
			},
		},
		{
			path: "pkg/services/factory_runtime/internal/service",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/internal/service",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal",
			},
		},
		{
			path: "pkg/services/factory_runtime/testkit",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/testkit",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal",
			},
		},
	}

	for _, tc := range cases {
		got, ok := mapCommittedOwnerPackage(tc.path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
		}
		if tc.wantRetain {
			if got.Disposition != DispositionRetain || got.Destination != tc.retainOwner {
				t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
			}
			continue
		}
		if got != tc.want {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
		}
	}
}

func TestFactoryRuntimeTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["factory_runtime"]
	for _, child := range spec.unexpected {
		rest := child
		destination, ok := nestedOwnerMoveDestination("factory_runtime", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(factory_runtime, %q) ok = false", rest)
		}
		if destination == "factory_runtime" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}

func TestFactoryRuntimeInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_runtime/"
	for _, packagePath := range manifest.Inventory {
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

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "factory_runtime" {
			t.Fatalf("unexpected retain→factory_runtime for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}
