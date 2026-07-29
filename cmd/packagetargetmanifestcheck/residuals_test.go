package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapResidualPackageRetainsApprovedFamilies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want PackageMapping
	}{
		{
			path: "pkg/initializer",
			want: PackageMapping{
				PackagePath: "pkg/initializer",
				Disposition: DispositionRetain,
				Destination: "initializer",
			},
		},
		{
			path: "pkg/initializer/lifecycle",
			want: PackageMapping{
				PackagePath: "pkg/initializer/lifecycle",
				Disposition: DispositionRetain,
				Destination: "initializer",
			},
		},
		{
			path: "pkg/platform/clock",
			want: PackageMapping{
				PackagePath: "pkg/platform/clock",
				Disposition: DispositionRetain,
				Destination: "platform",
			},
		},
		{
			path: "pkg/platform/internal/runtimeartifact",
			want: PackageMapping{
				PackagePath: "pkg/platform/internal/runtimeartifact",
				Disposition: DispositionRetain,
				Destination: "platform",
			},
		},
		{
			path: "pkg/root",
			want: PackageMapping{
				PackagePath: "pkg/root",
				Disposition: DispositionRetain,
				Destination: "root",
			},
		},
		{
			path: "pkg/transports/cli",
			want: PackageMapping{
				PackagePath: "pkg/transports/cli",
				Disposition: DispositionRetain,
				Destination: "transports",
			},
		},
		{
			path: "pkg/transports/mapping/factorydefinition/retiredboundary",
			want: PackageMapping{
				PackagePath: "pkg/transports/mapping/factorydefinition/retiredboundary",
				Disposition: DispositionRetain,
				Destination: "transports",
			},
		},
		{
			path: "pkg/wire",
			want: PackageMapping{
				PackagePath: "pkg/wire",
				Disposition: DispositionRetain,
				Destination: "wire",
			},
		},
	}

	for _, tc := range cases {
		got, ok := mapResidualPackage(tc.path)
		if !ok {
			t.Fatalf("mapResidualPackage(%q) ok = false", tc.path)
		}
		if got != tc.want {
			t.Fatalf("mapResidualPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
		}
	}
}

func TestMapResidualPackageSkipsOwnerAndEdgesPackages(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"pkg/services/work",
		"pkg/services/providers/internal/services/execution/internal/provider",
		"pkg/services/edges",
	} {
		if _, ok := mapResidualPackage(path); ok {
			t.Fatalf("mapResidualPackage(%q) ok = true, want owner/edges skip", path)
		}
	}
}

func TestMapResidualPackageQueuesUnknownResidualsForDeletion(t *testing.T) {
	t.Parallel()

	got, ok := mapResidualPackage("pkg/legacy/orphan")
	if !ok {
		t.Fatal("mapResidualPackage(pkg/legacy/orphan) ok = false")
	}
	if got.Disposition != DispositionDelete {
		t.Fatalf("disposition = %q, want delete", got.Disposition)
	}
	if strings.TrimSpace(got.DeletionSuccessor) == "" {
		t.Fatal("deletionSuccessor is required for unknown residual")
	}
	if strings.TrimSpace(got.DeletionCondition) == "" {
		t.Fatal("deletionCondition is required for unknown residual")
	}
	closed := closedDestinationSet()
	if err := validateDestination(got.Destination, closed); err != nil {
		t.Fatalf("destination %q invalid: %v", got.Destination, err)
	}
	root, _, ok := splitDestination(got.Destination)
	if !ok {
		t.Fatalf("destination %q did not split", got.Destination)
	}
	if _, isOwner := productOwnerSet()[root]; !isOwner {
		if _, isFamily := nonServiceFamilySet()[root]; !isFamily {
			t.Fatalf("unknown residual destination %q invents a top-level owner outside closed vocabulary", got.Destination)
		}
	}
}

func TestBuildResidualPackagesCoversInventoryFamilies(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	rows, err := buildResidualPackages(manifest.Inventory)
	if err != nil {
		t.Fatalf("buildResidualPackages() error = %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("buildResidualPackages() returned no rows")
	}

	byPath := make(map[string]PackageMapping, len(rows))
	for _, row := range rows {
		byPath[row.PackagePath] = row
	}

	wantFamilies := map[string]struct{}{
		"initializer": {},
		"platform":    {},
		"root":        {},
		"transports":  {},
		"wire":        {},
	}
	seenFamilies := map[string]struct{}{}
	for _, packagePath := range manifest.Inventory {
		if _, ok := mapCommittedOwnerPackage(packagePath); ok {
			if _, present := byPath[packagePath]; present {
				t.Fatalf("residual rows unexpectedly include owner package %q", packagePath)
			}
			continue
		}
		if _, ok := mapEdgesExceptionPackage(packagePath); ok {
			if _, present := byPath[packagePath]; present {
				t.Fatalf("residual rows unexpectedly include edges package %q", packagePath)
			}
			continue
		}
		got, present := byPath[packagePath]
		if !present {
			t.Fatalf("residual packages missing inventory path %q", packagePath)
		}
		want, ok := mapResidualPackage(packagePath)
		if !ok {
			t.Fatalf("mapResidualPackage(%q) ok = false for inventory residual", packagePath)
		}
		if got != want {
			t.Fatalf("residual packages[%q] = %#v, want %#v", packagePath, got, want)
		}
		root, _, ok := splitDestination(got.Destination)
		if !ok {
			t.Fatalf("residual destination %q for %q did not split", got.Destination, packagePath)
		}
		if _, isFamily := wantFamilies[root]; isFamily && got.Disposition == DispositionRetain {
			seenFamilies[root] = struct{}{}
		}
	}

	for family := range wantFamilies {
		if _, ok := seenFamilies[family]; !ok {
			t.Fatalf("residual rows missing retain destination for approved family %q", family)
		}
	}
}

func TestCommittedManifestMapsResidualsWithoutReopeningOwners(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	wantRows, err := buildResidualPackages(manifest.Inventory)
	if err != nil {
		t.Fatalf("buildResidualPackages() error = %v", err)
	}
	gotByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		gotByPath[row.PackagePath] = row
	}
	for _, want := range wantRows {
		got, ok := gotByPath[want.PackagePath]
		if !ok {
			t.Fatalf("committed packages missing residual %q", want.PackagePath)
		}
		if got != want {
			t.Fatalf("committed packages[%q] = %#v, want residual %#v", want.PackagePath, got, want)
		}
	}

	vocabOwners := append([]string{}, manifest.DestinationVocabulary.ProductOwners...)
	wantOwners := closedDestinationVocabulary().ProductOwners
	if strings.Join(vocabOwners, ",") != strings.Join(wantOwners, ",") {
		t.Fatalf("destination vocabulary productOwners changed; residual mapping must not reopen the 13-owner tree")
	}
}

