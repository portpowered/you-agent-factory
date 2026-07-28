package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestMapPackageOperatorSettingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		wantRetain  bool
		retainOwner string
		wantMove    *ownershipinventory.PackageRow
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
			path:        "pkg/services/operator_settings/internal/services/document/wire",
			wantRetain:  true,
			retainOwner: "operator_settings",
		},
		{
			path: "pkg/services/operator_settings/identityinventory",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/operator_settings/identityinventory",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "operator_settings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/operator_settings/internal/services/document",
				DeletionCondition: "delete public package after IMP-OPS-document private subservice cutover proof",
			},
		},
		{
			path: "pkg/services/operator_settings/servicewire",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/operator_settings/servicewire",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "operator_settings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/operator_settings/internal",
				DeletionCondition: "delete transitional top-level package after CLN-OPS-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/operator_settings/testlink",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/operator_settings/testlink",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "operator_settings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/operator_settings/internal",
				DeletionCondition: "delete transitional top-level package after CLN-OPS-FOLD-TOPLEVEL cutover proof",
			},
		},
		{
			path: "pkg/services/operator_settings/testdata/fixtures/valid",
			wantMove: &ownershipinventory.PackageRow{
				PackagePath:       "pkg/services/operator_settings/testdata/fixtures/valid",
				Disposition:       ownershipinventory.DispositionMove,
				Destination:       "operator_settings",
				DestinationKind:   ownershipinventory.DestinationKindOwner,
				Successor:         "pkg/services/operator_settings/internal",
				DeletionCondition: "delete transitional top-level package after CLN-OPS-FOLD-TOPLEVEL cutover proof",
			},
		},
	}

	for _, tc := range cases {
		got, err := ownershipinventory.MapPackage(tc.path)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", tc.path, err)
		}
		if tc.wantRetain {
			if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != tc.retainOwner {
				t.Fatalf("MapPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
			}
			continue
		}
		if got != *tc.wantMove {
			t.Fatalf("MapPackage(%q) = %#v, want %#v", tc.path, got, *tc.wantMove)
		}
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, packagePath := range packages {
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

		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
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
