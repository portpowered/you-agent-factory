package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageWorkersTransitionalDebtMoves(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want PackageMapping
	}{
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
			path: "pkg/services/workers/diagnostics",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/diagnostics",
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
			path: "pkg/services/workers/execution",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/execution",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/execution/recording",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/execution/recording",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/executor",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/executor",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/executor/agentrun",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/executor/agentrun",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
			},
		},
		{
			path: "pkg/services/workers/invocation",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/invocation",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/workstations",
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
			path: "pkg/services/workers/skippermissions",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/skippermissions",
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
			path: "pkg/services/workers/services/inference",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/services/inference",
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
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
		if got != tc.want {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
		}
	}
}

func TestMapCommittedOwnerPackageWorkersProvidersExtractionMoves(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want PackageMapping
	}{
		{
			path: "pkg/services/workers/agypty",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/agypty",
				Disposition: DispositionMove,
				Destination: "providers/internal/services/execution",
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
			path: "pkg/services/workers/provider",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/provider",
				Disposition: DispositionMove,
				Destination: "providers/internal/services/execution",
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

func TestWorkersTopLevelTransitionalDebtDirectoriesMapToMove(t *testing.T) {
	t.Parallel()

	topLevelDebt := []string{
		"construction",
		"diagnostics",
		"execution",
		"executor",
		"interface",
		"invocation",
		"process",
		"prompting",
		"runner",
		"service",
		"services",
		"skippermissions",
		"worktree",
	}

	for _, dir := range topLevelDebt {
		path := "pkg/services/workers/" + dir
		got, ok := mapCommittedOwnerPackage(path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", path)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("top-level debt %q disposition = %q, want move", path, got.Disposition)
		}
		if got.Destination == "workers" {
			t.Fatalf("top-level debt %q move destination = owner root, want nested plan path", path)
		}
	}
}

func TestMapCommittedOwnerPackageWorkersCanonicalRetains(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want PackageMapping
	}{
		{
			path: "pkg/services/workers",
			want: PackageMapping{
				PackagePath: "pkg/services/workers",
				Disposition: DispositionRetain,
				Destination: "workers",
			},
		},
		{
			path: "pkg/services/workers/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/wire",
				Disposition: DispositionRetain,
				Destination: "workers",
			},
		},
		{
			path: "pkg/services/workers/internal/services/runners",
			want: PackageMapping{
				PackagePath: "pkg/services/workers/internal/services/runners",
				Disposition: DispositionRetain,
				Destination: "workers/internal/services/runners",
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

func TestCommittedManifestWorkersRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/workers/"
	for _, row := range manifest.Packages {
		if row.PackagePath == "pkg/services/workers" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if workersCanonicalRetainRest(rest) {
			continue
		}
		if _, extracted := mapProvidersExtraction(row.PackagePath); extracted {
			if row.Disposition != DispositionMove || !strings.HasPrefix(row.Destination, "providers/") {
				t.Fatalf("providers extraction %q = disposition %q destination %q, want move→providers/*",
					row.PackagePath, row.Disposition, row.Destination)
			}
			continue
		}
		if row.Disposition == DispositionRetain && row.Destination == "workers" {
			t.Fatalf("committed manifest row retain→workers for %q", row.PackagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Destination == "workers" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", row.PackagePath)
		}
	}
}

func TestWorkersInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	for _, packagePath := range manifest.Inventory {
		if packagePath != "pkg/services/workers" && !strings.HasPrefix(packagePath, "pkg/services/workers/") {
			continue
		}
		if _, extracted := mapProvidersExtraction(packagePath); extracted {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}

		switch packagePath {
		case "pkg/services/workers", "pkg/services/workers/wire":
			if got.Disposition != DispositionRetain || got.Destination != "workers" {
				t.Fatalf("canonical workers path %q = %#v, want retain→workers", packagePath, got)
			}
		case "pkg/services/workers/internal/services/runners",
			"pkg/services/workers/internal/services/runtime_assembly",
			"pkg/services/workers/internal/services/workstations":
			if got.Disposition != DispositionRetain || !strings.HasPrefix(got.Destination, "workers/internal/services/") {
				t.Fatalf("committed nested subservice %q = %#v, want retain→workers/internal/services/*", packagePath, got)
			}
		default:
			if strings.HasPrefix(packagePath, "pkg/services/workers/internal/services/") {
				if got.Disposition != DispositionRetain || !strings.HasPrefix(got.Destination, "workers/internal/services/") {
					t.Fatalf("nested workers subservice path %q = %#v, want retain under committed subservice", packagePath, got)
				}
				continue
			}
			if got.Disposition == DispositionRetain && got.Destination == "workers" {
				t.Fatalf("unexpected retain→workers for inventory path %q", packagePath)
			}
			if got.Disposition != DispositionMove {
				t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
			}
		}
	}
}

func workersCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case strings.HasPrefix(rest, "internal/services/runtime_assembly"):
		return true
	case strings.HasPrefix(rest, "internal/services/runners"):
		return true
	case strings.HasPrefix(rest, "internal/services/workstations"):
		return true
	default:
		return false
	}
}
