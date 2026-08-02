package factorysessionsshim

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// recordingFactorySessionsService is a recording fake over
// factorysessions.Service. It embeds the interface unimplemented so any call
// to a capability beyond Start/Invoke/Cancel/Close panics on a nil method
// value, proving the shim never reaches beyond its four delegated operations.
type recordingFactorySessionsService struct {
	factorysessions.Service

	startCalls  []factorysessions.StartRequest
	startResult factorysessions.AsyncStartResult
	startErr    error

	invokeCalls  []invokeCall
	invokeResult factorysessions.InvocationResult
	invokeErr    error

	cancelCalls  []cancelCall
	cancelResult factorysessions.LifecycleControlResult
	cancelErr    error

	closeCalls []string
	closeErr   error
}

type invokeCall struct {
	sessionID string
	request   factorysessions.InvocationRequest
}

type cancelCall struct {
	sessionID string
	request   factorysessions.ControlRequest
}

func (fake *recordingFactorySessionsService) StartAsync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	fake.startCalls = append(fake.startCalls, request)
	return fake.startResult, fake.startErr
}

func (fake *recordingFactorySessionsService) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	fake.invokeCalls = append(fake.invokeCalls, invokeCall{sessionID: sessionID, request: request})
	return fake.invokeResult, fake.invokeErr
}

func (fake *recordingFactorySessionsService) Cancel(
	_ context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	fake.cancelCalls = append(fake.cancelCalls, cancelCall{sessionID: sessionID, request: request})
	return fake.cancelResult, fake.cancelErr
}

func (fake *recordingFactorySessionsService) CloseFactorySession(_ context.Context, sessionID string) error {
	fake.closeCalls = append(fake.closeCalls, sessionID)
	return fake.closeErr
}

func TestShimStart(t *testing.T) {
	tests := []struct {
		name   string
		result factorysessions.AsyncStartResult
		err    error
	}{
		{
			name:   "success",
			result: factorysessions.AsyncStartResult{SessionID: "session-1", Status: "RUNNING"},
		},
		{
			name: "provider error",
			err:  errors.New("start failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &recordingFactorySessionsService{startResult: tt.result, startErr: tt.err}
			shim := New(fake)
			request := factorysessions.StartRequest{RequestID: "req-1", Args: map[string]any{"k": "v"}}

			got, err := shim.StartFactoryTarget(context.Background(), request)

			if len(fake.startCalls) != 1 {
				t.Fatalf("StartAsync call count = %d, want 1", len(fake.startCalls))
			}
			if !reflect.DeepEqual(fake.startCalls[0], request) {
				t.Fatalf("StartAsync request = %#v, want %#v", fake.startCalls[0], request)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("StartFactoryTarget() error = %v, want %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("StartFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
		})
	}
}

func TestShimInvoke(t *testing.T) {
	tests := []struct {
		name   string
		result factorysessions.InvocationResult
		err    error
	}{
		{
			name:   "success",
			result: factorysessions.InvocationResult{RequestID: "req-1", SessionID: "session-1", Status: factorysessions.InvocationTerminalStatusCompleted},
		},
		{
			name: "provider error",
			err:  errors.New("invoke failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &recordingFactorySessionsService{invokeResult: tt.result, invokeErr: tt.err}
			shim := New(fake)
			request := factorysessions.InvocationRequest{RequestID: strPtr("req-1")}

			got, err := shim.InvokeFactoryTarget(context.Background(), "session-1", request)

			if len(fake.invokeCalls) != 1 {
				t.Fatalf("InvokeFactorySession call count = %d, want 1", len(fake.invokeCalls))
			}
			if fake.invokeCalls[0].sessionID != "session-1" || !reflect.DeepEqual(fake.invokeCalls[0].request, request) {
				t.Fatalf("InvokeFactorySession call = %#v, want session-1 / %#v", fake.invokeCalls[0], request)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("InvokeFactoryTarget() error = %v, want %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("InvokeFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
			if len(fake.startCalls) != 0 {
				t.Fatalf("StartAsync call count = %d, want 0", len(fake.startCalls))
			}
		})
	}
}

func TestShimCancel(t *testing.T) {
	tests := []struct {
		name   string
		result factorysessions.LifecycleControlResult
		err    error
	}{
		{
			name:   "success",
			result: factorysessions.LifecycleControlResult{SessionID: "session-1", Outcome: "CANCELED"},
		},
		{
			name: "provider error",
			err:  errors.New("cancel failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &recordingFactorySessionsService{cancelResult: tt.result, cancelErr: tt.err}
			shim := New(fake)
			request := factorysessions.ControlRequest{RequestID: "req-1", Reason: "user requested"}

			got, err := shim.CancelFactoryTarget(context.Background(), "session-1", request)

			if len(fake.cancelCalls) != 1 {
				t.Fatalf("Cancel call count = %d, want 1", len(fake.cancelCalls))
			}
			if fake.cancelCalls[0].sessionID != "session-1" || !reflect.DeepEqual(fake.cancelCalls[0].request, request) {
				t.Fatalf("Cancel call = %#v, want session-1 / %#v", fake.cancelCalls[0], request)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("CancelFactoryTarget() error = %v, want %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("CancelFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
			if len(fake.startCalls) != 0 || len(fake.invokeCalls) != 0 || len(fake.closeCalls) != 0 {
				t.Fatalf(
					"unexpected delegation: start=%d invoke=%d close=%d, want all 0",
					len(fake.startCalls), len(fake.invokeCalls), len(fake.closeCalls),
				)
			}
		})
	}
}

func TestShimClose(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "provider error", err: errors.New("close failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &recordingFactorySessionsService{closeErr: tt.err}
			shim := New(fake)

			err := shim.CloseFactoryTarget(context.Background(), "session-1")

			if len(fake.closeCalls) != 1 || fake.closeCalls[0] != "session-1" {
				t.Fatalf("CloseFactorySession calls = %#v, want [session-1]", fake.closeCalls)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("CloseFactoryTarget() error = %v, want %v", err, tt.err)
			}
			if len(fake.startCalls) != 0 || len(fake.invokeCalls) != 0 || len(fake.cancelCalls) != 0 {
				t.Fatalf(
					"unexpected delegation: start=%d invoke=%d cancel=%d, want all 0",
					len(fake.startCalls), len(fake.invokeCalls), len(fake.cancelCalls),
				)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
