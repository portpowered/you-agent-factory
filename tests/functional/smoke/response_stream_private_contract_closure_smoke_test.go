package smoke

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream/removalgate"
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
