package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

type packageMappingCase struct {
	path        string
	wantRetain  bool
	retainOwner string
	wantMove    *ownershipinventory.PackageRow
}

func assertPackageMapping(t *testing.T, tc packageMappingCase) {
	t.Helper()

	got, err := ownershipinventory.MapPackage(tc.path)
	if err != nil {
		t.Fatalf("MapPackage(%q) error = %v", tc.path, err)
	}
	if tc.wantRetain {
		if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != tc.retainOwner {
			t.Fatalf("MapPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
		}
		return
	}
	if got != *tc.wantMove {
		t.Fatalf("MapPackage(%q) = %#v, want %#v", tc.path, got, *tc.wantMove)
	}
}
