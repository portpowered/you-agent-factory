package internal

import (
	"context"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// runtimeLifecycleAdapter keeps the legacy host implementation behind the
// neutral Runtime opening contract. The adapter is Factory Runtime-owned; no
// peer service needs to import or name the hosted lifecycle types.
type runtimeLifecycleAdapter struct {
	lifecycle factoryruntime.Lifecycle
}

func adaptRuntimeLifecycle(lifecycle factoryruntime.Lifecycle) factoryruntime.RuntimeLifecycle {
	if lifecycle == nil {
		return nil
	}
	return runtimeLifecycleAdapter{lifecycle: lifecycle}
}

func (adapter runtimeLifecycleAdapter) Start(
	ctx context.Context,
	instance factoryruntime.RuntimeRecord,
) (factoryruntime.RuntimeRun, error) {
	hosted, err := hostedInstance(instance)
	if err != nil {
		return nil, err
	}
	handle, err := adapter.lifecycle.Start(ctx, hosted)
	if err != nil {
		return nil, err
	}
	return runtimeRunAdapter{handle: handle}, nil
}

func (adapter runtimeLifecycleAdapter) WaitForStart(ctx context.Context, run factoryruntime.RuntimeRun) error {
	handle, err := hostedHandle(run)
	if err != nil {
		return err
	}
	return adapter.lifecycle.WaitForStart(ctx, handle)
}

func (adapter runtimeLifecycleAdapter) Stop(run factoryruntime.RuntimeRun) error {
	handle, err := hostedHandle(run)
	if err != nil {
		return err
	}
	return adapter.lifecycle.Stop(handle)
}

func (adapter runtimeLifecycleAdapter) StopSidecars(run factoryruntime.RuntimeRun) {
	handle, err := hostedHandle(run)
	if err == nil {
		adapter.lifecycle.StopSidecars(handle)
	}
}

func (adapter runtimeLifecycleAdapter) PublishReplacement(
	ctx context.Context,
	current factoryruntime.RuntimeRun,
	replacement factoryruntime.RuntimeRecord,
) error {
	currentHandle, err := hostedHandle(current)
	if err != nil {
		return err
	}
	replacementInstance, err := hostedInstance(replacement)
	if err != nil {
		return err
	}
	return adapter.lifecycle.PublishReplacement(ctx, currentHandle, replacementInstance)
}

type runtimeRunAdapter struct {
	handle factoryruntime.HostedHandle
}

func (adapter runtimeRunAdapter) RuntimeInstance() factoryruntime.RuntimeRecord {
	if adapter.handle == nil {
		return nil
	}
	return adapter.handle.RuntimeInstance()
}

func (adapter runtimeRunAdapter) Completed() bool {
	return adapter.handle == nil || adapter.handle.Completed()
}

func (adapter runtimeRunAdapter) Result() error {
	if adapter.handle == nil {
		return nil
	}
	return adapter.handle.Result()
}

func (adapter runtimeRunAdapter) Wait() error {
	if adapter.handle == nil {
		return nil
	}
	return adapter.handle.Wait()
}

func (adapter runtimeRunAdapter) CancelRun() {
	if adapter.handle != nil {
		adapter.handle.CancelRun()
	}
}

func (adapter runtimeRunAdapter) RunDoneCh() <-chan struct{} {
	if adapter.handle == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return adapter.handle.RunDoneCh()
}

func hostedInstance(instance factoryruntime.RuntimeRecord) (factoryruntime.HostedInstance, error) {
	if instance == nil {
		return nil, fmt.Errorf("Factory Runtime record is required")
	}
	hosted, ok := instance.(factoryruntime.HostedInstance)
	if !ok || hosted == nil {
		return nil, fmt.Errorf("Factory Runtime record is not supported by the host")
	}
	return hosted, nil
}

func hostedHandle(run factoryruntime.RuntimeRun) (factoryruntime.HostedHandle, error) {
	if run == nil {
		return nil, fmt.Errorf("Factory Runtime run is required")
	}
	if adapter, ok := run.(runtimeRunAdapter); ok {
		if adapter.handle == nil {
			return nil, fmt.Errorf("Factory Runtime run is not supported by the host")
		}
		return adapter.handle, nil
	}
	if adapter, ok := run.(*runtimeRunAdapter); ok {
		if adapter == nil || adapter.handle == nil {
			return nil, fmt.Errorf("Factory Runtime run is not supported by the host")
		}
		return adapter.handle, nil
	}
	return nil, fmt.Errorf("Factory Runtime run is not supported by the host")
}

type runtimeSidecarAdapter struct {
	sidecars factoryruntime.Sidecars
}

func adaptRuntimeSidecars(sidecars factoryruntime.Sidecars) factoryruntime.RuntimeSidecars {
	if sidecars == nil {
		return nil
	}
	return runtimeSidecarAdapter{sidecars: sidecars}
}

func (adapter runtimeSidecarAdapter) Preseed(ctx context.Context, instance factoryruntime.RuntimeRecord) error {
	hosted, err := hostedInstance(instance)
	if err != nil {
		return err
	}
	return adapter.sidecars.Preseed(ctx, hosted)
}

func (adapter runtimeSidecarAdapter) Start(ctx context.Context, run factoryruntime.RuntimeRun) error {
	handle, err := hostedHandle(run)
	if err != nil {
		return err
	}
	return adapter.sidecars.Start(ctx, handle)
}

func (adapter runtimeSidecarAdapter) Stop(run factoryruntime.RuntimeRun) {
	handle, err := hostedHandle(run)
	if err == nil {
		adapter.sidecars.Stop(handle)
	}
}

type runtimeReplacementAdapter struct {
	builder factoryruntime.ReplacementBuilder
}

func adaptRuntimeReplacementBuilder(builder factoryruntime.ReplacementBuilder) factoryruntime.RuntimeReplacementBuilder {
	if builder == nil {
		return nil
	}
	return runtimeReplacementAdapter{builder: builder}
}

func (adapter runtimeReplacementAdapter) BuildReplacement(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
	executionBaseDir string,
) (factoryruntime.RuntimeRecord, error) {
	return adapter.builder.BuildReplacement(ctx, folderPath, factoryDir, sessionID, executionBaseDir)
}
