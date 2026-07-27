package wire

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewServiceBuildsUsableRoot(t *testing.T) {
	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := root.ListProviders(
		context.Background(),
		providers.ListProvidersRequest{},
	)
	if err != nil || len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = (%#v, %v), want catalog entries", result, err)
	}
}

func TestNewRootRejectsMissingCatalog(t *testing.T) {
	root, err := newRoot(nil)
	if err == nil || root != nil {
		t.Fatalf("newRoot(nil) = (%v, %v), want construction error", root, err)
	}
}
