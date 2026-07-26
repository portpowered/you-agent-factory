package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// RuntimeSidecars owns runtime-scoped metrics, input listeners, and
// automation. Initializer decides when to start it; Factory Runtime owns what
// starting it means.
type RuntimeSidecars struct {
	automation automations.Service
	enabled    bool
}

// PreseedRuntimeInputs materializes listener-backed inputs before execution.
func PreseedRuntimeInputs(ctx context.Context, bundle *factoryhost.Bundle) error {
	if bundle == nil || bundle.Listener == nil {
		return nil
	}
	if err := bundle.Listener.PreseedInputs(ctx); err != nil {
		return fmt.Errorf("preseed inputs: %w", err)
	}
	return nil
}

func NewRuntimeSidecars(automation automations.Service, enabled bool) *RuntimeSidecars {
	return &RuntimeSidecars{automation: automation, enabled: enabled}
}

func (*RuntimeSidecars) Preseed(ctx context.Context, instance factory.HostedInstance) error {
	bundle, _ := instance.(*factoryhost.Bundle)
	if instance != nil && bundle == nil {
		return fmt.Errorf("factory runtime service requires a built runtime instance")
	}
	return PreseedRuntimeInputs(ctx, bundle)
}

func (s *RuntimeSidecars) Start(ctx context.Context, hosted factory.HostedHandle) error {
	handle, _ := hosted.(*factoryhost.Handle)
	if handle == nil || handle.Bundle == nil {
		return fmt.Errorf("runtime handle is required")
	}
	handle.SidecarMu.Lock()
	defer handle.SidecarMu.Unlock()
	if handle.SidecarCancel != nil {
		return nil
	}

	sidecarCtx, cancel := context.WithCancel(ctx)
	handle.SidecarCancel = cancel
	handle.Sidecars.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		factoryhost.ObserveRuntimeMetrics(sidecarCtx, handle)
	}()
	if listener := handle.Bundle.Listener; listener != nil {
		handle.Sidecars.Add(1)
		go func() {
			defer handle.Sidecars.Done()
			if err := listener.Watch(sidecarCtx); err != nil && !errors.Is(err, context.Canceled) {
				handle.Bundle.RuntimeLogger().Error("file watcher error", zap.Error(err))
			}
		}()
	}

	if s.enabled {
		if s.automation == nil {
			return s.failStart(handle, cancel, fmt.Errorf("automation service is required"))
		}
		runtimeCfg := handle.Bundle.RuntimeCfg
		if runtimeCfg == nil {
			return s.failStart(handle, cancel, fmt.Errorf("runtime config is required"))
		}
		if err := s.automation.StartSchedulerSidecarsForRuntime(
			sidecarCtx,
			&handle.Sidecars,
			runtimeCfg.FactoryDir(),
			runtimeCfg.FactoryConfig(),
			runtimeCfg,
			automations.WorkRequestSubmitter(func(ctx context.Context, request work.WorkRequest) error {
				_, err := handle.Bundle.Factory.SubmitWorkRequest(ctx, request)
				return err
			}),
		); err != nil {
			return s.failStart(handle, cancel, fmt.Errorf("attach automation sidecars: %w", err))
		}
	}
	return nil
}

func (*RuntimeSidecars) failStart(handle *factoryhost.Handle, cancel context.CancelFunc, err error) error {
	cancel()
	handle.Sidecars.Wait()
	handle.SidecarCancel = nil
	return err
}

func (*RuntimeSidecars) Stop(hosted factory.HostedHandle) {
	handle, _ := hosted.(*factoryhost.Handle)
	factoryhost.StopSidecars(handle)
}
