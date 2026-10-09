package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type DurableResourceOpening func(
	context.Context, string, factorydefinitions.RuntimeSelection,
	factorysessions.PersistencePolicy, string, string,
	operatorsettings.ResolvedDefaults, RuntimeRoot, factoryruntime.Clock,
	providers.Service, *workers.MockWorkersConfig,
) (DurableExecution, error)

// RuntimeResourceAcquisition retains behavior only; acquired resources and
// observations belong to the caller's opening and cleanup owner.
type RuntimeResourceAcquisition struct {
	openDurable      DurableResourceOpening
	modelService     models.Service
	providerOverride providers.Service
}

func NewRuntimeResourceAcquisition(openDurable DurableResourceOpening, modelService models.Service, providerOverride ProviderOverrideService) *RuntimeResourceAcquisition {
	return &RuntimeResourceAcquisition{openDurable: openDurable, modelService: modelService, providerOverride: providerOverride}
}

type RuntimeResourceRequest struct {
	Configured     preparedRuntime
	FactoryRootDir string
	ModelsRuntime  *models.RuntimeConfig
}

type RuntimeResourceResult struct {
	OperatorSettingsPath string
	DurableExecution     DurableExecution
	Observations         factoryruntime.SessionObservations
	ModelsScope          models.RuntimeScopeRef
}

func (acquisition *RuntimeResourceAcquisition) Acquire(
	ctx context.Context,
	request RuntimeResourceRequest,
	selectedClock factoryruntime.Clock,
	selectedLogger *zap.Logger,
	cleanup *runtimeOpeningCleanup,
) (RuntimeResourceResult, error) {
	configured := request.Configured
	selection := sessionRuntimeSelection(&configured.Session)
	path, err := operatorConfigPath(selection.SystemConfigPath, selection.SystemConfigHome)
	if err != nil {
		return RuntimeResourceResult{}, fmt.Errorf("resolve operator settings path for runtime transport: %w", err)
	}
	execution, err := acquisition.openDurable(
		ctx, configured.Session.SessionID, configured.Definition, configured.Session.Persistence,
		selection.SystemConfigHome, selection.SystemConfigPath, configured.OperatorDefaults,
		RuntimeRoot{FactoryRootDir: request.FactoryRootDir, RuntimeInstanceID: configured.Runtime.RuntimeInstanceID, BaseLogger: selectedLogger},
		selectedClock, acquisition.providerOverride, configured.Workers.MockWorkers,
	)
	if release := execution.Release; release != nil {
		cleanup.Add(func() error {
			if err := release(context.WithoutCancel(ctx)); err != nil {
				return fmt.Errorf("release durable Factory Session execution: %w", err)
			}
			return nil
		})
	}
	result := RuntimeResourceResult{OperatorSettingsPath: path, DurableExecution: execution}
	if err != nil {
		return result, err
	}
	observations, ok := execution.Service.(factoryruntime.SessionObservations)
	if !ok {
		return result, fmt.Errorf("compose runtime: durable execution owner must record mutations and publish worker progress")
	}
	result.Observations = observations
	bind, err := acquisition.bindModelsRuntimeScope(ctx, configured.ModelCacheDirectory, request.ModelsRuntime, execution.OperatorModels)
	cleanup.OwnModelsScope(context.WithoutCancel(ctx), bind)
	result.ModelsScope = bind.Scope
	return result, err
}
