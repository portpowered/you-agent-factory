package lifecycle

import (
	"context"
	"sync"
)

// ReadinessGate joins one published startup signal with the operation that
// requires it. The process lifecycle owns this synchronization.
type ReadinessGate struct {
	ready chan struct{}
	once  sync.Once
}

func NewReadinessGate() *ReadinessGate {
	return &ReadinessGate{ready: make(chan struct{})}
}

func (gate *ReadinessGate) Publish(observe func()) {
	gate.once.Do(func() {
		if observe != nil {
			observe()
		}
		close(gate.ready)
	})
}

func (gate *ReadinessGate) Wait(ctx context.Context) error {
	select {
	case <-gate.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
