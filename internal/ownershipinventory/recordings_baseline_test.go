package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestRecordingsCommittedManifestAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	manifest, err := loadPackageTargetManifest(root)
	if err != nil {
		t.Fatalf("loadPackageTargetManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, row := range manifest.Packages {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			continue
		}

		got, err := ownershipinventory.MapPackage(row.PackagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", row.PackagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("MapPackage(%q) disposition = %q, want move", row.PackagePath, got.Disposition)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if got.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, MapPackage has %q",
				row.PackagePath, row.Destination, wantSuccessor, got.Successor)
		}
	}
}

func TestRecordingsUnexpectedSiblingMoveDestinationsLocked(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path            string
		wantSuccessor   string
		wantDestination string
	}{
		{
			path:            "pkg/services/recordings/artifacts",
			wantSuccessor:   "pkg/services/recordings/internal/services/artifacts_export",
			wantDestination: "recordings",
		},
		{
			path:            "pkg/services/recordings/events",
			wantSuccessor:   "pkg/services/recordings/internal/services/canonical_ledger",
			wantDestination: "recordings",
		},
		{
			path:            "pkg/services/recordings/events/snapshot",
			wantSuccessor:   "pkg/services/recordings/internal/services/canonical_ledger",
			wantDestination: "recordings",
		},
		{
			path:            "pkg/services/recordings/projections",
			wantSuccessor:   "pkg/services/recordings/internal/services/projection_query",
			wantDestination: "recordings",
		},
		{
			path:            "pkg/services/recordings/replay",
			wantSuccessor:   "pkg/services/recordings/internal/services/replay",
			wantDestination: "recordings",
		},
		{
			path:            "pkg/services/recordings/service",
			wantSuccessor:   "pkg/services/recordings/internal",
			wantDestination: "recordings",
		},
	}

	for _, tc := range cases {
		got, err := ownershipinventory.MapPackage(tc.path)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", tc.path, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "recordings" {
			t.Fatalf("MapPackage(%q) regressed to retain→recordings", tc.path)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("MapPackage(%q) disposition = %q, want move", tc.path, got.Disposition)
		}
		if got.Destination != tc.wantDestination {
			t.Fatalf("MapPackage(%q) destination = %q, want %q", tc.path, got.Destination, tc.wantDestination)
		}
		if got.Successor != tc.wantSuccessor {
			t.Fatalf("MapPackage(%q) successor = %q, want %q", tc.path, got.Successor, tc.wantSuccessor)
		}
		if got.DeletionCondition == "" {
			t.Fatalf("MapPackage(%q) missing deletionCondition", tc.path)
		}
	}
}
