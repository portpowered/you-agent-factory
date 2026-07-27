package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewStarterBindsServesAndJoinsOnCancellation(t *testing.T) {
	bound := make(chan net.Listener, 1)
	starter, err := NewStarter(func(network, _ string) (net.Listener, error) {
		listener, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			bound <- listener
		}
		return listener, err
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	exit := make(chan error, 1)
	go func() {
		exit <- starter(ctx, StartRequest{
			Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, "ready")
			}),
			Port: 8123, Logger: zap.NewNop(),
		})
	}()

	listener := receiveBefore(t, bound)
	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET hosted handler: %v", err)
	}
	payload, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read hosted response: %v", err)
	}
	if got := string(payload); got != "ready" {
		t.Fatalf("response = %q, want ready", got)
	}

	cancel()
	if err := receiveBefore(t, exit); err != nil {
		t.Fatalf("starter cancellation: %v", err)
	}
}

func TestNewStarterReturnsListenerFailure(t *testing.T) {
	wantErr := errors.New("address unavailable")
	starter, err := NewStarter(func(string, string) (net.Listener, error) {
		return nil, wantErr
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	err = starter(t.Context(), StartRequest{Handler: http.NotFoundHandler(), Port: 8123, Logger: zap.NewNop()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("starter error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewStarterRequiresInjectedListenerFactory(t *testing.T) {
	starter, err := NewStarter(nil)
	if err == nil || starter != nil {
		t.Fatalf("NewStarter(nil) = (%v, %v), want nil starter and error", starter, err)
	}
}

func TestNewStarterAutoPortReportsSelectedFallback(t *testing.T) {
	busyErr := errors.New("busy")
	requests := make([]string, 0, 2)
	listener := failingListener{err: errors.New("done")}
	starter, err := NewStarter(func(_ string, address string) (net.Listener, error) {
		requests = append(requests, address)
		if address == "localhost:8123" {
			return nil, busyErr
		}
		return listener, nil
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	var bound Binding
	err = starter(t.Context(), StartRequest{
		Handler: http.NotFoundHandler(), Port: 8123, AutoPort: true,
		Logger: zap.NewNop(), OnBound: func(value Binding) { bound = value },
	})
	if err == nil || bound.Port != 8124 {
		t.Fatalf("starter = (bound=%+v, err=%v), want port 8124 and terminal serve error", bound, err)
	}
	if got, want := requests, []string{"localhost:8123", "localhost:8124"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("listener addresses = %v, want %v", got, want)
	}
}

func TestNewStarterBindsOnlyRequestedLoopbackHost(t *testing.T) {
	t.Parallel()

	listener := failingListener{err: errors.New("done")}
	var address string
	starter, err := NewStarter(func(_ string, candidate string) (net.Listener, error) {
		address = candidate
		return listener, nil
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	var bound Binding
	err = starter(t.Context(), StartRequest{
		Handler: http.NotFoundHandler(), Host: "127.0.0.1", Port: 8123,
		Logger: zap.NewNop(), OnBound: func(value Binding) { bound = value },
	})
	if err == nil {
		t.Fatal("starter error = nil, want terminal serve error")
	}
	if address != "127.0.0.1:8123" {
		t.Fatalf("listener address = %q, want exact IPv4 loopback", address)
	}
	if bound.Host != "127.0.0.1" || bound.Port != 8123 {
		t.Fatalf("binding = %+v, want requested loopback endpoint", bound)
	}
}

func TestNewStarterRejectsNonLoopbackHostBeforeListenerEffect(t *testing.T) {
	t.Parallel()

	var calls int
	starter, err := NewStarter(func(string, string) (net.Listener, error) {
		calls++
		return nil, errors.New("unexpected listener call")
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	err = starter(t.Context(), StartRequest{
		Handler: http.NotFoundHandler(), Host: "0.0.0.0", Port: 8123,
		Logger: zap.NewNop(),
	})
	if !IsBindError(err) {
		t.Fatalf("starter error = %v, want BindError", err)
	}
	if calls != 0 {
		t.Fatalf("listener calls = %d, want none for non-loopback host", calls)
	}
}

func TestNewStarterAutoPortTriesThrough65535WithoutWrapping(t *testing.T) {
	t.Parallel()

	busyErr := errors.New("busy")
	var requests []string
	starter, err := NewStarter(func(_ string, address string) (net.Listener, error) {
		requests = append(requests, address)
		return nil, busyErr
	})
	if err != nil {
		t.Fatalf("NewStarter: %v", err)
	}
	err = starter(t.Context(), StartRequest{
		Handler: http.NotFoundHandler(), Host: "::1", Port: 65534,
		AutoPort: true, Logger: zap.NewNop(),
	})
	if !IsBindError(err) {
		t.Fatalf("starter error = %v, want BindError", err)
	}
	want := []string{"[::1]:65534", "[::1]:65535"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("listener addresses = %v, want %v", requests, want)
	}
}

func TestServeReturnsTerminalListenerFailure(t *testing.T) {
	wantErr := errors.New("accept failed")
	err := Serve(t.Context(), http.NotFoundHandler(), failingListener{err: wantErr}, zap.NewNop())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Serve error = %v, want wrapped %v", err, wantErr)
	}
}

func TestServeValidatesRequiredInputs(t *testing.T) {
	listener := failingListener{err: errors.New("unused")}
	tests := []struct {
		name    string
		ctx     context.Context
		handler http.Handler
		listen  net.Listener
		want    string
	}{
		{name: "context", handler: http.NotFoundHandler(), listen: listener, want: "context is required"},
		{name: "handler", ctx: t.Context(), listen: listener, want: "handler is required"},
		{name: "listener", ctx: t.Context(), handler: http.NotFoundHandler(), want: "listener is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Serve(test.ctx, test.handler, test.listen, nil)
			if err == nil || err.Error() != "serve HTTP: "+test.want {
				t.Fatalf("Serve error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStarterWithListenerReportsBindingServesAndRejectsReuse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	starter := StarterWithListener(listener)
	ctx, cancel := context.WithCancel(t.Context())
	exit := make(chan error, 1)
	var bound Binding
	go func() {
		exit <- starter(ctx, StartRequest{
			Handler: http.NotFoundHandler(),
			OnBound: func(value Binding) { bound = value },
		})
	}()
	address := listener.Addr().(*net.TCPAddr)
	for bound.Port == 0 {
		select {
		case err := <-exit:
			t.Fatalf("starter exited before binding: %v", err)
		case <-time.After(time.Millisecond):
		}
	}
	if bound.Host != address.IP.String() || bound.Port != address.Port {
		t.Fatalf("binding = %+v, want %s:%d", bound, address.IP, address.Port)
	}
	cancel()
	if err := receiveBefore(t, exit); err != nil {
		t.Fatalf("starter cancellation: %v", err)
	}
	if err := starter(t.Context(), StartRequest{Handler: http.NotFoundHandler()}); err == nil ||
		err.Error() != "process-owned API server listener was already used" {
		t.Fatalf("second starter call error = %v, want reuse rejection", err)
	}
}

func TestStarterWithListenerRejectsNilListenerAndBindErrorFormatsCause(t *testing.T) {
	starter := StarterWithListener(nil)
	err := starter(t.Context(), StartRequest{Handler: http.NotFoundHandler()})
	if err == nil || err.Error() != "process-owned API server listener is required" {
		t.Fatalf("nil listener error = %v", err)
	}

	cause := errors.New("ports exhausted")
	bindErr := &BindError{Host: "localhost", PreferredPort: 65535, Cause: cause}
	if !strings.Contains(bindErr.Error(), "localhost") || !errors.Is(bindErr, cause) {
		t.Fatalf("BindError = %q, unwrap=%v", bindErr.Error(), bindErr.Unwrap())
	}
	var nilBindErr *BindError
	if nilBindErr.Error() != "" || nilBindErr.Unwrap() != nil {
		t.Fatalf("nil BindError = (%q, %v), want empty and nil", nilBindErr.Error(), nilBindErr.Unwrap())
	}
}

type failingListener struct {
	err error
}

func (listener failingListener) Accept() (net.Conn, error) { return nil, listener.err }
func (failingListener) Close() error                       { return nil }
func (failingListener) Addr() net.Addr                     { return testAddr("test") }

type testAddr string

func (address testAddr) Network() string { return string(address) }
func (address testAddr) String() string  { return string(address) }

func receiveBefore[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for HTTP host")
		var zero T
		return zero
	}
}
