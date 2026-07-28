package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

type blockingReplaceFactory struct {
	executeObserverFactory
}

func (f *blockingReplaceFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func attachSidecarsToHandle(
	t *testing.T,
	handle factory.HostedHandle,
	attach func(context.Context, factory.HostedHandle) error,
) {
	t.Helper()
	if attach == nil {
		return
	}
	if err := attach(context.Background(), handle); err != nil {
		t.Fatalf("attach sidecars: %v", err)
	}
}

func startReadyHostedHandleWithSidecars(
	t *testing.T,
	host *Host,
	factoryStub *lifecycleControlFactory,
	instanceID string,
	attach func(context.Context, factory.HostedHandle) error,
) factory.HostedHandle {
	t.Helper()
	handle := startReadyHostedHandle(t, host, factoryStub, instanceID)
	attachSidecarsToHandle(t, handle, attach)
	return handle
}

func TestReplaceSuccessfulStartsReplacementAttachesSidecarsAndSwapsActiveHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	var currentAttachCalls, currentRestoreCalls, replacementAttachCalls int
	currentAttach := func(_ context.Context, handle factory.HostedHandle) error {
		if handle == nil {
			t.Fatal("current sidecar attach requires handle")
		}
		currentAttachCalls++
		return nil
	}
	current := startReadyHostedHandleWithSidecars(
		t, host, currentFactory, "runtime-replace-current", currentAttach,
	)

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-next")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replacementHandle, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            ctx,
		ServiceContext:              ctx,
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			if handle == nil {
				t.Fatal("replacement sidecar attach requires handle")
			}
			concrete, ok := handle.(*factoryhost.Handle)
			if !ok {
				t.Fatalf("replacement handle type = %T, want *factoryhost.Handle", handle)
			}
			if concrete.Bundle != nil && concrete.Bundle.RuntimeInstanceID == "runtime-replace-next" {
				replacementAttachCalls++
				return nil
			}
			currentRestoreCalls++
			if handle != current {
				t.Fatalf("restore handle = %p, want current %p", handle, current)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if replacementHandle == nil || replacementHandle == current {
		t.Fatalf("Replace() = %v, want distinct replacement handle", replacementHandle)
	}
	if replacementAttachCalls != 1 {
		t.Fatalf("replacement sidecar attach calls = %d, want 1", replacementAttachCalls)
	}
	if currentRestoreCalls != 0 {
		t.Fatalf("restore calls after successful replace = %d, want 0", currentRestoreCalls)
	}
	if host.handles["runtime-replace-next"] != replacementHandle {
		t.Fatal("replacement handle is not the active registered handle")
	}
	if _, ok := host.handles["runtime-replace-current"]; ok {
		t.Fatal("prior handle remained registered after successful replace")
	}
	if len(host.handles) != 1 {
		t.Fatalf("active handles = %d, want one after successful replace", len(host.handles))
	}
	if currentAttachCalls != 1 {
		t.Fatalf("current sidecar attach calls = %d, want initial attach only", currentAttachCalls)
	}
}

func TestReplaceStopsReplacementWhenReadinessFailsAndKeepsPriorHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	var restoreCalls int
	current := startReadyHostedHandleWithSidecars(
		t, host, currentFactory, "runtime-replace-readiness-fail",
		func(context.Context, factory.HostedHandle) error { return nil },
	)

	replacementFactory := &blockingReplaceFactory{}
	replacementFactory.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStatePaused),
	})
	replacementBundle := testBundle(replacementFactory, "runtime-replace-readiness-next")

	readinessCtx, cancelReadiness := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelReadiness()
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            readinessCtx,
		ServiceContext:              runCtx,
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			restoreCalls++
			if handle != current {
				t.Fatalf("restore handle = %p, want current %p", handle, current)
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("Replace() error = nil, want readiness failure")
	}
	if !strings.Contains(err.Error(), "start replacement Runtime") {
		t.Fatalf("Replace() error = %v, want wrapped readiness failure", err)
	}
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1 after readiness failure", restoreCalls)
	}
	if host.handles["runtime-replace-readiness-fail"] != current {
		t.Fatal("prior handle is not still the active registered handle after readiness failure")
	}
	if len(host.handles) != 1 {
		t.Fatalf("active handles = %d, want prior handle only", len(host.handles))
	}
}

func TestReplaceStopsReplacementWhenSidecarAttachFails(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandleWithSidecars(
		t, host, currentFactory, "runtime-replace-sidecar-fail",
		func(context.Context, factory.HostedHandle) error { return nil },
	)

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-sidecar-next")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var restoreCalls int
	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            ctx,
		ServiceContext:              ctx,
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			concrete, ok := handle.(*factoryhost.Handle)
			if !ok {
				return fmt.Errorf("replacement handle type = %T, want *factoryhost.Handle", handle)
			}
			if concrete.Bundle != nil && concrete.Bundle.RuntimeInstanceID == "runtime-replace-sidecar-next" {
				return fmt.Errorf("sidecar startup failed")
			}
			restoreCalls++
			return nil
		},
	})
	if err == nil {
		t.Fatal("Replace() error = nil, want sidecar attach failure")
	}
	if !strings.Contains(err.Error(), "start replacement runtime sidecars") {
		t.Fatalf("Replace() error = %v, want sidecar attach failure", err)
	}
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1 after sidecar failure", restoreCalls)
	}
	if host.handles["runtime-replace-sidecar-fail"] != current {
		t.Fatal("prior handle is not still active after sidecar attach failure")
	}
}

func TestReplaceRestoresPriorSidecarsOnFailure(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandle(t, host, currentFactory, "runtime-replace-restore")

	replacementFactory := &blockingReplaceFactory{}
	replacementFactory.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateIdle),
	})
	replacementBundle := testBundle(replacementFactory, "runtime-replace-restore-next")

	var restoreCalls int
	readinessCtx, cancelReadiness := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelReadiness()

	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            readinessCtx,
		ServiceContext:              context.Background(),
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			if handle != current {
				t.Fatalf("restore handle = %p, want current %p", handle, current)
			}
			restoreCalls++
			return nil
		},
	})
	if err == nil {
		t.Fatal("Replace() error = nil, want failure")
	}
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", restoreCalls)
	}
}

func TestReplaceDoesNotRestorePriorSidecarsAfterCommit(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandle(t, host, currentFactory, "runtime-replace-no-restore")

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-no-restore-next")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var restoreCalls int
	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            ctx,
		ServiceContext:              ctx,
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			concrete, ok := handle.(*factoryhost.Handle)
			if ok && concrete.Bundle != nil && concrete.Bundle.RuntimeInstanceID == "runtime-replace-no-restore-next" {
				return nil
			}
			restoreCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if restoreCalls != 0 {
		t.Fatalf("restore calls after commit = %d, want 0", restoreCalls)
	}
}

func TestReplaceRejectsInvalidCurrentAndReplacement(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandle(t, host, currentFactory, "runtime-replace-invalid")
	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-invalid-next")
	ctx := context.Background()

	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext: ctx,
		ServiceContext:   ctx,
		Current:          nil,
		Replacement:      replacementBundle,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a runtime handle") {
		t.Fatalf("Replace(nil current) error = %v, want runtime-handle validation failure", err)
	}

	_, err = host.Replace(instancehost.ReplaceRequest{
		ReadinessContext: ctx,
		ServiceContext:   ctx,
		Current:          current,
		Replacement:      nil,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a built runtime instance") {
		t.Fatalf("Replace(nil replacement) error = %v, want built-instance validation error", err)
	}
}

func TestReplaceStopsSidecarsBeforeAttemptInServiceMode(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	currentConcrete := startReadyHostedHandle(t, host, currentFactory, "runtime-replace-stop-sidecars").(*factoryhost.Handle)

	sidecarCtx, sidecarCancel := context.WithCancel(context.Background())
	currentConcrete.SidecarCancel = sidecarCancel
	currentConcrete.Sidecars.Add(1)
	var stopped sync.WaitGroup
	stopped.Add(1)
	go func() {
		defer currentConcrete.Sidecars.Done()
		<-sidecarCtx.Done()
		stopped.Done()
	}()

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-stop-sidecars-next")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            ctx,
		ServiceContext:              ctx,
		Current:                     currentConcrete,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			if handle == currentConcrete {
				return nil
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		stopped.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prior sidecars to stop before replacement attempt")
	}
	if currentConcrete.SidecarCancel != nil {
		t.Fatal("expected prior sidecar cancel to be cleared during replacement attempt")
	}
}
