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
			path: "pkg/services/work/internal/services/state_access/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/services/state_access/wire",
				Disposition: DispositionRetain,
				Destination: "work/internal/services/state_access",
			},
		},
		{
			path: "pkg/services/work/internal/service",
			want: PackageMapping{
				PackagePath: "pkg/services/work/internal/service",
				Disposition: DispositionMove,
				Destination: "work/internal",
			},
		},
		{
			path: "pkg/services/work/service",
			want: PackageMapping{
				PackagePath: "pkg/services/work/service",
				Disposition: DispositionMove,
				Destination: "work/internal",
			},
		},
		{
			path: "pkg/services/work/stateaccessrecordings",
			want: PackageMapping{
				PackagePath: "pkg/services/work/stateaccessrecordings",
				Disposition: DispositionMove,
				Destination: "work/internal/services/state_access",
			},
		},
		{
			path: "pkg/services/work/stateaccessrecordings/exercise",
			want: PackageMapping{
				PackagePath: "pkg/services/work/stateaccessrecordings/exercise",
				Disposition: DispositionMove,
				Destination: "work/internal/services/state_access",
			},
		},
		{
			path: "pkg/services/work/testdata/primary_result_regression",
			want: PackageMapping{
				PackagePath: "pkg/services/work/testdata/primary_result_regression",
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
	wantDestination := map[string]string{
		"testdata": "work/internal",
	}
	if len(spec.unexpected) != len(wantDestination) {
		t.Fatalf("unexpected inventory drift: got %v, want keys %v", spec.unexpected, wantDestination)
	}

	for _, child := range spec.unexpected {
		rest := child
		want, ok := wantDestination[child]
		if !ok {
			t.Fatalf("unexpected sibling %q missing from confirmed inventory destinations", child)
		}
		if child == "service" {
			got, ok := mapLegacyServiceImplementationPackage("work", "pkg/services/work/"+child, rest)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage() ok = false for %q", child)
			}
			if got.Disposition != DispositionMove || got.Destination != want {
				t.Fatalf("service move mapping = %#v, want move→%s", got, want)
			}
			continue
		}

		destination, ok := nestedOwnerMoveDestination("work", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(work, %q) ok = false", rest)
		}
		if destination != want {
			t.Fatalf("unexpected top-level child %q destination = %q, want %q", child, destination, want)
		}
	}
}

func TestWorkUnexpectedSiblingMoveDestinationsLocked(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path            string
		wantDestination string
	}{
		{
			path:            "pkg/services/work/service",
			wantDestination: "work/internal",
		},
		{
			path:            "pkg/services/work/stateaccessrecordings",
			wantDestination: "work/internal/services/state_access",
		},
		{
			path:            "pkg/services/work/testdata",
			wantDestination: "work/internal",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			got, ok := mapCommittedOwnerPackage(tc.path)
			if !ok {
				t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
			}
			if mappingIsRetainToOwnerRoot(got, "work") {
				t.Fatalf("mapCommittedOwnerPackage(%q) regressed to retain→work", tc.path)
			}
			if got.Disposition != DispositionMove {
				t.Fatalf("mapCommittedOwnerPackage(%q) disposition = %q, want move", tc.path, got.Disposition)
			}
			if got.Destination != tc.wantDestination {
				t.Fatalf("mapCommittedOwnerPackage(%q) destination = %q, want %q", tc.path, got.Destination, tc.wantDestination)
			}
		})
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
