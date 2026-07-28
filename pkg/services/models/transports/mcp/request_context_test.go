package modelmcp_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

func TestBind_PrepareAssetsContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		modelmcp.ToolPrepareAssets,
		prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake models root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"model.request.canceled",
		false,
	)
	if envelope.Message != "models request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_PrepareAssetsContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeModelsRoot{
		prepareModelAssets: func(ctx context.Context, _ models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return models.PrepareModelAssetsResult{}, ctx.Err()
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			modelmcp.ToolPrepareAssets,
			prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake models root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(prepare_assets) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"model.request.canceled",
		false,
	)
	if envelope.Message != "models request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_PrepareAssetsContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeModelsRoot{
		prepareModelAssets: func(ctx context.Context, _ models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
			<-ctx.Done()
			return models.PrepareModelAssetsResult{}, ctx.Err()
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		modelmcp.ToolPrepareAssets,
		prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"model.request.timed_out",
		true,
	)
	if envelope.Message != "models request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}
