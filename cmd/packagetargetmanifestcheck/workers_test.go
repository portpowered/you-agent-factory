package main

import (
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageWorkersMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
	}{
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
			path: "pkg/services/workers/internal/services/runners/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/internal/services/runners/wire",
				Disposition: DispositionRetain,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/service",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/service",
				Disposition: DispositionMove,
				Destination: "workers/internal",
			},
		},
		{
			path: "pkg/services/workers/construction",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/construction",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runtime_assembly",
			},
		},
		{
			path: "pkg/services/workers/prompting",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/prompting",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/worktree",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/worktree",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/skippermissions",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/skippermissions",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/diagnostics",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/diagnostics",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/execution",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/execution",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/execution/recording",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/execution/recording",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/executor",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/executor",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/executor/agentrun",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/executor/agentrun",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/invocation",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/invocation",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/process",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/process",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/runner",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/runner",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/interface",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/interface",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/services/inference",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/services/inference",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/services/testing",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/services/testing",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			},
		},
		{
			path: "pkg/services/workers/provider/registry",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/provider/registry",
				Disposition: DispositionMove,
				Destination: "providers/internal/services/catalog",
			},
		},
		{
			path: "pkg/services/workers/provider_test",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/provider_test",
				Disposition: DispositionMove,
				Destination: "providers/internal/services/execution",
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

func TestWorkersTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["workers"]
	for _, child := range spec.unexpected {
		switch child {
		case "agypty", "cliprovider", "provider", "provider_test":
			path := "pkg/services/workers/" + child
			got, ok := mapProvidersExtraction(path)
			if !ok {
				t.Fatalf("mapProvidersExtraction(%q) ok = false", path)
			}
			if got.Disposition != DispositionMove || strings.HasPrefix(got.Destination, "workers") {
				t.Fatalf("providers extraction %q = %#v, want move outside workers", path, got)
			}
			continue
		case "service":
			got, ok := mapLegacyServiceImplementationPackage("workers", "pkg/services/workers/"+child, child)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage() ok = false for %q", child)
			}
			if got.Disposition != DispositionMove || got.Destination != "workers/internal" {
				t.Fatalf("service move mapping = %#v, want move→workers/internal", got)
			}
			continue
		}

		destination, ok := nestedOwnerMoveDestination("workers", child)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(workers, %q) ok = false", child)
		}
		if destination == "workers" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}
