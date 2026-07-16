package smoke

import (
	"context"
	"testing"

	removalgate "github.com/portpowered/infinite-you/internal/testutil/responsestreamremovalgate"
)

// TestResponseStreamPrivateContractClosureSmoke is the maintained functional
// entrypoint for Batch 09 Story 005 single-vocabulary closure evidence.
func TestResponseStreamPrivateContractClosureSmoke(t *testing.T) {
	repoRoot, err := removalgate.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := removalgate.AssertClosure(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
}
