package factoryvisualization_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestBind_ObserveContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{invoked: &invoked}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		mcpfactoryvisualization.ToolObserve,
		json.RawMessage(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake visualization root was invoked for pre-canceled context")
	}
	assertRequestContextErrorEnvelope(
		t,
		string(raw),
		"factory_visualization.request.canceled",
		false,
		"factory visualization request was canceled",
		"CANCELED",
	)
}

func TestBind_ObserveContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeVisualizationRoot{
		observe: func(ctx context.Context, _ factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return factoryvisualization.ObserveResult{}, ctx.Err()
		},
	}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			mcpfactoryvisualization.ToolObserve,
			json.RawMessage(`{"mode":"RETAINED_THEN_LIVE"}`),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake visualization root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(observe) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", callErr)
	}
	assertRequestContextErrorEnvelope(
		t,
		string(raw),
		"factory_visualization.request.canceled",
		false,
		"factory visualization request was canceled",
		"CANCELED",
	)
}

func TestBind_ObserveContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		observe: func(ctx context.Context, _ factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			<-ctx.Done()
			return factoryvisualization.ObserveResult{}, ctx.Err()
		},
	}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		mcpfactoryvisualization.ToolObserve,
		json.RawMessage(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want typed tool response", err)
	}
	assertRequestContextErrorEnvelope(
		t,
		string(raw),
		"factory_visualization.request.timed_out",
		true,
		"factory visualization request timed out",
		"TIMED_OUT",
	)
}

func assertRequestContextErrorEnvelope(
	t *testing.T,
	response string,
	wantCode string,
	wantRetryable bool,
	wantMessage string,
	wantReason string,
) {
	t.Helper()

	var envelope struct {
		Error *mcpfactoryvisualization.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v\nresponse=%s", err, response)
	}
	if envelope.Error == nil {
		t.Fatalf("response = %s, want error envelope", response)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, wantCode)
	}
	if envelope.Error.Retryable != wantRetryable {
		t.Fatalf("error retryable = %v, want %v", envelope.Error.Retryable, wantRetryable)
	}
	if envelope.Error.Message != wantMessage {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Error.Message, wantMessage, envelope.Error)
	}
	if envelope.Error.Details == nil {
		t.Fatal("error details are required for request-context envelopes")
	}
	reason, ok := envelope.Error.Details["reason"].(string)
	if !ok || reason != wantReason {
		t.Fatalf("error details reason = %v, want %q", envelope.Error.Details["reason"], wantReason)
	}
}
