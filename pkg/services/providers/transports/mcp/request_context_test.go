package providersmcp_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
)

func TestBind_ListProvidersContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.Bind(providersmcp.RootDependencies{
		Providers: fakeProvidersRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		providersmcp.ToolListProviders,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"provider.request.canceled",
		false,
	)
	if envelope.Message != "providers request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ListProvidersContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeProvidersRoot{
		listProviders: func(ctx context.Context, _ providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return providers.ListProvidersResult{}, ctx.Err()
		},
	}
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			providersmcp.ToolListProviders,
			json.RawMessage(`{}`),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake Providers root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(list_providers) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"provider.request.canceled",
		false,
	)
	if envelope.Message != "providers request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ListProvidersContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeProvidersRoot{
		listProviders: func(ctx context.Context, _ providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			<-ctx.Done()
			return providers.ListProvidersResult{}, ctx.Err()
		},
	}
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		providersmcp.ToolListProviders,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"provider.request.timed_out",
		true,
	)
	if envelope.Message != "providers request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}
