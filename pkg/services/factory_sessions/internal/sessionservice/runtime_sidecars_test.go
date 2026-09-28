package service

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type stubRuntimeSidecars struct {
	preseedCalls int
	startCalls   int
	stopCalls    int
}

func (s *stubRuntimeSidecars) Preseed(context.Context, factoryruntime.RuntimeRecord) error {
	s.preseedCalls++
	return nil
}

func (s *stubRuntimeSidecars) Start(context.Context, factoryruntime.RuntimeRun) error {
	s.startCalls++
	return nil
}

func (s *stubRuntimeSidecars) Stop(factoryruntime.RuntimeRun) {
	s.stopCalls++
}

type stubRuntimeLifecycle struct {
	stopSidecarsCalls int
}

func (s *stubRuntimeLifecycle) Start(context.Context, factoryruntime.RuntimeRecord) (factoryruntime.RuntimeRun, error) {
	return nil, nil
}

func (s *stubRuntimeLifecycle) WaitForStart(context.Context, factoryruntime.RuntimeRun) error {
	return nil
}

func (s *stubRuntimeLifecycle) Stop(factoryruntime.RuntimeRun) error {
	return nil
}

func (s *stubRuntimeLifecycle) StopSidecars(factoryruntime.RuntimeRun) {
	s.stopSidecarsCalls++
}

func (s *stubRuntimeLifecycle) PublishReplacement(context.Context, factoryruntime.RuntimeRun, factoryruntime.RuntimeRecord) error {
	return nil
}

func TestStopLiveRuntimeSidecars_UsesInjectedSidecarsExactlyOnce(t *testing.T) {
	t.Parallel()

	sidecars := &stubRuntimeSidecars{}
	runtime := &SessionRuntime{runtimeSidecars: sidecars}

	runtime.StopLiveRuntimeSidecars(nil)

	if sidecars.stopCalls != 1 {
		t.Fatalf("runtimeSidecars.Stop calls = %d, want 1", sidecars.stopCalls)
	}
}

func TestStopLiveRuntimeSidecars_MissingSidecarsSkipsLifecycleFallback(t *testing.T) {
	t.Parallel()

	lifecycle := &stubRuntimeLifecycle{}
	runtime := &SessionRuntime{runtimeLifecycle: lifecycle}

	runtime.StopLiveRuntimeSidecars(nil)

	if lifecycle.stopSidecarsCalls != 0 {
		t.Fatalf("runtimeLifecycle.StopSidecars calls = %d, want 0", lifecycle.stopSidecarsCalls)
	}
}
