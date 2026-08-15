package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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

func TestServeCancellationWaitsForFlushedSSETerminalEvent(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listenerClosed := make(chan struct{})
	listener = &closeNotifyingListener{Listener: listener, closed: listenerClosed}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	clientResult := make(chan struct {
		status int
		body   []byte
		err    error
	}, 1)
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writer := &blockingSSEWriter{
			ResponseWriter: response,
			Context:        request.Context(),
			Started:        writeStarted,
			Release:        releaseWrite,
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(writer, "data: {\"type\":\"RUN_RESPONSE\"}\n\n"); err != nil {
			return
		}
		writer.Flush()
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- Serve(ctx, handler, listener, zap.NewNop())
	}()

	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			clientResult <- struct {
				status int
				body   []byte
				err    error
			}{err: err}
			return
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr == nil {
			readErr = closeErr
		}
		clientResult <- struct {
			status int
			body   []byte
			err    error
		}{status: response.StatusCode, body: body, err: readErr}
	}()

	// The handler has received the terminal SSE event but cannot write it to
	// the connection until the test releases the write. Cancellation occurs
	// while that handoff is blocked, which is the production shutdown race.
	receiveBefore(t, writeStarted)
	cancel()
	receiveBefore(t, listenerClosed)
	close(releaseWrite)

	client := receiveBefore(t, clientResult)
	if client.err != nil {
		t.Fatalf("SSE client read: %v", client.err)
	}
	if client.status != http.StatusOK {
		t.Fatalf("SSE status = %d, want %d", client.status, http.StatusOK)
	}
	if got := string(client.body); got != "data: {\"type\":\"RUN_RESPONSE\"}\n\n" {
		t.Fatalf("SSE body = %q, want terminal RUN_RESPONSE frame followed by EOF", got)
	}
	if err := receiveBefore(t, serveResult); err != nil {
		t.Fatalf("Serve cancellation: %v", err)
	}
}

func TestServeCancellationForceClosesNonReturningHandlerAfterGracePeriod(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listenerClosed := make(chan struct{})
	listener = &closeNotifyingListener{Listener: listener, closed: listenerClosed}

	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseHandler) }) }
	t.Cleanup(release)
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseHandler
	})

	const testGracePeriod = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(ctx, handler, listener, zap.NewNop(), testGracePeriod)
	}()
	clientResult := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr != nil {
			clientResult <- requestErr
			return
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		clientResult <- errors.Join(readErr, closeErr)
	}()

	receiveBefore(t, requestStarted)
	cancel()
	receiveBefore(t, listenerClosed)
	select {
	case err := <-serveResult:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Serve cancellation error = %v, want bounded graceful-shutdown deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve cancellation remained blocked after the graceful-shutdown deadline")
	}

	// The handler deliberately ignores request cancellation. Releasing it only
	// after Serve returns proves forced connection close does not wait forever
	// for a non-cooperative handler.
	release()
	_ = receiveBefore(t, clientResult)
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
	// OnBound fires on the starter's goroutine while this one polls for the
	// binding, so the value has to be published under a lock rather than read
	// straight out of a shared variable.
	var boundMu sync.Mutex
	var bound Binding
	readBound := func() Binding {
		boundMu.Lock()
		defer boundMu.Unlock()
		return bound
	}
	go func() {
		exit <- starter(ctx, StartRequest{
			Handler: http.NotFoundHandler(),
			OnBound: func(value Binding) {
				boundMu.Lock()
				bound = value
				boundMu.Unlock()
			},
		})
	}()
	address := listener.Addr().(*net.TCPAddr)
	for readBound().Port == 0 {
		select {
		case err := <-exit:
			t.Fatalf("starter exited before binding: %v", err)
		case <-time.After(time.Millisecond):
		}
	}
	if observed := readBound(); observed.Host != address.IP.String() || observed.Port != address.Port {
		t.Fatalf("binding = %+v, want %s:%d", observed, address.IP, address.Port)
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

type closeNotifyingListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (listener *closeNotifyingListener) Close() error {
	listener.once.Do(func() { close(listener.closed) })
	return listener.Listener.Close()
}

type blockingSSEWriter struct {
	http.ResponseWriter
	Context context.Context
	Started chan struct{}
	Release <-chan struct{}
	once    sync.Once
}

func (writer *blockingSSEWriter) Write(payload []byte) (int, error) {
	writer.once.Do(func() { close(writer.Started) })
	select {
	case <-writer.Release:
	case <-writer.Context.Done():
		return 0, writer.Context.Err()
	}
	return writer.ResponseWriter.Write(payload)
}

func (writer *blockingSSEWriter) Flush() {
	writer.ResponseWriter.(http.Flusher).Flush()
}

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
