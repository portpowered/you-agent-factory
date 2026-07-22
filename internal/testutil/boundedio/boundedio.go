// Package boundedio owns asynchronous and timed I/O mechanics for repository
// integration tests. It is an external test host, not an application or
// product-service composition root.
package boundedio

import (
	"context"
	"errors"
	"time"
)

var ErrTimeout = errors.New("bounded I/O wait elapsed")

// Pending is one in-flight blocking I/O operation. The result channel is
// buffered so an operation may finish after its caller's bounded wait.
type Pending[T any] struct {
	result <-chan T
}

// Start begins one blocking I/O operation.
func Start[T any](operation func() T) *Pending[T] {
	result := make(chan T, 1)
	go func() { result <- operation() }()
	return &Pending[T]{result: result}
}

// Await waits until the operation completes, the caller cancels, or the bound
// elapses. A timeout does not discard the in-flight operation; callers may
// await the same Pending value again.
func (pending *Pending[T]) Await(ctx context.Context, timeout time.Duration) (T, error) {
	var zero T
	if pending == nil {
		return zero, errors.New("bounded I/O operation is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case result := <-pending.result:
		return result, nil
	case <-wait.Done():
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, ErrTimeout
	}
}

// CancelScope returns a caller-owned cancellation scope for a long-lived I/O
// request.
func CancelScope(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// TimeoutScope returns a caller-owned bounded scope for one I/O request.
func TimeoutScope(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// Wait blocks for the supplied test-only observation interval.
func Wait(timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	<-timer.C
}

// Deadline returns the host-clock deadline for a bounded test I/O operation.
// Transport harnesses use this owner operation rather than selecting a clock.
func Deadline(timeout time.Duration) time.Time {
	return time.Now().Add(timeout)
}

// Remaining returns the host-clock interval until a bounded I/O deadline.
func Remaining(deadline time.Time) time.Duration {
	return time.Until(deadline)
}
