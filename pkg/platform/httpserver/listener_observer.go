package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultListenerStopObservationInterval is the cadence used while checking
// whether a listener still accepts connections after shutdown is acknowledged.
const DefaultListenerStopObservationInterval = 50 * time.Millisecond

// ListenerDialContext is the replaceable network effect used by
// ListenerStopObserver.
type ListenerDialContext func(context.Context, string, string) (net.Conn, error)

// ListenerStopObserver confirms that a TCP listener no longer accepts new
// connections. It owns the bounded observation lifecycle so protocol
// transports only coordinate the operation and map its result.
type ListenerStopObserver struct {
	dialContext ListenerDialContext
	interval    time.Duration
}

// NewListenerStopObserver constructs a listener observer from its network
// effect and polling cadence.
func NewListenerStopObserver(dialContext ListenerDialContext, interval time.Duration) ListenerStopObserver {
	if interval <= 0 {
		interval = DefaultListenerStopObservationInterval
	}
	return ListenerStopObserver{dialContext: dialContext, interval: interval}
}

// Wait returns nil when the listener refuses a connection before the timeout.
// A still-open listener returns context.DeadlineExceeded or context.Canceled.
func (observer ListenerStopObserver) Wait(ctx context.Context, address string, timeout time.Duration) error {
	if ctx == nil {
		return errors.New("observe listener stop: context is required")
	}
	if observer.dialContext == nil {
		return errors.New("observe listener stop: dialer is required")
	}
	if timeout <= 0 {
		return fmt.Errorf("observe listener stop: timeout %s must be positive", timeout)
	}

	observeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(observer.interval)
	defer ticker.Stop()

	for {
		connection, err := observer.dialContext(observeCtx, "tcp", address)
		if err != nil {
			if observeCtx.Err() != nil {
				return observeCtx.Err()
			}
			return nil
		}
		if connection != nil {
			_ = connection.Close()
		}

		select {
		case <-observeCtx.Done():
			return observeCtx.Err()
		case <-ticker.C:
		}
	}
}
