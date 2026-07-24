package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageCoversAllThirteenOwners(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	rows, err := buildCommittedOwnerPackages(manifest.Inventory)
	if err != nil {
		t.Fatalf("buildCommittedOwnerPackages() error = %v", err)
	}
	if err := ensureAllProductOwnersPresent(rows); err != nil {
		t.Fatalf("ensureAllProductOwnersPresent() error = %v", err)
	}

	byPath := make(map[string]PackageMapping, len(rows))
	for _, row := range rows {
		byPath[row.PackagePath] = row
	}

	for _, owner := range closedDestinationVocabulary().ProductOwners {
		if owner == "providers" {
			continue
		}
		prefix := "pkg/services/" + owner
		for _, packagePath := range manifest.Inventory {
			if packagePath != prefix && !strings.HasPrefix(packagePath, prefix+"/") {
				continue
			}
			if _, ok := mapProvidersExtraction(packagePath); ok {
				continue
			}
			if _, ok := byPath[packagePath]; !ok {
				t.Fatalf("missing committed-owner mapping for %q", packagePath)
			}
		}
	}

	providersSeen := false
	for _, row := range rows {
		root, _, ok := splitDestination(row.Destination)
		if ok && root == "providers" {
			providersSeen = true
			if row.Disposition != DispositionMove {
				t.Fatalf("providers extraction %q disposition = %q, want move", row.PackagePath, row.Disposition)
			}
		}
	}
	if !providersSeen {
		t.Fatal("expected at least one providers destination from workers extraction paths")
	}
}

func TestMapCommittedOwnerPackageUsesPlanNestedDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want PackageMapping
	}{
		{
			path: "pkg/services/factory_sessions/internal/services/identity",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_sessions/internal/services/identity",
				Disposition: DispositionRetain,
				Destination: "factory_sessions/internal/services/identity",
			},
		},
		{
			path: "pkg/services/factory_sessions/internal/runtimeopening",
			want: PackageMapping{
				PackagePath: "pkg/services/factory_sessions/internal/runtimeopening",
				Disposition: DispositionMove,
				Destination: "factory_sessions/internal/services/runtime_opening",
			},
		},
		{
			path: "pkg/services/work/materialize",
			want: PackageMapping{
				PackagePath: "pkg/services/work/materialize",
				Disposition: DispositionMove,
				Destination: "work/internal/services/content_materialization",
			},
		},
		{
			path: "pkg/services/models/internal/catalog",
			want: PackageMapping{
				PackagePath: "pkg/services/models/internal/catalog",
				Disposition: DispositionMove,
				Destination: "models/internal/services/catalog",
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
			path: "pkg/services/workers/cliprovider",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/cliprovider",
				Disposition: DispositionMove,
				Destination: "providers/internal/services/execution",
			},
		},
		{
			path: "pkg/services/provider_sessions/cursor",
			want: PackageMapping{
				PackagePath: "pkg/services/provider_sessions/cursor",
				Disposition: DispositionMove,
				Destination: "provider_sessions/internal/services/cursor_reader",
			},
		},
		{
			path: "pkg/services/operator_settings",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings",
				Disposition: DispositionRetain,
				Destination: "operator_settings",
			},
		},
		{
			path: "pkg/services/system_initialization",
			want: PackageMapping{
				PackagePath: "pkg/services/system_initialization",
				Disposition: DispositionRetain,
				Destination: "system_initialization",
			},
		},
	}

	for _, tc := range cases {
		got, ok := mapCommittedOwnerPackage(tc.path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
		}
		if got != tc.want {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
		}
	}
}

func TestMapCommittedOwnerPackageSkipsResiduals(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"pkg/services/edges",
		"pkg/platform/clock",
		"pkg/transports/cli",
		"pkg/wire",
		"pkg/root",
		"pkg/initializer",
	} {
		if _, ok := mapCommittedOwnerPackage(path); ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = true, want residual skip", path)
		}
	}
}

func TestValidateManifestRejectsUncommittedNestedSubservice(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version:               1,
		Stage:                 manifestStage,
		DestinationVocabulary: closedDestinationVocabulary(),
		ArchitectureExceptionNotes: map[string]string{
			"edges": edgesArchitectureExceptionNote,
		},
		FutureDebt: []FutureDebt{edgesFutureDebtEntry()},
		Packages: []PackageMapping{{
			PackagePath: "pkg/services/work/mystery",
			Disposition: DispositionMove,
			Destination: "work/internal/services/mystery_cut",
		}},
	}

	err := validateManifest(manifest)
	if err == nil {
		t.Fatal("validateManifest() error = nil, want nested subservice rejection")
	}
	if !strings.Contains(err.Error(), "mystery_cut") {
		t.Fatalf("validateManifest() error = %v, want mystery_cut", err)
	}
}

func TestCommittedManifestMapsProductOwnersWithoutRediscovery(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	wantRows, err := buildCommittedOwnerPackages(manifest.Inventory)
	if err != nil {
		t.Fatalf("buildCommittedOwnerPackages() error = %v", err)
	}
	gotByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		gotByPath[row.PackagePath] = row
	}
	for _, want := range wantRows {
		got, ok := gotByPath[want.PackagePath]
		if !ok {
			t.Fatalf("committed packages missing %q", want.PackagePath)
		}
		if got != want {
			t.Fatalf("committed packages[%q] = %#v, want %#v", want.PackagePath, got, want)
		}
	}
	if err := ensureAllProductOwnersPresent(manifest.Packages); err != nil {
		t.Fatalf("committed packages missing owners: %v", err)
	}
}
