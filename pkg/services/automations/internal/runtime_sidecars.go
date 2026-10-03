package internal

import (
	"context"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

const (
	runtimeSchedulerSourceID   = "scheduler-sidecars"
	runtimeSchedulerSourceKind = "runtime-scheduler-sidecars"
)

// StartSchedulerSidecarsForRuntime attaches runtime-scoped source supervision.
func (s *Service) StartSchedulerSidecarsForRuntime(ctx context.Context, sidecars *sync.WaitGroup,
	factoryDir string, factoryConfig *interfaces.FactoryConfig,
	runtimeConfig interfaces.RuntimeConfigLookup, submitter automations.WorkRequestSubmitter,
) error {
	if factoryConfig == nil || runtimeConfig == nil || sidecars == nil || submitter == nil {
		return nil
	}
	identity := s.schedulerSourceIdentity(factoryDir)
	configuration := s.schedulerConfiguration(identity, factoryDir, factoryConfig, runtimeConfig, submitter, s.workflowID, s.cursorScope)
	return s.startConfiguredSchedulerSource(ctx, sidecars, configuration)
}

func (s *Service) startConfiguredSchedulerSource(ctx context.Context, sidecars *sync.WaitGroup,
	configuration sourcelifecycle.RuntimeSourceConfiguration) error {
	identity := runtimeSchedulerIdentity(configuration.Snapshot.Invocation.WorkflowID, configuration.Snapshot.FactoryDir)
	if err := s.lifecycle.ConfigureRuntimeSource(ctx, configuration); err != nil {
		return err
	}
	started, err := s.reconciler.StartSourceForRuntime(ctx, configuration.RuntimeID, automations.StartSourceRequest{
		Identity: identity, Kind: runtimeSchedulerSourceKind,
	})
	if err != nil {
		return err
	}
	if _, err := s.reconciler.WaitSourceForRuntime(ctx, configuration.RuntimeID, automations.WaitSourceRequest{
		Identity: identity, Desired: automations.DesiredLifecycleRunning,
	}); err != nil {
		return err
	}
	if !started.Outcome.Idempotent {
		sidecars.Add(1)
		go s.monitorSchedulerSource(configuration.RuntimeID, identity, ctx, sidecars)
	}
	return nil
}

func (s *Service) schedulerConfiguration(identity automations.SourceIdentity, factoryDir string,
	factoryConfig *interfaces.FactoryConfig, runtimeConfig interfaces.RuntimeConfigLookup,
	submitter automations.WorkRequestSubmitter, workflowID string, scope scriptpollers.CursorScope,
) sourcelifecycle.RuntimeSourceConfiguration {
	snapshot := interfaces.RuntimeSnapshot{FactoryDir: factoryDir, RuntimeBaseDir: runtimeConfig.RuntimeBaseDir(), EffectiveFactory: *factoryConfig}
	snapshot.Invocation.WorkflowID = identity.AutomationID
	for _, ws := range factoryConfig.Workstations {
		copied := interfaces.CloneWorkstationConfig(ws)
		if selected, ok := runtimeConfig.Workstation(ws.Name); ok && selected != nil {
			copied.Limits = selected.Limits
		}
		snapshot.Workstations = append(snapshot.Workstations, copied)
	}
	names := make(map[string]bool)
	for _, worker := range factoryConfig.Workers {
		names[worker.Name] = true
	}
	for _, ws := range factoryConfig.Workstations {
		if ws.WorkerTypeName != "" {
			names[ws.WorkerTypeName] = true
		}
	}
	for name := range names {
		if selected, ok := runtimeConfig.Worker(name); ok && selected != nil {
			snapshot.Workers = append(snapshot.Workers, interfaces.CloneWorkerConfig(*selected))
		}
	}
	return sourcelifecycle.RuntimeSourceConfiguration{RuntimeID: scope.RuntimeID, CursorBaseDir: scope.BaseDir, WorkflowID: strings.TrimSpace(workflowID), Snapshot: snapshot,
		Inputs: automations.RuntimeActivationInputs{StartSchedulers: true, Submitter: submitter}}
}
func (s *Service) schedulerSourceIdentity(factoryDir string) automations.SourceIdentity {
	return runtimeSchedulerIdentity(s.workflowIdentity(factoryDir), factoryDir)
}

func runtimeSchedulerIdentity(workflowID, factoryDir string) automations.SourceIdentity {
	if strings.TrimSpace(workflowID) == "" {
		workflowID = factoryDir
	}
	return automations.SourceIdentity{
		AutomationID: strings.TrimSpace(workflowID),
		SourceID:     runtimeSchedulerSourceID,
	}
}

func (s *Service) monitorSchedulerSource(
	runtimeID string,
	identity automations.SourceIdentity,
	sourceCtx context.Context,
	sidecars *sync.WaitGroup,
) {
	defer sidecars.Done()
	stopCtx := context.WithoutCancel(sourceCtx)
	if _, err := s.lifecycle.Wait(stopCtx, sourcelifecycle.WaitEffect{
		RuntimeID: runtimeID, Desired: automations.DesiredLifecycleStopped,
		Observation: automations.SourceObservation{Identity: identity},
	}); err != nil {
		s.loggerValue.Error("join scheduler source failed", zap.Error(err))
		return
	}
	if _, err := s.reconciler.StopSourceForRuntime(stopCtx, runtimeID, automations.StopSourceRequest{
		Identity: identity,
	}); err != nil {
		s.loggerValue.Error("stop scheduler source reconciliation failed", zap.Error(err))
		return
	}
	if _, err := s.reconciler.WaitSourceForRuntime(stopCtx, runtimeID, automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleStopped,
	}); err != nil {
		s.loggerValue.Error("wait scheduler source reconciliation failed", zap.Error(err))
	}
}
