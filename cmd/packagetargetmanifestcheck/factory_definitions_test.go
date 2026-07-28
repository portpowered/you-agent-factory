package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageFactoryDefinitionsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
	}{
		{
			path: "pkg/services/factory_definitions",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/definition",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/definition",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
			},
		},
		{
			path:        "pkg/services/factory_definitions/internal/services/snapshots_portability/wire",
			wantRetain:  true,
			retainOwner: "factory_definitions/internal/services/snapshots_portability",
		},
		{
			path: "pkg/services/factory_definitions/wire/defaultscaffold",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/transports/http",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/internal/services/catalog/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/internal/services/catalog/wire",
				Disposition: DispositionRetain,
				Destination: "factory_definitions/internal/services/catalog",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/validation/internal/topology",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/internal/services/validation/internal/topology",
				Disposition: DispositionRetain,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/service",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/service",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/services/catalog/namedfactories",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/internal/services/catalog/namedfactories",
				Disposition: DispositionRetain,
				Destination: "factory_definitions/internal/services/catalog",
			},
		},
		{
			path: "pkg/services/factory_definitions/validation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/validation",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/packages/goal",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/packages/goal",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/contracts",
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path:        "pkg/services/factory_definitions/internal",
			wantRetain:  true,
			retainOwner: "factory_definitions/internal",
		},
		{
			path: "pkg/services/factory_definitions/namevalue",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/namevalue",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/workers",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/workers",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/workers/taxonomy",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/workers/taxonomy",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/replayconfig",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/replayconfig",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/snapshots_portability",
			},
		},
		{
			path: "pkg/services/factory_definitions/resource",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/resource",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/validation",
			},
		},
		{
			path: "pkg/services/factory_definitions/decisionenvelope",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/decisionenvelope",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/invocationinterpolation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/invocationinterpolation",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/invocationoutput",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/invocationoutput",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/invocationworktype",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/invocationworktype",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/quorumpolicy",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/quorumpolicy",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/workpropagation",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/workpropagation",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/workstationexecution",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/workstationexecution",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/ttsobservability",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/ttsobservability",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/invocation_policy",
			},
		},
		{
			path: "pkg/services/factory_definitions/internal/testcomposition",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/internal/testcomposition",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
			},
		},
		{
			path: "pkg/services/factory_definitions/clonetests",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/clonetests",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
			},
		},
		{
			path: "pkg/services/factory_definitions/systeminitializationtests",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/systeminitializationtests",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
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

func TestFactoryDefinitionsTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["factory_definitions"]
	for _, child := range spec.unexpected {
		rest := child
		if child == "service" {
			got, ok := mapLegacyServiceImplementationPackage("factory_definitions", "pkg/services/factory_definitions/"+child, rest)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage() ok = false for %q", child)
			}
			if got.Disposition != DispositionMove || got.Destination != "factory_definitions/internal" {
				t.Fatalf("service move mapping = %#v, want move→factory_definitions/internal", got)
			}
			continue
		}

		destination, ok := nestedOwnerMoveDestination("factory_definitions", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(factory_definitions, %q) ok = false", rest)
		}
		if destination == "factory_definitions" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}

func TestCommittedManifestFactoryDefinitionsRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, row := range manifest.Packages {
		if row.PackagePath == "pkg/services/factory_definitions" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if factoryDefinitionsCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == DispositionRetain && row.Destination == "factory_definitions" {
			t.Fatalf("committed manifest row retain→factory_definitions for %q", row.PackagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Destination == "factory_definitions" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", row.PackagePath)
		}
	}
}

func TestFactoryDefinitionsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/factory_definitions/"
	for _, packagePath := range manifest.Inventory {
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

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "factory_definitions" {
			t.Fatalf("unexpected retain→factory_definitions for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}

func TestCommittedManifestResidualInvocationPolicyPackagesLocked(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	byPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		byPath[row.PackagePath] = row
	}

	for _, rest := range []string{
		"decisionenvelope",
		"invocationinterpolation",
		"invocationoutput",
		"invocationworktype",
		"quorumpolicy",
		"workpropagation",
		"workstationexecution",
		"ttsobservability",
		"packages/goal",
	} {
		packagePath := "pkg/services/factory_definitions/" + rest
		got, ok := byPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing row for %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("committed manifest %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Destination != "factory_definitions/internal/services/invocation_policy" {
			t.Fatalf("committed manifest %q destination = %q, want factory_definitions/internal/services/invocation_policy",
				packagePath, got.Destination)
		}
		if got.Disposition == DispositionRetain && got.Destination == "factory_definitions" {
			t.Fatalf("committed manifest %q must not retain→factory_definitions", packagePath)
		}
	}
}

func factoryDefinitionsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "internal":
		return true
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/catalog"):
		return true
	case strings.HasPrefix(rest, "internal/services/authoring_layout"):
		return true
	case strings.HasPrefix(rest, "internal/services/validation"):
		return true
	case strings.HasPrefix(rest, "internal/services/compilation"):
		return true
	case strings.HasPrefix(rest, "internal/services/distribution"):
		return true
	case strings.HasPrefix(rest, "internal/services/invocation_policy"):
		return true
	case strings.HasPrefix(rest, "internal/services/snapshots_portability"):
		return true
	case strings.HasPrefix(rest, "internal/lifecycle"):
		return true
	case strings.HasPrefix(rest, "internal/contracts"):
		return true
	case strings.HasPrefix(rest, "internal/lifecycle"):
		return true
	default:
		return false
	}
}
