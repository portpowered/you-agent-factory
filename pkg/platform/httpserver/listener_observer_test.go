package httpserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewListenerStopObserverNormalizesNonPositiveInterval(t *testing.T) {
	observer := NewListenerStopObserver(
		func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
		0,
	)

	if observer.interval != DefaultListenerStopObservationInterval {
		t.Fatalf("normalized interval = %s, want %s", observer.interval, DefaultListenerStopObservationInterval)
	}
	if err := observer.Wait(t.Context(), "127.0.0.1:1234", time.Second); err != nil {
		t.Fatalf("Wait after immediate refusal: %v", err)
	}
}

func TestListenerStopObserverValidatesInputs(t *testing.T) {
	observer := NewListenerStopObserver(func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}, time.Millisecond)

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "context",
			call: func() error { return observer.Wait(nil, "127.0.0.1:1234", time.Second) },
			want: "observe listener stop: context is required",
		},
		{
			name: "dialer",
			call: func() error {
				return NewListenerStopObserver(nil, time.Millisecond).Wait(t.Context(), "127.0.0.1:1234", time.Second)
			},
			want: "observe listener stop: dialer is required",
		},
		{
			name: "timeout",
			call: func() error { return observer.Wait(t.Context(), "127.0.0.1:1234", 0) },
			want: "observe listener stop: timeout 0s must be positive",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil || err.Error() != test.want {
				t.Fatalf("Wait error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestListenerStopObserverClosesAcceptedConnectionsBeforeRefusal(t *testing.T) {
	const successfulAttempts = 3
	var (
		mu        sync.Mutex
		addresses []string
		closed    int
	)
	observer := NewListenerStopObserver(func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			t.Errorf("dial network = %q, want tcp", network)
		}
		mu.Lock()
		defer mu.Unlock()
		addresses = append(addresses, address)
		if len(addresses) > successfulAttempts {
			return nil, errors.New("connection refused")
		}
		return &observerTestConn{close: func() { closed++ }}, nil
	}, time.Nanosecond)

	if err := observer.Wait(t.Context(), "127.0.0.1:8123", time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(addresses) != successfulAttempts+1 {
		t.Fatalf("dial attempts = %d, want %d", len(addresses), successfulAttempts+1)
	}
	if closed != successfulAttempts {
		t.Fatalf("closed connections = %d, want %d", closed, successfulAttempts)
	}
	for _, address := range addresses {
		if address != "127.0.0.1:8123" {
			t.Fatalf("dial address = %q, want 127.0.0.1:8123", address)
		}
	}
}

func TestListenerStopObserverReturnsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	dialStarted := make(chan struct{})
	observer := NewListenerStopObserver(func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}, time.Millisecond)
	result := make(chan error, 1)
	go func() { result <- observer.Wait(ctx, "127.0.0.1:8123", time.Second) }()
	receiveBefore(t, dialStarted)
	cancel()
	if err := receiveBefore(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait cancellation = %v, want context.Canceled", err)
	}
}

func TestListenerStopObserverReturnsCancellationWhilePolling(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	connectionReturned := make(chan struct{})
	var returnOnce sync.Once
	observer := NewListenerStopObserver(func(context.Context, string, string) (net.Conn, error) {
		returnOnce.Do(func() { close(connectionReturned) })
		return &observerTestConn{close: func() {}}, nil
	}, time.Hour)
	result := make(chan error, 1)
	go func() { result <- observer.Wait(ctx, "127.0.0.1:8123", time.Second) }()
	receiveBefore(t, connectionReturned)
	cancel()
	if err := receiveBefore(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait polling cancellation = %v, want context.Canceled", err)
	}
}

func TestListenerStopObserverReturnsObservationDeadline(t *testing.T) {
	observer := NewListenerStopObserver(func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, time.Millisecond)

	err := observer.Wait(t.Context(), "127.0.0.1:8123", 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait deadline = %v, want context.DeadlineExceeded", err)
	}
}

type observerTestConn struct {
	close func()
	once  sync.Once
}

func (conn *observerTestConn) Read([]byte) (int, error)  { return 0, errors.New("not readable") }
func (conn *observerTestConn) Write([]byte) (int, error) { return 0, errors.New("not writable") }
func (conn *observerTestConn) Close() error {
	conn.once.Do(conn.close)
	return nil
}
func (*observerTestConn) LocalAddr() net.Addr              { return observerTestAddr("local") }
func (*observerTestConn) RemoteAddr() net.Addr             { return observerTestAddr("remote") }
func (*observerTestConn) SetDeadline(time.Time) error      { return nil }
func (*observerTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*observerTestConn) SetWriteDeadline(time.Time) error { return nil }

type observerTestAddr string

func (addr observerTestAddr) Network() string { return "tcp" }
func (addr observerTestAddr) String() string  { return string(addr) }
