package root_composition_test

import (
	"context"
	"fmt"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// gatedFailureCommandRunner is local to the incident witness because the
// deadcode gate cannot see reachability through the external CommandRunner
// interface. It holds the provider edge until the public move requests
// cancellation, then returns one controlled late failure.
type gatedFailureCommandRunner struct {
	mu        sync.Mutex
	stdout    []byte
	gate      chan struct{}
	started   chan struct{}
	finished  chan struct{}
	canceled  chan platformprocess.CancellationReason
	callCount int

	startOnce   sync.Once
	releaseOnce sync.Once
	finishOnce  sync.Once
	cancelOnce  sync.Once
}

func newGatedFailureCommandRunner(stdout string) *gatedFailureCommandRunner {
	return &gatedFailureCommandRunner{
		stdout:   []byte(stdout),
		gate:     make(chan struct{}),
		started:  make(chan struct{}),
		finished: make(chan struct{}),
		canceled: make(chan platformprocess.CancellationReason, 1),
	}
}

func (r *gatedFailureCommandRunner) WaitForStart(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("gated failure command runner is required")
	}
	select {
	case <-r.started:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *gatedFailureCommandRunner) Release() {
	if r == nil {
		return
	}
	r.releaseOnce.Do(func() { close(r.gate) })
}

func (r *gatedFailureCommandRunner) WaitForCompletion(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("gated failure command runner is required")
	}
	select {
	case <-r.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *gatedFailureCommandRunner) WaitForCancellation(ctx context.Context) (platformprocess.CancellationReason, error) {
	if r == nil {
		return "", fmt.Errorf("gated failure command runner is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case reason := <-r.canceled:
		return reason, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (r *gatedFailureCommandRunner) CallCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func (r *gatedFailureCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if r == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("gated failure command runner is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	r.callCount++
	r.mu.Unlock()
	r.startOnce.Do(func() { close(r.started) })
	go func() {
		select {
		case <-ctx.Done():
			reason := platformprocess.CancellationReasonFromContext(ctx)
			if reason == "" {
				reason = platformprocess.CancellationReasonCanceled
			}
			r.cancelOnce.Do(func() { r.canceled <- reason })
		case <-r.finished:
		}
	}()
	<-r.gate
	r.finishOnce.Do(func() { close(r.finished) })
	return platformprocess.CommandResult{
		Stdout:   support.CodexSuccessStdout(string(r.stdout)),
		ExitCode: 1,
	}, fmt.Errorf("controlled late provider failure")
}

var _ platformprocess.CommandRunner = (*gatedFailureCommandRunner)(nil)
