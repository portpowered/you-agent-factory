package run

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type hostedInvokerServiceFake struct {
	factorysessions.Service
	result factorysessions.InvocationResult
	err    error
}

func (fake hostedInvokerServiceFake) InvokeFactorySession(
	context.Context,
	string,
	factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	return fake.result, fake.err
}

func TestHostedInvokerAdapterMapsRootResult(t *testing.T) {
	want := factorysessions.InvocationResult{
		RequestID: "request-1", TraceID: "trace-1",
		Status:    factorysessions.InvocationTerminalStatusCompleted,
		SessionID: "session-1", WorkID: "work-1", WorkName: "work",
		WorkState: "done", ErrorCode: "", Message: "completed",
	}
	got, err := (hostedInvokerAdapter{service: hostedInvokerServiceFake{result: want}}).InvokeFactorySession(
		context.Background(), "session-1", factorysessions.InvocationRequest{},
	)
	if err != nil {
		t.Fatalf("InvokeFactorySession() error = %v", err)
	}
	if got.Status != factorydefinitions.InvocationTerminalStatusCompleted || got.RequestID != want.RequestID || got.WorkID != want.WorkID {
		t.Fatalf("mapped result = %#v, want completed result %#v", got, want)
	}
}

func TestHostedInvokerAdapterPropagatesError(t *testing.T) {
	wantErr := errors.New("invoke failed")
	_, err := (hostedInvokerAdapter{service: hostedInvokerServiceFake{err: wantErr}}).InvokeFactorySession(
		context.Background(), "session-1", factorysessions.InvocationRequest{},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("InvokeFactorySession() error = %v, want %v", err, wantErr)
	}
}
