package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommittedOwnerPackageOperatorSettingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		want        PackageMapping
		wantRetain  bool
		retainOwner string
	}{
		{
			path:        "pkg/services/operator_settings",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path:        "pkg/services/operator_settings/wire",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path:        "pkg/services/operator_settings/transports/http",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path: "pkg/services/operator_settings/internal/services/document/wire",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/internal/services/document/wire",
				Disposition: DispositionRetain,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/identityinventory",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/identityinventory",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/identityinventory/input_index",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/identityinventory/input_index",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal/services/document",
			},
		},
		{
			path: "pkg/services/operator_settings/servicewire",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/servicewire",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
		{
			path: "pkg/services/operator_settings/testlink",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/testlink",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
			},
		},
		{
			path: "pkg/services/operator_settings/testdata/fixtures/valid",
			want: PackageMapping{
				PackagePath: "pkg/services/operator_settings/testdata/fixtures/valid",
				Disposition: DispositionMove,
				Destination: "operator_settings/internal",
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

func TestOperatorSettingsTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["operator_settings"]
	for _, child := range spec.unexpected {
		rest := child
		destination, ok := nestedOwnerMoveDestination("operator_settings", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(operator_settings, %q) ok = false", rest)
		}
		if destination == "operator_settings" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, packagePath := range manifest.Inventory {
		if packagePath == "pkg/services/operator_settings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if operatorSettingsCanonicalRetainRest(rest) {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}

func operatorSettingsCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/document"):
		return true
	case strings.HasPrefix(rest, "internal/services/resolution"):
		return true
	default:
		return false
	}
}
