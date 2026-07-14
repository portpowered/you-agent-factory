package smoke

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream/removalgate"
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
