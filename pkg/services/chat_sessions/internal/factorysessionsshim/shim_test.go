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

	startCalls  []startCall
	startResult factorysessions.AsyncStartResult
	startErr    error

	invokeCalls  []invokeCall
	invokeResult factorysessions.InvocationResult
	invokeErr    error

	cancelCalls  []cancelCall
	cancelResult factorysessions.LifecycleControlResult
	cancelErr    error

	closeCalls []closeCall
	closeErr   error
}

type startCall struct {
	ctx     context.Context
	request factorysessions.StartRequest
}

type invokeCall struct {
	ctx       context.Context
	sessionID string
	request   factorysessions.InvocationRequest
}

type cancelCall struct {
	ctx       context.Context
	sessionID string
	request   factorysessions.ControlRequest
}

type closeCall struct {
	ctx       context.Context
	sessionID string
}

func (fake *recordingFactorySessionsService) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	fake.startCalls = append(fake.startCalls, startCall{ctx: ctx, request: request})
	return fake.startResult, fake.startErr
}

func (fake *recordingFactorySessionsService) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	fake.invokeCalls = append(fake.invokeCalls, invokeCall{ctx: ctx, sessionID: sessionID, request: request})
	return fake.invokeResult, fake.invokeErr
}

func (fake *recordingFactorySessionsService) Cancel(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	fake.cancelCalls = append(fake.cancelCalls, cancelCall{ctx: ctx, sessionID: sessionID, request: request})
	return fake.cancelResult, fake.cancelErr
}

func (fake *recordingFactorySessionsService) CloseFactorySession(ctx context.Context, sessionID string) error {
	fake.closeCalls = append(fake.closeCalls, closeCall{ctx: ctx, sessionID: sessionID})
	return fake.closeErr
}

// assertNoOtherCalls fails the test unless every recorded call slice other
// than the named target operation is empty, proving the shim delegated to
// exactly one Factory Sessions capability.
func assertNoOtherCalls(t *testing.T, fake *recordingFactorySessionsService, target string) {
	t.Helper()
	counts := map[string]int{
		"start":  len(fake.startCalls),
		"invoke": len(fake.invokeCalls),
		"cancel": len(fake.cancelCalls),
		"close":  len(fake.closeCalls),
	}
	for name, count := range counts {
		if name == target {
			continue
		}
		if count != 0 {
			t.Fatalf("unexpected delegation to %s: call count = %d, want 0", name, count)
		}
	}
}

// ctxProbeKey distinguishes each test's context.Context value so the
// delegation assertions below prove the shim forwards the exact context it
// was given, rather than substituting a fresh one.
type ctxProbeKey struct{}

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
			ctx := context.WithValue(context.Background(), ctxProbeKey{}, "start-"+tt.name)
			request := factorysessions.StartRequest{RequestID: "req-1", Args: map[string]any{"k": "v"}}

			got, err := shim.StartFactoryTarget(ctx, request)

			if len(fake.startCalls) != 1 {
				t.Fatalf("StartAsync call count = %d, want 1", len(fake.startCalls))
			}
			if fake.startCalls[0].ctx != ctx {
				t.Fatalf("StartAsync ctx = %#v, want the exact ctx passed in", fake.startCalls[0].ctx)
			}
			if !reflect.DeepEqual(fake.startCalls[0].request, request) {
				t.Fatalf("StartAsync request = %#v, want %#v", fake.startCalls[0].request, request)
			}
			if err != tt.err {
				t.Fatalf("StartFactoryTarget() error = %v, want the exact same error value %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("StartFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
			assertNoOtherCalls(t, fake, "start")
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
			ctx := context.WithValue(context.Background(), ctxProbeKey{}, "invoke-"+tt.name)
			request := factorysessions.InvocationRequest{RequestID: strPtr("req-1")}

			got, err := shim.InvokeFactoryTarget(ctx, "session-1", request)

			if len(fake.invokeCalls) != 1 {
				t.Fatalf("InvokeFactorySession call count = %d, want 1", len(fake.invokeCalls))
			}
			if fake.invokeCalls[0].ctx != ctx {
				t.Fatalf("InvokeFactorySession ctx = %#v, want the exact ctx passed in", fake.invokeCalls[0].ctx)
			}
			if fake.invokeCalls[0].sessionID != "session-1" || !reflect.DeepEqual(fake.invokeCalls[0].request, request) {
				t.Fatalf("InvokeFactorySession call = %#v, want session-1 / %#v", fake.invokeCalls[0], request)
			}
			if err != tt.err {
				t.Fatalf("InvokeFactoryTarget() error = %v, want the exact same error value %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("InvokeFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
			assertNoOtherCalls(t, fake, "invoke")
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
			ctx := context.WithValue(context.Background(), ctxProbeKey{}, "cancel-"+tt.name)
			request := factorysessions.ControlRequest{RequestID: "req-1", Reason: "user requested"}

			got, err := shim.CancelFactoryTarget(ctx, "session-1", request)

			if len(fake.cancelCalls) != 1 {
				t.Fatalf("Cancel call count = %d, want 1", len(fake.cancelCalls))
			}
			if fake.cancelCalls[0].ctx != ctx {
				t.Fatalf("Cancel ctx = %#v, want the exact ctx passed in", fake.cancelCalls[0].ctx)
			}
			if fake.cancelCalls[0].sessionID != "session-1" || !reflect.DeepEqual(fake.cancelCalls[0].request, request) {
				t.Fatalf("Cancel call = %#v, want session-1 / %#v", fake.cancelCalls[0], request)
			}
			if err != tt.err {
				t.Fatalf("CancelFactoryTarget() error = %v, want the exact same error value %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("CancelFactoryTarget() result = %#v, want %#v", got, tt.result)
			}
			assertNoOtherCalls(t, fake, "cancel")
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
			ctx := context.WithValue(context.Background(), ctxProbeKey{}, "close-"+tt.name)

			err := shim.CloseFactoryTarget(ctx, "session-1")

			if len(fake.closeCalls) != 1 {
				t.Fatalf("CloseFactorySession call count = %d, want 1", len(fake.closeCalls))
			}
			if fake.closeCalls[0].ctx != ctx {
				t.Fatalf("CloseFactorySession ctx = %#v, want the exact ctx passed in", fake.closeCalls[0].ctx)
			}
			if fake.closeCalls[0].sessionID != "session-1" {
				t.Fatalf("CloseFactorySession sessionID = %q, want session-1", fake.closeCalls[0].sessionID)
			}
			if err != tt.err {
				t.Fatalf("CloseFactoryTarget() error = %v, want the exact same error value %v", err, tt.err)
			}
			assertNoOtherCalls(t, fake, "close")
		})
	}
}

func strPtr(s string) *string { return &s }
