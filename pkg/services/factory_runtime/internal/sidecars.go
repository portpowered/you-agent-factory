package internal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
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

// runtimeAutomationService is the optional runtime-owned capability set
// retained by Automations while the peer-facing Service stays root-only.
type runtimeAutomationService interface {
	automations.Service
	StartSchedulerSidecarsForRuntime(
		context.Context,
		*sync.WaitGroup,
		string,
		*interfaces.FactoryConfig,
		interfaces.RuntimeConfigLookup,
		automations.WorkRequestSubmitter,
	) error
	NewFilesystemWatcher(automations.FilesystemWatcherConfig) automations.FilesystemWatcher
}

// PreseedRuntimeInputs materializes listener-backed inputs before execution.
func PreseedRuntimeInputs(ctx context.Context, automation automations.Service, bundle *factoryhost.Bundle) error {
	if bundle == nil || automation == nil {
		return nil
	}
	runtimeAutomation, ok := automation.(runtimeAutomationService)
	if !ok {
		return nil
	}
	watcher := newFilesystemWatcher(runtimeAutomation, bundle)
	if watcher == nil {
		return nil
	}
	if err := watcher.PreseedInputs(ctx); err != nil {
		return fmt.Errorf("preseed inputs: %w", err)
	}
	return nil
}

func NewRuntimeSidecars(automation automations.Service, enabled bool) *RuntimeSidecars {
	return &RuntimeSidecars{automation: automation, enabled: enabled}
}

func (s *RuntimeSidecars) Preseed(ctx context.Context, instance factory.HostedInstance) error {
	bundle, _ := instance.(*factoryhost.Bundle)
	if instance != nil && bundle == nil {
		return fmt.Errorf("factory runtime service requires a built runtime instance")
	}
	return PreseedRuntimeInputs(ctx, s.automation, bundle)
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
	if runtimeAutomation, ok := s.automation.(runtimeAutomationService); ok {
		if watcher := newFilesystemWatcher(runtimeAutomation, handle.Bundle); watcher != nil {
			handle.Sidecars.Add(1)
			go func() {
				defer handle.Sidecars.Done()
				if err := watcher.Watch(sidecarCtx); err != nil && !errors.Is(err, context.Canceled) {
					handle.Bundle.RuntimeLogger().Error("file watcher error", zap.Error(err))
				}
			}()
		}
	}
	if schedules, ok := s.automation.(invocationScheduleService); ok {
		runtimeCfg := handle.Bundle.RuntimeCfg
		if runtimeCfg == nil {
			return s.failStart(handle, cancel, fmt.Errorf("runtime config is required"))
		}
		var scheduleFactory *invocationScheduleFactory
		if existing, wrapped := handle.Bundle.Factory.(*invocationScheduleFactory); wrapped {
			existing.setContext(sidecarCtx)
			scheduleFactory = existing
		} else {
			scheduleFactory = &invocationScheduleFactory{
				Factory: handle.Bundle.Factory, schedules: schedules,
				factoryDir: runtimeCfg.FactoryDir(), factoryConfig: runtimeCfg.FactoryConfig(),
				runtimeConfig: runtimeCfg, ctx: sidecarCtx,
			}
			handle.Bundle.Factory = scheduleFactory
		}
		if err := scheduleFactory.recoverInvocationSchedules(sidecarCtx); err != nil {
			return s.failStart(handle, cancel, fmt.Errorf("recover invocation schedules: %w", err))
		}
	}

	if s.enabled {
		runtimeAutomation, ok := s.automation.(runtimeAutomationService)
		if !ok {
			return s.failStart(handle, cancel, fmt.Errorf("automation service is required"))
		}
		runtimeCfg := handle.Bundle.RuntimeCfg
		if runtimeCfg == nil {
			return s.failStart(handle, cancel, fmt.Errorf("runtime config is required"))
		}
		if err := runtimeAutomation.StartSchedulerSidecarsForRuntime(
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
	// Start the observer after schedule recovery has finished. Recovery may
	// replace handle.Bundle.Factory, and the observer reads that field from its
	// goroutine; starting it earlier creates a startup data race.
	handle.Sidecars.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		factoryhost.ObserveRuntimeMetrics(sidecarCtx, handle)
	}()
	return nil
}

func newFilesystemWatcher(automation runtimeAutomationService, bundle *factoryhost.Bundle) automations.FilesystemWatcher {
	if automation == nil || bundle == nil || bundle.Factory == nil {
		return nil
	}
	if bundle.InputFiles == nil || bundle.InputDirectoryWalker == nil || bundle.WorkRequestIDs == nil {
		return nil
	}
	inputsDir := filepath.Join(bundle.Dir, interfaces.InputsDir)
	return automation.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir:               inputsDir,
		Logger:            bundle.RuntimeLogger(),
		ValidStatesByType: state.ValidStatesByType(bundle.Net.WorkTypes),
		Files:             bundle.InputFiles,
		WalkDirectory:     automations.FilesystemDirectoryWalker(bundle.InputDirectoryWalker),
		WorkRequestIDs:    bundle.WorkRequestIDs,
		Submitter: automations.WorkRequestSubmitter(func(ctx context.Context, request work.WorkRequest) error {
			_, err := bundle.Factory.SubmitWorkRequest(ctx, request)
			return err
		}),
	})
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
