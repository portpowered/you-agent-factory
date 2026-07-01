package service

import (
	"context"
	"errors"
	"sync"
)

// Handle hosts one running factory runtime bundle and coordinates its run loop.
type Handle struct {
	Bundle *Bundle

	RunCancel context.CancelFunc
	RunDone   chan struct{}

	SidecarCancel context.CancelFunc
	Sidecars      sync.WaitGroup
	SidecarMu     sync.Mutex

	runErrMu             sync.RWMutex
	runErr               error
	lifecycleMetricsOnce sync.Once
}

// Start launches the hosted runtime run loop for bundle without blocking on readiness.
func Start(ctx context.Context, bundle *Bundle) *Handle {
	if bundle == nil {
		return nil
	}
	runCtx, runCancel := context.WithCancel(ctx)
	handle := &Handle{
		Bundle:    bundle,
		RunCancel: runCancel,
		RunDone:   make(chan struct{}),
	}
	if bundle.Recording != nil {
		bundle.Recording.Start(runCtx)
		if err := bundle.Recording.Flush(); err != nil {
			handle.setRunResult(err)
			return handle
		}
	}
	bundle.EmitRuntimeLifecycleStart()
	go func() {
		err := bundle.Factory.Run(runCtx)
		if err == nil && runCtx.Err() != nil {
			err = context.Canceled
		}
		handle.setRunResult(err)
	}()
	return handle
}

// Completed reports whether the hosted run loop has finished.
func (h *Handle) Completed() bool {
	if h == nil {
		return true
	}
	select {
	case <-h.RunDone:
		return true
	default:
		return false
	}
}

// Result returns the hosted run loop result after completion.
func (h *Handle) Result() error {
	if h == nil {
		return nil
	}
	h.runErrMu.RLock()
	defer h.runErrMu.RUnlock()
	return h.runErr
}

// SetRunResult records the hosted run loop result and unblocks waiters.
// It is exported for tests that simulate run-loop completion without starting Factory.Run.
func (h *Handle) SetRunResult(err error) {
	h.setRunResult(err)
}

func (h *Handle) setRunResult(err error) {
	if h == nil {
		return
	}
	h.runErrMu.Lock()
	h.runErr = err
	h.runErrMu.Unlock()
	close(h.RunDone)
}

// Wait blocks until the hosted run loop completes and returns its result.
func (h *Handle) Wait() error {
	if h == nil {
		return nil
	}
	<-h.RunDone
	return h.Result()
}

// CancelRun requests cancellation of the hosted run loop when it is still active.
func (h *Handle) CancelRun() {
	if h == nil || h.RunCancel == nil || h.Completed() {
		return
	}
	h.RunCancel()
}

// RunDoneCh exposes the run-completion channel for callers that multiplex shutdown.
func (h *Handle) RunDoneCh() <-chan struct{} {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return h.RunDone
}

// LifecycleMetricsOnce exposes the stop-metrics guard used by lifecycle observers.
func (h *Handle) LifecycleMetricsOnce() *sync.Once {
	if h == nil {
		return nil
	}
	return &h.lifecycleMetricsOnce
}

// CanceledRun reports whether the hosted run result is context cancellation.
func CanceledRun(err error) bool {
	return errors.Is(err, context.Canceled)
}
