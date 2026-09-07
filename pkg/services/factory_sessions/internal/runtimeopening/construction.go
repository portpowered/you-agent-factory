package runtimeopening

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/operatordefaults"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type preparedRuntime struct {
	Definition          factorydefinitions.RuntimeOpeningRequest
	DefinitionSnapshot  *factorydefinitions.RuntimeSnapshot
	Runtime             factoryruntime.RuntimeOpeningRequest
	Session             factorysessions.SessionRuntimeOpeningRequest
	Workers             workers.RuntimeOpeningRequest
	Recordings          recordings.RuntimeOpeningRequest
	ModelCacheDirectory string
	OperatorDefaults    operatorconfig.ResolvedDefaults
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func PrepareRuntime(
	ctx context.Context,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	runtimeRequest factoryruntime.RuntimeOpeningRequest,
	sessionRequest factorysessions.SessionRuntimeOpeningRequest,
	workerRequest workers.RuntimeOpeningRequest,
	recordingRequest recordings.RuntimeOpeningRequest,
	modelCacheDirectory string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	baseLogger *zap.Logger,
	clockEdge factoryruntime.Clock,
	factoryDefinitionValidator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	replayClock func(*factorydefinitions.ReplayArtifact) recordings.Clock,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	resolveClock factoryruntime.ClockResolver,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	ensureOperatorBackendScope operatorconfig.BackendScopeEnsurer,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	providerIdentities factorysessions.ProviderIdentityResolver,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (
	prepared preparedRuntime,
	root RuntimeRoot,
	load RuntimeLoad,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	err error,
) {
	if err := factoryruntime.ValidateRecordReplayPaths(recordingRequest.RecordPath, recordingRequest.ReplayPath); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	if factoryScaffoldInitializer == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, fmt.Errorf(
			"Factory Definitions scaffold initializer is required",
		)
	}
	if editableFactoryValidator == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, fmt.Errorf(
			"Factory Definitions editable validator is required",
		)
	}
	prepared = preparedRuntime{
		Definition: definitionRequest, Runtime: runtimeRequest, Session: sessionRequest,
		Workers: workerRequest, Recordings: recordingRequest, ModelCacheDirectory: modelCacheDirectory,
		OperatorDefaults: operatorDefaults, DefinitionSnapshot: definitionSnapshot,
	}
	root, err = ResolveRuntimeRoot(prepared.Definition.Directory, baseLogger, prepared.Runtime.RuntimeInstanceID, generateRuntimeInstanceID, resolveHome)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	prepared.Definition.Directory = root.FactoryRootDir
	prepared.Runtime.RuntimeInstanceID = root.RuntimeInstanceID
	var resolveCurrentDir func(string) (string, error)
	if namedPaths != nil {
		resolveCurrentDir = namedPaths.ResolveCurrentDir
	}
	selectedDefinitionPath, err := resolveDefinitionPath(
		&prepared.Definition,
		prepared.Recordings.ReplayPath,
		resolveCurrentDir,
		resolveHome,
	)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	load, err = loadRuntime(
		selectedDefinitionPath,
		prepared.Definition.ExecutionBaseDir,
		prepared.Recordings.ReplayPath,
		prepared.OperatorDefaults,
		nil,
		root,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		replayInputs,
		captureLoadedFactorySnapshot,
		newSessionLogger,
		prepared.DefinitionSnapshot,
		replayInput,
		prepared.Session.FactorySessionID,
	)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	if load.HistoricalReplay != nil {
		return prepared, root, load, nil, load.SessionLogger, nil
	}
	if err := ensureBackendScope(ensureOperatorBackendScope, &prepared.Session, root.BaseLogger); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	if err := operatordefaultsruntime.ResolveConcreteProviderSelections(
		load.LoadedFactoryCfg,
		providerIdentities,
	); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, fmt.Errorf(
			"validate Factory provider selections: %w",
			err,
		)
	}
	if factoryDefinitionValidator == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, fmt.Errorf(
			"Factory Definition validator is required",
		)
	}
	if load.LoadedFactoryCfg != nil {
		result := factoryDefinitionValidator.ValidateBlockingLoad(ctx, load.LoadedFactoryCfg.FactoryConfig())
		if err := factorydefinitions.NewBlockingFactoryLoadError(result); err != nil {
			return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
		}
	}
	selectedClock, clockErr := clockForReplay(
		clockEdge, load.ReplayArtifact, replayClock, resolveClock,
	)
	if clockErr != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, clockErr
	}
	return prepared,
		root,
		load,
		selectedClock,
		root.BaseLogger,
		nil
}

func NewDurableExecution(
	loadOperatorConfig operatorconfig.ConfigLoader,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	sessionRequest factorysessions.SessionRuntimeOpeningRequest,
	resolvedDefaults operatorconfig.ResolvedDefaults,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride providers.Service,
	mockWorkersConfig *workers.MockWorkersConfig,
	executionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (DurableExecution, error) {
	if executionFactory == nil {
		return DurableExecution{}, fmt.Errorf("compose durable session execution: Factory Sessions execution factory is required")
	}
	projectRoot := firstNonEmpty(definitionRequest.ExecutionBaseDir, definitionRequest.Directory, root.FactoryRootDir)
	configPath, err := operatorConfigPath(sessionRequest)
	if err != nil {
		return DurableExecution{}, err
	}
	operatorConfig, err := loadOperatorConfig(configPath)
	if err != nil {
		return DurableExecution{}, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	if providerIdentities == nil {
		return DurableExecution{}, fmt.Errorf("compose durable session worker presets: provider identity resolver is required")
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]factoryruntime.JavaScriptWorkerPreset, len(operatorConfig.WorkerPresets))
	for index, preset := range operatorConfig.WorkerPresets {
		canonicalProvider, err := providerIdentities(preset.ModelProvider)
		if err != nil {
			return DurableExecution{}, fmt.Errorf(
				"compose durable session worker presets: workerPresets[%d].modelProvider: %w",
				index,
				err,
			)
		}
		workerPresetIDs[preset.ID] = struct{}{}
		workerPresets[preset.ID] = factoryruntime.JavaScriptWorkerPreset{
			ModelProvider:   canonicalProvider,
			Model:           preset.Model,
			ReasoningEffort: preset.ReasoningEffort,
		}
	}
	defaultProvider := firstNonEmpty(resolvedDefaults.WorkerModelProvider, operatorConfig.Defaults.WorkerModelProvider)
	if strings.TrimSpace(defaultProvider) != "" {
		defaultProvider, err = providerIdentities(defaultProvider)
		if err != nil {
			return DurableExecution{}, fmt.Errorf(
				"compose durable session worker presets: defaults.workerModelProvider: %w",
				err,
			)
		}
	}
	execution, err := executionFactory(
		projectRoot,
		sessionRequest.PersistencePolicy,
		providerOverride,
		clock,
		workerPresetIDs,
		factoryruntime.JavaScriptWorkerSettings{
			Presets:              workerPresets,
			DefaultModelProvider: defaultProvider,
			DefaultModel:         firstNonEmpty(resolvedDefaults.WorkerModel, operatorConfig.Defaults.WorkerModel),
		},
		mockWorkersConfig,
		append([]operatorconfig.ACPIntegration(nil), operatorConfig.Workers.ACP.Integrations...),
	)
	if err != nil {
		return DurableExecution{}, fmt.Errorf("compose durable session persistence: %w", err)
	}
	return DurableExecution{
		Service:         execution,
		ACPIntegrations: append([]operatorconfig.ACPIntegration(nil), operatorConfig.Workers.ACP.Integrations...),
		OperatorModels:  projectOperatorModelOverlays(operatorConfig.Models),
	}, nil
}

// projectOperatorModelOverlays snapshots the operator-settings model map at
// the same durable opening boundary as worker presets. Models owns the
// resulting representation; this package only translates the settings
// boundary without retaining pointers into the decoded operator document.
func projectOperatorModelOverlays(
	configured map[string]operatorconfig.ModelConfig,
) map[string]models.ModelOverlay {
	if len(configured) == 0 {
		return nil
	}
	projected := make(map[string]models.ModelOverlay, len(configured))
	for name, config := range configured {
		overlay := models.ModelOverlay{
			Source:     cloneStringPointer(config.Source),
			Backend:    cloneStringPointer(config.Backend),
			Operations: append([]string(nil), config.Operations...),
		}
		if config.LoadPolicy != nil {
			policy := models.LoadPolicy(*config.LoadPolicy)
			overlay.LoadPolicy = &policy
		}
		projected[name] = overlay
	}
	return projected
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func resolveDefinitionPath(
	definition *factorydefinitions.RuntimeOpeningRequest,
	replayPath string,
	resolveCurrentDir func(string) (string, error),
	resolveHome factorysessions.HomeDirectoryResolver,
) (string, error) {
	if replayPath != "" {
		return definition.Directory, nil
	}
	if definition.SourcePath != "" {
		resolved, err := logicaltarget.AbsolutizeFactoryDirectory(definition.SourcePath, resolveHome)
		if err != nil {
			return "", fmt.Errorf("resolve factory source: %w", err)
		}
		return resolved, nil
	}
	if resolveCurrentDir == nil {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	resolvedDir, err := resolveCurrentDir(definition.Directory)
	if err != nil {
		return "", fmt.Errorf("resolve factory dir: %w", err)
	}
	definition.Directory, err = logicaltarget.AbsolutizeFactoryDirectory(resolvedDir, resolveHome)
	if err != nil {
		return "", fmt.Errorf("resolve factory dir: %w", err)
	}
	return definition.Directory, nil
}

func operatorConfigPath(request factorysessions.SessionRuntimeOpeningRequest) (string, error) {
	if strings.TrimSpace(request.SystemConfigPath) != "" {
		return strings.TrimSpace(request.SystemConfigPath), nil
	}
	homeDir := strings.TrimSpace(request.SystemConfigHome)
	if homeDir == "" {
		return "", fmt.Errorf("operator config home is required")
	}
	return operatorconfig.DefaultConfigPath(homeDir), nil
}

func ensureBackendScope(ensure operatorconfig.BackendScopeEnsurer, request *factorysessions.SessionRuntimeOpeningRequest, logger *zap.Logger) error {
	if request == nil {
		return fmt.Errorf("Factory Session request is required to resolve backend scope")
	}
	if strings.TrimSpace(request.BackendScopeID) != "" {
		return nil
	}
	if ensure == nil {
		return fmt.Errorf("Operator Settings backend-scope ensurer is required")
	}
	configPath, err := operatorConfigPath(*request)
	if err != nil {
		return err
	}
	resolved, err := ensure(configPath)
	if err != nil {
		return err
	}
	request.BackendScopeID = resolved.BackendScopeID
	if logger != nil {
		logger.Info("resolved backend scope for local backend", zap.String("diagnostics", resolved.DiagnosticsLine()))
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func clockForReplay(
	clock factoryruntime.Clock,
	artifact *factorydefinitions.ReplayArtifact,
	replayClock func(*factorydefinitions.ReplayArtifact) recordings.Clock,
	resolveClock factoryruntime.ClockResolver,
) (factoryruntime.Clock, error) {
	if clock == nil && artifact != nil && replayClock != nil {
		clock = replayClock(artifact)
	}
	if resolveClock == nil {
		return nil, fmt.Errorf("Factory Runtime clock resolver is required")
	}
	clock = resolveClock(clock)
	if clock == nil {
		return nil, fmt.Errorf("Factory Runtime clock resolver returned nil")
	}
	return clock, nil
}
