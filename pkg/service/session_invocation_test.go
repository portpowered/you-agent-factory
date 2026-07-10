package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/invocations"
)

type forwardingSessionInvoker struct {
	ctx       context.Context
	sessionID string
	request   factoryapi.InvocationRequest
	result    invocations.FactoryInvocationResult
	err       error
}

func (s *forwardingSessionInvoker) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (invocations.FactoryInvocationResult, error) {
	s.ctx = ctx
	s.sessionID = sessionID
	s.request = request
	return s.result, s.err
}

func TestFactoryService_InvokeFactorySessionForwardsToCanonicalOwner(t *testing.T) {
	requestID := "request-1"
	request := factoryapi.InvocationRequest{RequestId: &requestID, Args: &map[string]any{"input": "hello"}}
	wantResult := invocations.FactoryInvocationResult{
		RequestID: "result-request", TraceID: "trace-1",
		Status: factoryapi.InvocationTerminalStatusCompleted,
	}
	wantErr := errors.New("owner failure")
	invoker := &forwardingSessionInvoker{result: wantResult, err: wantErr}
	svc := &FactoryService{sessionInvoker: invoker}
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("forwarding"), "preserved")

	got, err := svc.InvokeFactorySession(ctx, "session-1", request)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want owner error %v", err, wantErr)
	}
	if !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("result = %#v, want %#v", got, wantResult)
	}
	if invoker.ctx != ctx || invoker.sessionID != "session-1" {
		t.Fatalf("forwarded ctx/session = %#v/%q", invoker.ctx, invoker.sessionID)
	}
	if invoker.request.RequestId == nil || *invoker.request.RequestId != requestID || invoker.request.Args == nil || (*invoker.request.Args)["input"] != "hello" {
		t.Fatalf("forwarded request = %#v", invoker.request)
	}
}
