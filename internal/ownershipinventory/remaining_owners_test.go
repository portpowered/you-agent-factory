package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestAllProductOwnerUnexpectedSiblingsMapPackageRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	for _, spec := range ownershipinventory.ProductOwnerTopLevelSpecs() {
		if len(spec.Unexpected) == 0 {
			continue
		}
		for _, child := range spec.Unexpected {
			child := child
			t.Run(spec.Owner+"/"+child, func(t *testing.T) {
				t.Parallel()

				path := "pkg/services/" + spec.Owner + "/" + child
				got, err := ownershipinventory.MapPackage(path)
				if err != nil {
					t.Fatalf("MapPackage(%q) error = %v", path, err)
				}
				if packageRowIsRetainToOwnerRoot(got, spec.Owner) {
					t.Fatalf("unexpected sibling %q maps retain→%s: %#v", path, spec.Owner, got)
				}
				if got.Disposition != ownershipinventory.DispositionMove && got.Disposition != ownershipinventory.DispositionDelete {
					t.Fatalf("unexpected sibling %q disposition = %q, want move or delete", path, got.Disposition)
				}
				if got.Disposition == ownershipinventory.DispositionMove {
					if got.Successor == "" || got.DeletionCondition == "" {
						t.Fatalf("move mapping for %q missing successor/deletionCondition: %#v", path, got)
					}
				}
			})
		}
	}
}
