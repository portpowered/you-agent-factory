package internal

import (
	"context"
	"fmt"
	"sort"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// RuntimeSidecars supplies metrics, input-listener and automation behavior.
// Runtime handles own scoped state; Initializer decides when to start it.
type RuntimeSidecars struct {
	automation       runtimeAutomationLifecycle
	enabled          bool
	metricsClock     platformclock.TimerSource
	historicalReplay bool
}

// RuntimeAutomationLifecycle is the complete process-selected Automations role.
type RuntimeAutomationLifecycle = runtimeAutomationLifecycle

type runtimeAutomationLifecycle interface {
	automations.Service
	ActivateRuntime(context.Context, automations.RuntimeActivationRequest) (automations.RuntimeActivationResult, error)
	DeactivateRuntime(context.Context, automations.RuntimeDeactivationRequest) (automations.RuntimeDeactivationResult, error)
	StartRuntime(context.Context, string) error
}

// SidecarOpening retains process-selected behavior. Runtime handles own all
// cancellation and goroutine state, so independent sessions can share it.
type SidecarOpening struct {
	foreground RuntimeSidecars
	service    RuntimeSidecars
	historical RuntimeSidecars
}

func NewSidecarOpening(
	automation runtimeAutomationLifecycle,
	metricsClock platformclock.TimerSource,
) *SidecarOpening {
	return &SidecarOpening{
		foreground: RuntimeSidecars{automation: automation, metricsClock: metricsClock},
		service:    RuntimeSidecars{automation: automation, enabled: true, metricsClock: metricsClock},
		historical: RuntimeSidecars{automation: automation, metricsClock: metricsClock, historicalReplay: true},
	}
}

func (s *SidecarOpening) Scope(serviceMode, historicalReplay bool) factory.RuntimeSidecars {
	if historicalReplay {
		return &s.historical
	}
	if serviceMode {
		return &s.service
	}
	return &s.foreground
}

func (s *RuntimeSidecars) Preseed(ctx context.Context, instance factory.RuntimeRecord) error {
	bundle, _ := instance.(*factoryhost.Bundle)
	if instance != nil && bundle == nil {
		return fmt.Errorf("factory runtime service requires a built runtime instance")
	}
	if s.historicalReplay {
		return nil
	}
	if _, err := s.automation.ActivateRuntime(ctx, runtimeActivationRequest(bundle, s.enabled)); err != nil {
		return fmt.Errorf("activate automation runtime for Factory Session %q (Runtime %q): %w", bundle.FactorySessionID, bundle.RuntimeInstanceID, err)
	}
	return nil
}

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
	if !s.historicalReplay {
		if err := s.startAdmission(sidecarCtx, handle, cancel); err != nil {
			return err
		}
	}
	// Start observation after schedule recovery and live admission are ready.
	handle.Sidecars.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		factoryhost.ObserveRuntimeMetrics(sidecarCtx, handle, s.metricsClock)
	}()
	return nil
}

// startAdmission activates live inputs and schedules for this runtime only.
func (s *RuntimeSidecars) startAdmission(sidecarCtx context.Context, handle *factoryhost.Handle, cancel context.CancelFunc) error {
	if _, err := s.automation.ActivateRuntime(sidecarCtx, runtimeActivationRequest(handle.Bundle, s.enabled)); err != nil {
		return s.failStart(sidecarCtx, handle, cancel, fmt.Errorf("activate automation runtime for Factory Session %q (Runtime %q): %w", handle.Bundle.FactorySessionID, handle.Bundle.RuntimeInstanceID, err))
	}
	if err := handle.Bundle.Factory.RecoverInvocationSchedules(sidecarCtx); err != nil {
		return s.failStart(sidecarCtx, handle, cancel, fmt.Errorf("recover invocation schedules: %w", err))
	}
	if err := s.automation.StartRuntime(sidecarCtx, handle.Bundle.RuntimeInstanceID); err != nil {
		return s.failStart(sidecarCtx, handle, cancel, fmt.Errorf("start automation runtime: %w", err))
	}
	return nil
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

func (s *RuntimeSidecars) failStart(ctx context.Context, handle *factoryhost.Handle, cancel context.CancelFunc, err error) error {
	cancel()
	handle.Sidecars.Wait()
	if handle.Bundle != nil {
		_, _ = s.automation.DeactivateRuntime(context.WithoutCancel(ctx), automations.RuntimeDeactivationRequest{
			RuntimeID: handle.Bundle.RuntimeInstanceID,
		})
	}
	handle.SidecarCancel = nil
	return err
}

func (s *RuntimeSidecars) Stop(hosted factory.RuntimeRun) {
	handle, _ := hosted.(*factoryhost.Handle)
	if !s.historicalReplay && handle != nil && handle.Bundle != nil {
		if _, err := s.automation.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{
			RuntimeID: handle.Bundle.RuntimeInstanceID,
		}); err != nil {
			handle.Bundle.RuntimeLogger().Error("deactivate automation runtime failed", zap.Error(err))
		}
	}
	factoryhost.StopSidecars(handle)
}
