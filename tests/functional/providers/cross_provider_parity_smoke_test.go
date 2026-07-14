package providers

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers/provider/parityfixtures"
)

// TestCrossProviderParitySmoke_ProviderSuiteEntrypoint is the maintained provider
// suite entrypoint for Batch 09 cross-provider CLI/API and mode parity proofs.
func TestCrossProviderParitySmoke_ProviderSuiteEntrypoint(t *testing.T) {
	if err := parityfixtures.AssertCrossProviderParityCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
}
