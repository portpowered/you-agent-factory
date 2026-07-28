package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

type gatedRunFactory struct {
	*lifecycleControlFactory
	enterRun   chan struct{}
	releaseRun chan struct{}
}

func newGatedRunFactory(state interfaces.FactoryState) *gatedRunFactory {
	return &gatedRunFactory{
		lifecycleControlFactory: newLifecycleControlFactory(state),
		enterRun:                make(chan struct{}),
		releaseRun:              make(chan struct{}),
	}
}

func (f *gatedRunFactory) Run(ctx context.Context) error {
	close(f.enterRun)
	select {
	case <-f.releaseRun:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestConcurrentStartAdmitsOneActiveHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	bundle := testBundle(factoryStub, "runtime-concurrent-start")
	ctx := context.Background()

	const attempts = 32
	var wg sync.WaitGroup
	successes := make(chan factory.HostedHandle, attempts)
	failures := make(chan error, attempts)
	for index := 0; index < attempts; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, err := host.Start(ctx, bundle)
			if err != nil {
				failures <- err
				return
			}
			successes <- handle
		}()
	}
	wg.Wait()
	close(successes)
	close(failures)

	var handles []factory.HostedHandle
	for handle := range successes {
		handles = append(handles, handle)
	}
	if len(handles) != 1 {
		t.Fatalf("successful starts = %d, want 1", len(handles))
	}
	for err := range failures {
		if err == nil || !strings.Contains(err.Error(), "already has an active hosted handle") {
			t.Fatalf("concurrent Start() failure = %v, want duplicate-active rejection", err)
		}
	}
	if len(host.handles) != 1 {
		t.Fatalf("registry size = %d, want 1", len(host.handles))
	}
	if err := host.Stop(handles[0]); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(host.handles) != 0 {
		t.Fatalf("registry after stop = %d, want empty", len(host.handles))
	}
}

func TestConcurrentStartAndTerminateConvergesWithoutOrphanedHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newGatedRunFactory(interfaces.FactoryStateIdle)
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateIdle),
	})
	bundle := testBundle(factoryStub, "runtime-start-terminate-race")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	<-factoryStub.enterRun

	var wg sync.WaitGroup
	wg.Add(2)
	var stopErr error
	go func() {
		defer wg.Done()
		_ = host.WaitForStart(ctx, handle)
	}()
	go func() {
		defer wg.Done()
		stopErr = host.Stop(handle)
	}()
	wg.Wait()
	close(factoryStub.releaseRun)
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		t.Fatalf("Stop() error = %v", stopErr)
	}
	if len(host.handles) != 0 {
		t.Fatalf("registry after concurrent start/terminate = %d, want empty", len(host.handles))
	}
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || !concrete.Completed() {
		t.Fatal("terminated handle should be completed without orphaned ready registry entry")
	}
}

func TestConcurrentPauseResumeNeverCreatesSecondHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-concurrent-pause-resume")

	const submissions = 64
	var wg sync.WaitGroup
	var invalidOutcomes atomic.Int64
	for index := 0; index < submissions; index++ {
		wg.Add(1)
		go func(pause bool) {
			defer wg.Done()
			ctx := context.Background()
			if pause {
				result, err := host.Pause(ctx, handle)
				if err != nil {
					invalidOutcomes.Add(1)
					return
				}
				if result.Outcome != factory.ControlOutcomeAccepted &&
					result.Outcome != factory.ControlOutcomeNoOp {
					invalidOutcomes.Add(1)
				}
				return
			}
			result, err := host.Resume(ctx, handle)
			if err != nil {
				invalidOutcomes.Add(1)
				return
			}
			if result.Outcome != factory.ControlOutcomeAccepted &&
				result.Outcome != factory.ControlOutcomeNoOp {
				invalidOutcomes.Add(1)
			}
		}(index%2 == 0)
	}
	wg.Wait()
	if invalidOutcomes.Load() != 0 {
		t.Fatalf("invalid pause/resume outcomes = %d, want only accepted or no-op", invalidOutcomes.Load())
	}
	if len(host.handles) != 1 {
		t.Fatalf("handles after concurrent pause/resume = %d, want single active handle", len(host.handles))
	}
}

func TestConcurrentReplaceAndTerminateDoesNotCommitReplacement(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandle(t, host, currentFactory, "runtime-replace-terminate-current")

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-replace-terminate-next")
	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	defer serviceCancel()
	readinessCtx := context.Background()
	replacementReady := make(chan struct{})
	releaseReplace := make(chan struct{})
	var replacementReadyOnce sync.Once

	var replaceErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, replaceErr = host.Replace(instancehost.ReplaceRequest{
			ReadinessContext:            readinessCtx,
			ServiceContext:              serviceCtx,
			Current:                     current,
			Replacement:                 replacementBundle,
			AttachSidecarsInServiceMode: true,
			AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
				concrete, ok := handle.(*factoryhost.Handle)
				if !ok || concrete == nil || concrete.Bundle == nil {
					return nil
				}
				if concrete.Bundle.RuntimeInstanceID != "runtime-replace-terminate-next" {
					return nil
				}
				replacementReadyOnce.Do(func() { close(replacementReady) })
				<-releaseReplace
				return nil
			},
		})
	}()

	select {
	case <-replacementReady:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement readiness did not arrive before timeout")
	}

	stopErr := host.Stop(current)
	close(releaseReplace)
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		t.Fatalf("Stop(current) error = %v", stopErr)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Replace() did not return after terminate won")
	}
	if !errors.Is(replaceErr, factory.ErrNotRunning) {
		t.Fatalf("Replace() after terminate = %v, want ErrNotRunning", replaceErr)
	}
	if len(host.handles) != 0 {
		t.Fatalf("registry after replace/terminate race = %d, want empty", len(host.handles))
	}
}

func TestExecuteFailureUnwindsInReverseStartupOrder(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	bundle := testBundle(factoryStub, "runtime-unwind-order")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var order []string
	var orderMu sync.Mutex
	record := func(step string) {
		orderMu.Lock()
		defer orderMu.Unlock()
		order = append(order, step)
	}

	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}

	attach := func(_ context.Context, hosted factory.HostedHandle) error {
		concrete, ok := hosted.(*factoryhost.Handle)
		if !ok || concrete == nil {
			return errors.New("handle required")
		}
		record("sidecar-start")
		sidecarCtx, sidecarCancel := context.WithCancel(context.Background())
		concrete.SidecarMu.Lock()
		concrete.SidecarCancel = sidecarCancel
		concrete.SidecarMu.Unlock()
		concrete.Sidecars.Add(1)
		go func() {
			defer concrete.Sidecars.Done()
			<-sidecarCtx.Done()
			record("sidecar-stop")
		}()
		return nil
	}
	if err := attach(ctx, handle); err != nil {
		t.Fatalf("attach sidecars: %v", err)
	}

	record("stop-request")
	if err := host.Stop(handle); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v", err)
	}
	record("stop-complete")

	orderMu.Lock()
	defer orderMu.Unlock()
	want := []string{"sidecar-start", "stop-request", "sidecar-stop", "stop-complete"}
	if len(order) != len(want) {
		t.Fatalf("unwind order = %#v, want %#v", order, want)
	}
	for index, step := range want {
		if order[index] != step {
			t.Fatalf("unwind order[%d] = %q, want %q (full %#v)", index, order[index], step, order)
		}
	}
}
