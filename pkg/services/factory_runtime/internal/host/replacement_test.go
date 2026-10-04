package host_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"go.uber.org/zap"
)

func TestStartReplacement_StopsReplacementWhenReadinessFails(t *testing.T) {
	t.Parallel()
	factoryStub := &blockingLifecycleFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStatePaused),
	})

	scheduler := &readinessTimers{created: make(chan *readinessTimer, 8)}
	recording := &terminalRecording{}
	result := make(chan error, 1)
	serviceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, err := factoryhost.StartReplacement(context.Background(), serviceCtx,
			&factoryhost.Bundle{Factory: factoryStub, Logger: zap.NewNop(), Recording: recording},
			clockwork.NewFakeClock(), scheduler, nil, false)
		result <- err
	}()
	deadline := scheduler.next(t, time.Second)
	poll := scheduler.next(t, 10*time.Millisecond)
	deadline.fired <- time.Time{}
	awaitReadiness(t, result, context.DeadlineExceeded)
	deadline.assertStopped(t)
	poll.assertStopped(t)
	if recording.finalizeCalls != 1 {
		t.Fatalf("replacement finalization calls = %d, want canceled run joined and finalized", recording.finalizeCalls)
	}
}

type blockingLifecycleFactory struct {
	lifecycleObserverFactory
}

func (f *blockingLifecycleFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestStartReplacement_AttachesSidecarsAfterReadinessInServiceMode(t *testing.T) {
	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	var attachCalls int
	handle, err := factoryhost.StartReplacement(
		context.Background(),
		context.Background(),
		&factoryhost.Bundle{
			Factory: factoryStub,
			Logger:  zap.NewNop(),
		},
		clockwork.NewFakeClock(),
		platformclock.Real{},
		func(_ context.Context, replacement *factoryhost.Handle) error {
			if replacement == nil {
				t.Fatal("replacement handle is required")
			}
			attachCalls++
			return nil
		},
		true,
	)
	if err != nil {
		t.Fatalf("StartReplacement: %v", err)
	}
	if attachCalls != 1 {
		t.Fatalf("sidecar attach calls = %d, want 1", attachCalls)
	}
	t.Cleanup(func() {
		_ = factoryhost.Stop(handle, clockwork.NewFakeClock())
	})
}

func TestStartReplacement_StopsReplacementWhenSidecarAttachFails(t *testing.T) {
	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	handle, err := factoryhost.StartReplacement(
		context.Background(),
		context.Background(),
		&factoryhost.Bundle{
			Factory: factoryStub,
			Logger:  zap.NewNop(),
		},
		clockwork.NewFakeClock(),
		platformclock.Real{},
		func(context.Context, *factoryhost.Handle) error {
			return fmt.Errorf("sidecar startup failed")
		},
		true,
	)
	if err == nil {
		if handle != nil {
			_ = factoryhost.Stop(handle, clockwork.NewFakeClock())
		}
		t.Fatal("expected sidecar attach failure")
	}
	if err.Error() != "start replacement runtime sidecars: sidecar startup failed" {
		t.Fatalf("StartReplacement error = %v, want sidecar startup failure", err)
	}
}

func TestReplacementAttempt_RestoresPriorSidecarsOnFailure(t *testing.T) {
	current := &factoryhost.Handle{RunDone: make(chan struct{})}
	close(current.RunDone)

	var restoreCalls int
	attempt := &factoryhost.ReplacementAttempt{
		Current:     current,
		ServiceCtx:  context.Background(),
		ServiceMode: true,
		RestoreSidecars: func(_ context.Context, handle *factoryhost.Handle) error {
			if handle != current {
				t.Fatalf("restore handle = %p, want %p", handle, current)
			}
			restoreCalls++
			return nil
		},
	}

	attempt.Begin()
	attempt.End()
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", restoreCalls)
	}
}

func TestReplacementAttempt_DoesNotRestoreAfterCommit(t *testing.T) {
	current := &factoryhost.Handle{RunDone: make(chan struct{})}
	close(current.RunDone)

	var restoreCalls int
	attempt := &factoryhost.ReplacementAttempt{
		Current:     current,
		ServiceCtx:  context.Background(),
		ServiceMode: true,
		RestoreSidecars: func(context.Context, *factoryhost.Handle) error {
			restoreCalls++
			return nil
		},
	}

	attempt.Begin()
	attempt.Commit()
	attempt.End()
	if restoreCalls != 0 {
		t.Fatalf("restore calls = %d, want 0 after commit", restoreCalls)
	}
}

func TestStopSidecars_WaitsForAttachedSidecars(t *testing.T) {
	handle := &factoryhost.Handle{
		RunDone: make(chan struct{}),
	}
	close(handle.RunDone)

	sidecarCtx, sidecarCancel := context.WithCancel(context.Background())
	handle.SidecarCancel = sidecarCancel
	handle.Sidecars.Add(1)

	var stopped sync.WaitGroup
	stopped.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		<-sidecarCtx.Done()
		stopped.Done()
	}()

	factoryhost.StopSidecars(handle)

	done := make(chan struct{})
	go func() {
		stopped.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sidecars to stop")
	}
	if handle.SidecarCancel != nil {
		t.Fatal("expected sidecar cancel to be cleared")
	}
}
