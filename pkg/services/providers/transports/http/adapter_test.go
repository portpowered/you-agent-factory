package http

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestAdapter_BindsProvidersRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		listProviders: func(
			_ context.Context,
			_ providers.ListProvidersRequest,
		) (providers.ListProvidersResult, error) {
			invoked = true
			return providers.ListProvidersResult{}, providers.ErrUnknownProvider
		},
	}

	adapter := NewAdapter(fake)
	if adapter.Root() != fake {
		t.Fatal("adapter must expose the injected Providers root")
	}

	_, err := adapter.invokeListProviders(context.Background(), providers.ListProvidersRequest{})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Providers root")
	}
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("invokeListProviders error = %v, want ErrUnknownProvider", err)
	}
}

func TestNewAdapter_RejectsNilRoot(t *testing.T) {
	t.Parallel()

	if NewAdapter(nil) != nil {
		t.Fatal("NewAdapter(nil) must return nil")
	}
}

func TestNewAdapter_IsInert(t *testing.T) {
	t.Parallel()

	panicFake := &rootFake{
		listProviders: func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			panic("ListProviders must not run during adapter construction")
		},
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			panic("GetProvider must not run during adapter construction")
		},
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			panic("Execute must not run during adapter construction")
		},
	}

	adapter := NewAdapter(panicFake)
	if adapter == nil || adapter.Root() != panicFake {
		t.Fatal("NewAdapter must retain the injected root without invoking it")
	}
}
