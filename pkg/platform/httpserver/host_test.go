package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
		if address == ":8123" {
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
	if got, want := requests, []string{":8123", ":8124"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("listener addresses = %v, want %v", got, want)
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
