package serverstop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestOperationSendsOneTypedRequestAndConfirmsStoppedListener(t *testing.T) {
	client := &fakeClient{response: acceptedShutdownResponse()}
	var selectedServer string
	op := NewOperationWithDependencies(
		func(server string) (Client, error) {
			selectedServer = server
			return client, nil
		},
		StopObserverFunc(func(context.Context, string, time.Duration) error {
			return nil
		}),
		time.Second,
		time.Second,
	)

	var output strings.Builder
	err := op(context.Background(), Config{
		Server: "http://127.0.0.1:7437", Output: &output,
	})
	if err != nil {
		t.Fatalf("operation error = %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("shutdown request calls = %d, want 1", client.calls)
	}
	if selectedServer != "http://127.0.0.1:7437" {
		t.Fatalf("selected server = %q", selectedServer)
	}
	if output.String() != "Server stopped: http://127.0.0.1:7437\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestOperationRejectsNonLoopbackBeforeRequest(t *testing.T) {
	called := false
	op := NewOperationWithDependencies(
		func(string) (Client, error) {
			called = true
			return &fakeClient{}, nil
		},
		StopObserverFunc(func(context.Context, string, time.Duration) error {
			return errors.New("must not dial")
		}),
		time.Second,
		time.Second,
	)

	err := op(context.Background(), Config{
		Server: "http://203.0.113.10:7437", Output: io.Discard,
	})
	assertServerStopError(t, err, InvalidTargetCode)
	if called {
		t.Fatal("client factory called for non-loopback target")
	}
}

func TestOperationMapsRejectedResponse(t *testing.T) {
	client := &fakeClient{response: &generatedclient.ShutdownServerClientResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusForbidden},
		JSON403: &generatedclient.ShutdownControlRejected{
			Code:    generatedclient.ErrorResponseCode(ControlRejectedCode),
			Family:  generatedclient.ErrorFamilyBadRequest,
			Message: "loopback peer required",
		},
	}}
	op := NewOperationWithDependencies(
		func(string) (Client, error) { return client, nil },
		StopObserverFunc(func(context.Context, string, time.Duration) error {
			return errors.New("must not observe rejected request")
		}),
		time.Second,
		time.Second,
	)

	err := op(context.Background(), Config{
		Server: "http://localhost:7437", Output: io.Discard,
	})
	assertServerStopError(t, err, ControlRejectedCode)
}

func TestOperationMapsUnreachableAndRequestTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "unreachable", err: errors.New("dial refused"), code: UnreachableCode},
		{name: "timeout", err: context.DeadlineExceeded, code: RequestTimeoutCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op := NewOperationWithDependencies(
				func(string) (Client, error) { return &fakeClient{err: test.err}, nil },
				StopObserverFunc(func(context.Context, string, time.Duration) error {
					return errors.New("must not observe failed request")
				}),
				time.Millisecond,
				time.Second,
			)
			err := op(context.Background(), Config{
				Server: "http://localhost:7437", Output: io.Discard,
			})
			assertServerStopError(t, err, test.code)
		})
	}
}

func TestOperationMapsCanceledRequestToTypedFailure(t *testing.T) {
	op := NewOperationWithDependencies(
		func(string) (Client, error) {
			return &fakeClient{err: context.Canceled}, nil
		},
		StopObserverFunc(func(context.Context, string, time.Duration) error {
			return errors.New("must not observe canceled request")
		}),
		time.Second,
		time.Second,
	)

	err := op(context.Background(), Config{
		Server: "http://localhost:7437", Output: io.Discard,
	})
	assertServerStopError(t, err, CanceledCode)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v, want context.Canceled cause", err)
	}
}

func TestOperationReturnsObservationTimeoutWhenListenerStaysOpen(t *testing.T) {
	op := NewOperationWithDependencies(
		func(string) (Client, error) { return &fakeClient{response: acceptedShutdownResponse()}, nil },
		StopObserverFunc(func(context.Context, string, time.Duration) error {
			return context.DeadlineExceeded
		}),
		time.Second,
		10*time.Millisecond,
	)

	err := op(context.Background(), Config{
		Server: "http://localhost:7437", Output: io.Discard,
	})
	assertServerStopError(t, err, ObservationTimeoutCode)
}

func acceptedShutdownResponse() *generatedclient.ShutdownServerClientResponse {
	return &generatedclient.ShutdownServerClientResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202: &generatedclient.ShutdownAcceptedResponse{
			Status:  generatedclient.Accepted,
			Message: "graceful shutdown accepted",
		},
	}
}

type fakeClient struct {
	response *generatedclient.ShutdownServerClientResponse
	err      error
	calls    int
}

func (client *fakeClient) ShutdownServerWithResponse(
	context.Context,
	...generatedclient.RequestEditorFn,
) (*generatedclient.ShutdownServerClientResponse, error) {
	client.calls++
	return client.response, client.err
}

func assertServerStopError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	var stopErr *ServerStopError
	if !errors.As(err, &stopErr) {
		t.Fatalf("error = %T %v, want *ServerStopError", err, err)
	}
	if stopErr.CLIErrorCode() != code {
		t.Fatalf("error code = %q, want %q", stopErr.CLIErrorCode(), code)
	}
}

var _ Client = (*fakeClient)(nil)
