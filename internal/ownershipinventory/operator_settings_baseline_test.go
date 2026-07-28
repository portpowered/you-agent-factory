package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyOperatorSettingsUnexpectedPublicSiblingRemapsPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsUnexpectedPublicSiblingRemaps() error = %v", err)
	}
}
