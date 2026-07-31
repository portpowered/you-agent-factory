package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageWorkMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
	}{
		{
			path:        "pkg/services/work",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path:        "pkg/services/work/wire",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path:        "pkg/services/work/transports/http",
			wantRetain:  true,
			retainOwner: "work",
		},
		{
			path: "pkg/services/work/internal",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal",
				Disposition: DispositionRetain,
				Destination: "work",
			},
		},
		{
			path: "pkg/services/work/internal/services/state_access/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/services/state_access/wire",
				Disposition: DispositionRetain,
				Destination: "work/internal/services/state_access",
			},
		},
		{
			path: "pkg/services/work/internal/lineagegraph",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/lineagegraph",
				Disposition: DispositionMove,
				Destination: "work/internal",
			},
		},
		{
			path: "pkg/services/work/internal/proposalmaterialization",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/proposalmaterialization",
				Disposition: DispositionMove,
				Destination: "work/internal",
			},
		},
		{
			path: "pkg/services/work/internal/stateaccessquery",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/stateaccessquery",
				Disposition: DispositionMove,
				Destination: "work/internal",
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

func TestWorkTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["work"]
	if len(spec.unexpected) != 0 {
		t.Fatalf("unexpected inventory drift: got %v, want no unexpected siblings", spec.unexpected)
	}
}

func TestWorkUnexpectedSiblingMoveDestinationsLocked(t *testing.T) {
	t.Parallel()

	if len(productOwnerTopLevelSpecs["work"].unexpected) != 0 {
		t.Fatal("unexpected Work siblings remain inventoried")
	}
}

func TestWorkInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/work/"
	for _, packagePath := range manifest.Inventory {
		if packagePath == "pkg/services/work" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if workCanonicalRetainRest(rest) {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "work" {
			t.Fatalf("unexpected retain→work for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}

func workCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "internal":
		return true
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/admission"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_staging"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_materialization"):
		return true
	case strings.HasPrefix(rest, "internal/services/state_access"):
		return true
	default:
		return false
	}
}
