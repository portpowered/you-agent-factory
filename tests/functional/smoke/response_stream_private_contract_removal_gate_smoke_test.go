package smoke

import (
	"context"
	"testing"

	removalgate "github.com/portpowered/infinite-you/internal/testutil/responsestreamremovalgate"
)

// TestResponseStreamPrivateContractRemovalGateSmoke is the maintained functional
// entrypoint for Batch 09 Story 001 prerequisite and residual-use evidence.
func TestResponseStreamPrivateContractRemovalGateSmoke(t *testing.T) {
	repoRoot, err := removalgate.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := removalgate.AssertGate(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
}
