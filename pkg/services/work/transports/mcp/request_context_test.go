package workmcp_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

func TestBind_GetContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		workmcp.ToolGet,
		json.RawMessage(`{"sessionId":"`+testSessionID+`","workId":"`+testWorkID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"work.request.canceled",
		false,
	)
	if envelope.Message != "work request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_GetContextCanceledDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	fake := fakeWorkRoot{
		getWork: func(ctx context.Context, _ string, _ string) (work.ReadModel, error) {
			enteredOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return work.ReadModel{}, ctx.Err()
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var raw json.RawMessage
	var callErr error
	go func() {
		defer close(done)
		raw, callErr = operation(
			ctx,
			workmcp.ToolGet,
			json.RawMessage(`{"sessionId":"`+testSessionID+`","workId":"`+testWorkID+`"}`),
		)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake Work root did not start before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool(get) hung after cancellation")
	}
	if callErr != nil {
		t.Fatalf("CallTool(get) transport error = %v, want typed tool response", callErr)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"work.request.canceled",
		false,
	)
	if envelope.Message != "work request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_GetContextDeadlineExceededDuringRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeWorkRoot{
		getWork: func(ctx context.Context, _ string, _ string) (work.ReadModel, error) {
			<-ctx.Done()
			return work.ReadModel{}, ctx.Err()
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	raw, err := operation(
		ctx,
		workmcp.ToolGet,
		json.RawMessage(`{"sessionId":"`+testSessionID+`","workId":"`+testWorkID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"work.request.timed_out",
		true,
	)
	if envelope.Message != "work request timed out" {
		t.Fatalf("error.message = %q, want timed out request message; envelope = %#v", envelope.Message, envelope)
	}
}
