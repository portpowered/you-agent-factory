package runtimeopening

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/operatordefaults"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"go.uber.org/zap"
)

type preparedRuntime struct {
	Definition       factorydefinitions.RuntimeOpeningRequest
	Runtime          factoryruntime.RuntimeOpeningRequest
	Session          factorysessions.SessionRuntimeOpeningRequest
	Workers          workers.RuntimeOpeningRequest
	Recordings       recordings.RuntimeOpeningRequest
	Models           models.RuntimeOpeningRequest
	OperatorDefaults operatorconfig.ResolvedDefaults
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
	modelRequest models.RuntimeOpeningRequest,
	operatorDefaults operatorconfig.ResolvedDefaults,
	baseLogger *zap.Logger,
	runtimeEdges ExternalEffects,
	factoryDefinitionValidator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	loadReplay recordings.ReplayArtifactLoader,
	replayClockFactory ReplayClockFactory,
	hostedPollersFactory AutomationHostedSourcesFactory,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	resolveClock factoryruntime.ClockResolver,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	ensureOperatorBackendScope operatorconfig.BackendScopeEnsurer,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	replayFiles fileeffects.ReplayRecordingReader,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (
	prepared preparedRuntime,
	root RuntimeRoot,
	load RuntimeLoad,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	hostedPollers automations.HostedPollers,
	err error,
) {
	if err := factoryruntime.ValidateRecordReplayPaths(recordingRequest.RecordPath, recordingRequest.ReplayPath); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
	}
	if factoryScaffoldInitializer == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, fmt.Errorf(
			"Factory Definitions scaffold initializer is required",
		)
	}
	if editableFactoryValidator == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, fmt.Errorf(
			"Factory Definitions editable validator is required",
		)
	}
	prepared = preparedRuntime{
		Definition: definitionRequest, Runtime: runtimeRequest, Session: sessionRequest,
		Workers: workerRequest, Recordings: recordingRequest, Models: modelRequest,
		OperatorDefaults: operatorDefaults,
	}
	root, err = ResolveRuntimeRoot(prepared.Definition.Directory, baseLogger, prepared.Runtime.RuntimeInstanceID, generateRuntimeInstanceID, resolveHome)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
	}
	prepared.Definition.Directory = root.FactoryRootDir
	prepared.Runtime.RuntimeInstanceID = root.RuntimeInstanceID
	if err := ensureBackendScope(ensureOperatorBackendScope, &prepared.Session, prepared.Recordings.ReplayPath, root.BaseLogger); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
	}
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
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
	}
	load, err = LoadRuntime(
		selectedDefinitionPath,
		prepared.Definition.ExecutionBaseDir,
		prepared.Recordings.ReplayPath,
		prepared.OperatorDefaults,
		nil,
		root,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		loadReplay,
		captureLoadedFactorySnapshot,
		newSessionLogger,
		replayFiles,
	)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
	}
	if err := operatordefaultsruntime.ResolveConcreteProviderSelections(
		load.LoadedFactoryCfg,
		providerIdentities,
	); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, fmt.Errorf(
			"validate Factory provider selections: %w",
			err,
		)
	}
	if factoryDefinitionValidator == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, fmt.Errorf(
			"Factory Definition validator is required",
		)
	}
	if load.LoadedFactoryCfg != nil {
		result := factoryDefinitionValidator.ValidateBlockingLoad(ctx, load.LoadedFactoryCfg.FactoryConfig())
		if err := factorydefinitions.NewBlockingFactoryLoadError(result); err != nil {
			return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, err
		}
	}
	if hostedPollersFactory == nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, fmt.Errorf(
			"Automations hosted sources factory is required",
		)
	}
	selectedClock, clockErr := clockForReplay(
		runtimeEdges.Clock, load.ReplayArtifact, replayClockFactory, resolveClock,
	)
	if clockErr != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, nil, clockErr
	}
	return prepared,
		root,
		load,
		selectedClock,
		root.BaseLogger,
		hostedPollersFactory(
			root.BaseLogger,
			runtimeEdges.HostedClock,
			runtimeEdges.HostedHTTPClient,
			runtimeEdges.HostedSecretResolver,
			runtimeEdges.HostedLinearEndpoint,
		),
		nil
}

func NewDurableExecution(
	loadOperatorConfig operatorconfig.ConfigLoader,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	sessionRequest factorysessions.SessionRuntimeOpeningRequest,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride workerprovider.Provider,
	mockWorkersConfig *workers.MockWorkersConfig,
	executionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (factorysessions.ExecutionService, error) {
	if executionFactory == nil {
		return nil, fmt.Errorf("compose durable session execution: Factory Sessions execution factory is required")
	}
	projectRoot := firstNonEmpty(definitionRequest.ExecutionBaseDir, definitionRequest.Directory, root.FactoryRootDir)
	configPath, err := operatorConfigPath(sessionRequest)
	if err != nil {
		return nil, err
	}
	operatorConfig, err := loadOperatorConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	if providerIdentities == nil {
		return nil, fmt.Errorf("compose durable session worker presets: provider identity resolver is required")
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]factoryruntime.JavaScriptWorkerPreset, len(operatorConfig.WorkerPresets))
	for index, preset := range operatorConfig.WorkerPresets {
		canonicalProvider, err := providerIdentities(preset.ModelProvider)
		if err != nil {
			return nil, fmt.Errorf(
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
	defaultProvider := operatorConfig.Defaults.WorkerModelProvider
	if strings.TrimSpace(defaultProvider) != "" {
		defaultProvider, err = providerIdentities(defaultProvider)
		if err != nil {
			return nil, fmt.Errorf(
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
			DefaultModel:         operatorConfig.Defaults.WorkerModel,
		},
		mockWorkersConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("compose durable session persistence: %w", err)
	}
	return execution, nil
}

func NewWorkerExecution(
	runtimeRequest factoryruntime.RuntimeOpeningRequest,
	workerRequest workers.RuntimeOpeningRequest,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	ptyAllocator agypty.PTYAllocator,
	providerOverride workerprovider.Provider,
	state roles.CurrentRuntimeResolver,
	modelService models.Service,
	modelsScope models.RuntimeScopeRef,
	contentMaterializer work.ContentMaterializer,
	factory WorkersRuntimeFactory,
) (workers.RuntimeService, error) {
	if factory == nil {
		return nil, fmt.Errorf("Workers runtime factory is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("Factory Runtime clock is required")
	}
	now := clock.Now
	return factory(
		state,
		modelService,
		modelsScope,
		providerCommandRunner,
		scriptCommandRunner,
		ptyAllocator,
		logger,
		runtimeRequest.Verbose,
		workerRequest.RunnerID,
		workerRequest.InvocationSkipPermissionsOverride,
		providerOverride,
		now,
		contentMaterializer,
	)
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

func ensureBackendScope(ensure operatorconfig.BackendScopeEnsurer, request *factorysessions.SessionRuntimeOpeningRequest, replayPath string, logger *zap.Logger) error {
	if request == nil {
		return fmt.Errorf("Factory Session request is required to resolve backend scope")
	}
	if strings.TrimSpace(replayPath) != "" || strings.TrimSpace(request.BackendScopeID) != "" {
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
	replayClockFactory ReplayClockFactory,
	resolveClock factoryruntime.ClockResolver,
) (factoryruntime.Clock, error) {
	if clock == nil && artifact != nil && replayClockFactory != nil {
		clock = replayClockFactory(artifact)
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
