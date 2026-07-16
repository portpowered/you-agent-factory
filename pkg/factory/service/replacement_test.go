package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"go.uber.org/zap"
)

func TestStartReplacement_StopsReplacementWhenReadinessFails(t *testing.T) {
	factoryStub := &blockingLifecycleFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStatePaused),
	})

	_, err := factoryservice.StartReplacement(factoryservice.StartReplacementInput{
		ReadinessCtx: context.Background(),
		ServiceCtx:   context.Background(),
		Bundle: &factoryservice.Bundle{
			Factory: factoryStub,
			Logger:  zap.NewNop(),
		},
		Clock: clockwork.NewFakeClock(),
	})
	if err == nil {
		t.Fatal("expected readiness failure")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StartReplacement error = %v, want deadline exceeded", err)
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
	handle, err := factoryservice.StartReplacement(factoryservice.StartReplacementInput{
		ReadinessCtx: context.Background(),
		ServiceCtx:   context.Background(),
		Bundle: &factoryservice.Bundle{
			Factory: factoryStub,
			Logger:  zap.NewNop(),
		},
		Clock:                       clockwork.NewFakeClock(),
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, replacement *factoryservice.Handle) error {
			if replacement == nil {
				t.Fatal("replacement handle is required")
			}
			attachCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("StartReplacement: %v", err)
	}
	if attachCalls != 1 {
		t.Fatalf("sidecar attach calls = %d, want 1", attachCalls)
	}
	t.Cleanup(func() {
		_ = factoryservice.Stop(handle, clockwork.NewFakeClock())
	})
}

func TestStartReplacement_StopsReplacementWhenSidecarAttachFails(t *testing.T) {
	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	handle, err := factoryservice.StartReplacement(factoryservice.StartReplacementInput{
		ReadinessCtx: context.Background(),
		ServiceCtx:   context.Background(),
		Bundle: &factoryservice.Bundle{
			Factory: factoryStub,
			Logger:  zap.NewNop(),
		},
		Clock:                       clockwork.NewFakeClock(),
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(context.Context, *factoryservice.Handle) error {
			return fmt.Errorf("sidecar startup failed")
		},
	})
	if err == nil {
		if handle != nil {
			_ = factoryservice.Stop(handle, clockwork.NewFakeClock())
		}
		t.Fatal("expected sidecar attach failure")
	}
	if err.Error() != "start replacement runtime sidecars: sidecar startup failed" {
		t.Fatalf("StartReplacement error = %v, want sidecar startup failure", err)
	}
}

func TestReplacementAttempt_RestoresPriorSidecarsOnFailure(t *testing.T) {
	current := &factoryservice.Handle{RunDone: make(chan struct{})}
	close(current.RunDone)

	var restoreCalls int
	attempt := &factoryservice.ReplacementAttempt{
		Current:     current,
		ServiceCtx:  context.Background(),
		ServiceMode: true,
		RestoreSidecars: func(_ context.Context, handle *factoryservice.Handle) error {
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
	current := &factoryservice.Handle{RunDone: make(chan struct{})}
	close(current.RunDone)

	var restoreCalls int
	attempt := &factoryservice.ReplacementAttempt{
		Current:     current,
		ServiceCtx:  context.Background(),
		ServiceMode: true,
		RestoreSidecars: func(context.Context, *factoryservice.Handle) error {
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
	handle := &factoryservice.Handle{
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

	factoryservice.StopSidecars(handle)

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
