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
			path: "pkg/services/factory_definitions/authoredlayout",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/authoredlayout",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/authoring_layout",
			},
		},
		{
			path: "pkg/services/factory_definitions/namedfactories",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/namedfactories",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/catalog",
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
			path: "pkg/services/factory_definitions/loading",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/loading",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/compilation",
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
			path: "pkg/services/factory_definitions/portableconfig",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/portableconfig",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/snapshots_portability",
			},
		},
		{
			path: "pkg/services/factory_definitions/packages/goal",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/packages/goal",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal/services/distribution",
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
			wantRetain: true,
			retainOwner: "factory_definitions",
		},
		{
			path: "pkg/services/factory_definitions/workers/taxonomy",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/workers/taxonomy",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
			},
		},
		{
			path: "pkg/services/factory_definitions/decisionenvelope",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_definitions/decisionenvelope",
				Disposition: DispositionMove,
				Destination: "factory_definitions/internal",
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
	case strings.HasPrefix(rest, "internal/services/validation"):
		return true
	case strings.HasPrefix(rest, "internal/contracts"):
		return true
	case rest == "namevalue" || strings.HasPrefix(rest, "namevalue/"):
		return true
	default:
		return false
	}
}
