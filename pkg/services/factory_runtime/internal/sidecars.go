package internal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
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
	automation   automations.Service
	enabled      bool
	metricsClock platformclock.TimerSource
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

type runtimeAutomationLifecycle interface {
	automations.Service
	ActivateRuntime(context.Context, automations.RuntimeActivationRequest) (automations.RuntimeActivationResult, error)
	DeactivateRuntime(context.Context, automations.RuntimeDeactivationRequest) (automations.RuntimeDeactivationResult, error)
	StartRuntime(context.Context, string) error
}

type runtimeAutomationStarter interface {
	StartRuntime(context.Context, string) error
}

// PreseedRuntimeInputs materializes listener-backed inputs before execution.
func PreseedRuntimeInputs(ctx context.Context, automation automations.Service, bundle *factoryhost.Bundle) error {
	if bundle == nil || automation == nil {
		return nil
	}
	if lifecycle, ok := automation.(runtimeAutomationLifecycle); ok {
		if _, err := lifecycle.ActivateRuntime(ctx, runtimeActivationRequest(bundle, false)); err != nil {
			return fmt.Errorf("activate automation runtime: %w", err)
		}
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

func NewRuntimeSidecars(
	automation automations.Service,
	enabled bool,
	metricsClock platformclock.TimerSource,
) *RuntimeSidecars {
	return &RuntimeSidecars{
		automation: automation, enabled: enabled, metricsClock: metricsClock,
	}
}

func (s *RuntimeSidecars) Preseed(ctx context.Context, instance factory.RuntimeRecord) error {
	bundle, _ := instance.(*factoryhost.Bundle)
	if instance != nil && bundle == nil {
		return fmt.Errorf("factory runtime service requires a built runtime instance")
	}
	if lifecycle, ok := s.automation.(runtimeAutomationLifecycle); ok {
		if _, err := lifecycle.ActivateRuntime(ctx, runtimeActivationRequest(bundle, s.enabled)); err != nil {
			return fmt.Errorf(
				"activate automation runtime for Factory Session %q (Runtime %q): %w",
				bundle.FactorySessionID,
				bundle.RuntimeInstanceID,
				err,
			)
		}
		return nil
	}
	return PreseedRuntimeInputs(ctx, s.automation, bundle)
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (s *RuntimeSidecars) Start(ctx context.Context, hosted factory.RuntimeRun) error {
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
	lifecycle, lifecycleActive := s.automation.(runtimeAutomationLifecycle)
	var runtimeStarter runtimeAutomationStarter
	if lifecycleActive {
		if _, err := lifecycle.ActivateRuntime(sidecarCtx, runtimeActivationRequest(handle.Bundle, s.enabled)); err != nil {
			return s.failStart(handle, cancel, fmt.Errorf("activate automation runtime for Factory Session %q (Runtime %q): %w", handle.Bundle.FactorySessionID, handle.Bundle.RuntimeInstanceID, err))
		}
		starter, ok := s.automation.(runtimeAutomationStarter)
		if !ok {
			return s.failStart(handle, cancel, fmt.Errorf("automations runtime starter is required"))
		}
		runtimeStarter = starter
	} else if runtimeAutomation, ok := s.automation.(runtimeAutomationService); ok {
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
				Engine: handle.Bundle.Factory, schedules: schedules,
				runtimeID:  handle.Bundle.RuntimeInstanceID,
				factoryDir: runtimeCfg.FactoryDir(), factoryConfig: runtimeCfg.FactoryConfig(),
				runtimeConfig: runtimeCfg, ctx: sidecarCtx,
			}
			handle.Bundle.Factory = scheduleFactory
		}
		if err := scheduleFactory.recoverInvocationSchedules(sidecarCtx); err != nil {
			return s.failStart(handle, cancel, fmt.Errorf("recover invocation schedules: %w", err))
		}
	}
	if runtimeStarter != nil {
		if err := runtimeStarter.StartRuntime(sidecarCtx, handle.Bundle.RuntimeInstanceID); err != nil {
			return s.failStart(handle, cancel, fmt.Errorf("start automation runtime: %w", err))
		}
	}

	if s.enabled && !lifecycleActive {
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
		factoryhost.ObserveRuntimeMetrics(sidecarCtx, handle, s.metricsClock)
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

func runtimeActivationRequest(bundle *factoryhost.Bundle, startSchedulers bool) automations.RuntimeActivationRequest {
	if bundle == nil || bundle.RuntimeCfg == nil {
		return automations.RuntimeActivationRequest{}
	}
	cfg := bundle.RuntimeCfg.FactoryConfig()
	if cfg == nil {
		return automations.RuntimeActivationRequest{
			RuntimeID: bundle.RuntimeInstanceID, FactorySessionID: bundle.FactorySessionID,
		}
	}
	snapshot := interfaces.RuntimeSnapshot{
		// The runtime host directory is the active session scope. A replayed
		// loaded config may retain the recorded source directory, but automation
		// watchers and scheduler effects must never observe that historical path.
		FactoryDir:       bundle.Dir,
		RuntimeBaseDir:   bundle.RuntimeCfg.RuntimeBaseDir(),
		EffectiveFactory: *cfg,
		Workers:          append([]interfaces.FactoryWorkerConfig(nil), cfg.Workers...),
		Workstations:     append([]interfaces.FactoryWorkstationConfig(nil), cfg.Workstations...),
	}
	request := automations.RuntimeActivationRequest{
		RuntimeID: bundle.RuntimeInstanceID, FactorySessionID: bundle.FactorySessionID,
		Snapshot: snapshot,
		Inputs: automations.RuntimeActivationInputs{
			StartSchedulers: startSchedulers,
			Submitter: automations.WorkRequestSubmitter(func(ctx context.Context, request work.WorkRequest) error {
				_, err := bundle.Factory.SubmitWorkRequest(ctx, request)
				return err
			}),
		},
	}
	if bundle.InputFiles != nil && bundle.InputDirectoryWalker != nil && bundle.WorkRequestIDs != nil {
		knownWorkTypes := make([]string, 0)
		if bundle.Net != nil {
			knownWorkTypes = make([]string, 0, len(bundle.Net.WorkTypes))
			for workType := range bundle.Net.WorkTypes {
				knownWorkTypes = append(knownWorkTypes, workType)
			}
			sort.Strings(knownWorkTypes)
		}
		request.Inputs.Filesystem = automations.RuntimeFilesystemInputs{
			Files:             bundle.InputFiles,
			WalkDirectory:     automations.FilesystemDirectoryWalker(bundle.InputDirectoryWalker),
			WorkRequestIDs:    bundle.WorkRequestIDs,
			KnownWorkTypes:    knownWorkTypes,
			ValidStatesByType: validStatesForBundle(bundle),
		}
	}
	return request
}

func validStatesForBundle(bundle *factoryhost.Bundle) map[string]map[string]bool {
	if bundle == nil || bundle.Net == nil {
		return nil
	}
	return state.ValidStatesByType(bundle.Net.WorkTypes)
}

func (s *RuntimeSidecars) failStart(handle *factoryhost.Handle, cancel context.CancelFunc, err error) error {
	cancel()
	handle.Sidecars.Wait()
	if lifecycle, ok := s.automation.(runtimeAutomationLifecycle); ok && handle != nil && handle.Bundle != nil {
		_, _ = lifecycle.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{
			RuntimeID: handle.Bundle.RuntimeInstanceID,
		})
	}
	handle.SidecarCancel = nil
	return err
}

func (s *RuntimeSidecars) Stop(hosted factory.RuntimeRun) {
	handle, _ := hosted.(*factoryhost.Handle)
	if handle != nil && handle.Bundle != nil {
		if lifecycle, ok := s.automation.(runtimeAutomationLifecycle); ok {
			if _, err := lifecycle.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{
				RuntimeID: handle.Bundle.RuntimeInstanceID,
			}); err != nil {
				handle.Bundle.RuntimeLogger().Error("deactivate automation runtime failed", zap.Error(err))
			}
		}
	}
	factoryhost.StopSidecars(handle)
}
