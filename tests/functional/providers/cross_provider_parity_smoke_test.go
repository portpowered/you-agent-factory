package providers

import (
	"context"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	"github.com/portpowered/infinite-you/pkg/workers/provider/parityfixtures"
)

// TestCrossProviderParitySmoke_ProviderSuiteEntrypoint is the maintained provider
// suite entrypoint for Batch 09 cross-provider CLI/API and mode parity proofs.
func TestCrossProviderParitySmoke_ProviderSuiteEntrypoint(t *testing.T) {
	if err := parityfixtures.AssertCrossProviderParityCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCrossProviderParitySmoke_AgyRetainsCallerOwnedPTYEdge(t *testing.T) {
	allocator := parityPTYAllocator{}
	providerAdapter, err := agy.NewAdapterWithAllocator(t.TempDir(), allocator, agy.WithExecutable("agy"))
	if err != nil {
		t.Fatalf("construct Agy adapter: %v", err)
	}
	if got := providerAdapter.Identity(); got != adapter.Identity(modelprovider.Agy) {
		t.Fatalf("provider identity = %q, want %q", got, modelprovider.Agy)
	}
	got, err := providerAdapter.PTYAllocator()
	if err != nil {
		t.Fatalf("resolve Agy PTY allocator: %v", err)
	}
	if got != allocator {
		t.Fatal("Agy adapter did not retain the caller-owned PTY allocator")
	}
}

type parityPTYAllocator struct{}

func (parityPTYAllocator) Allocate(context.Context, agypty.ProcessLaunch, agypty.SessionConfig) (agypty.PTYSession, error) {
	return nil, nil
}
