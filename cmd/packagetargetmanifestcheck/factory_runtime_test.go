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
			path: "pkg/services/factory_runtime/service",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/service",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal",
			},
		},
		{
			path: "pkg/services/factory_runtime/build",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/build",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/instance_host",
			},
		},
		{
			path: "pkg/services/factory_runtime/checkpointstore",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/checkpointstore",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/checkpoint_recovery",
			},
		},
		{
			path: "pkg/services/factory_runtime/checkpointsummary",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/checkpointsummary",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/checkpoint_recovery",
			},
		},
		{
			path: "pkg/services/factory_runtime/javascript",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/javascript",
				Disposition: DispositionMove,
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
			path: "pkg/services/factory_runtime/tooling/javascript/catalog",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/tooling/javascript/catalog",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/engine",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/engine",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/runtime",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/runtime",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/runtime/buffers",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/runtime/buffers",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/runtimecontract",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/runtimecontract",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/scheduler",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/scheduler",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/state",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/state",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/subsystems",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/subsystems",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/token",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/token",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/context",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/context",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/definitionmapping",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/definitionmapping",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/metrics",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/metrics",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/orchestrationowner",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/orchestrationowner",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/orchestratorcontract",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/orchestratorcontract",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/replayhooks",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/replayhooks",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/state/validation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/state/validation",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/throttle",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/throttle",
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			},
		},
		{
			path: "pkg/services/factory_runtime/token_transformer",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_runtime/token_transformer",
				Disposition: DispositionMove,
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
		if child == "service" {
			got, ok := mapLegacyServiceImplementationPackage("factory_runtime", "pkg/services/factory_runtime/"+child, rest)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage() ok = false for %q", child)
			}
			if got.Disposition != DispositionMove || got.Destination != "factory_runtime/internal" {
				t.Fatalf("service move mapping = %#v, want move→factory_runtime/internal", got)
			}
			continue
		}

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
