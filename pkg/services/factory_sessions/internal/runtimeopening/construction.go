package runtimeopening

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	"go.uber.org/zap"
)

type preparedRuntime struct {
	Definition          factorydefinitions.RuntimeOpeningRequest
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
		Workers: workerRequest, Recordings: recordingRequest, ModelCacheDirectory: modelCacheDirectory,
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
	resolvedDefaults operatorconfig.ResolvedDefaults,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride workers.Provider,
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
	}, nil
}

// NewWorkerExecution constructs the Factory Session's own Workers runtime and
// returns a SessionBuildFactory closure that reaches the same canonical
// Workers wire construction boundary for every later Factory Session build,
// with only the final per-build provider command runner, script command
// runner, and progress publisher supplied explicitly. A nil argument to the
// returned SessionBuildFactory preserves the resolved value used to
// construct this Factory Session's own runtime -- the exact instance
// resolved here, not read back off an already-built RuntimeService. Each
// runtime the SessionBuildFactory constructs is independent and is reported
// through registerSessionBuildRuntime so its caller can fold its lifecycle
// into the Factory Session's own cleanup. registerSessionBuildRuntime reports
// whether it accepted the registration; when it returns false (the Factory
// Session's cleanup has already run), the SessionBuildFactory closes the
// freshly constructed runtime itself instead of returning one nothing will
// ever close.
func NewWorkerExecution(
	runtimeRequest factoryruntime.RuntimeOpeningRequest,
	workerRequest workers.RuntimeOpeningRequest,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	ptyAllocator workers.PTYAllocator,
	providerOverride workers.Provider,
	state roles.CurrentRuntimeResolver,
	modelService models.Service,
	modelsScope models.RuntimeScopeRef,
	workService work.Service,
	factory WorkersRuntimeFactory,
	acpIntegrations []operatorconfig.ACPIntegration,
	registerSessionBuildRuntime func(workers.RuntimeService) bool,
) (workers.RuntimeService, workers.SessionBuildFactory, error) {
	if factory == nil {
		return nil, nil, fmt.Errorf("Workers runtime factory is required")
	}
	if clock == nil {
		return nil, nil, fmt.Errorf("Factory Runtime clock is required")
	}
	if workService == nil {
		return nil, nil, fmt.Errorf("Work service is required")
	}
	fixed := sessionBuildFixedRuntimeInputs{
		state:                   state,
		modelService:            modelService,
		modelsScope:             modelsScope,
		providerCommandRunner:   providerCommandRunner,
		scriptCommandRunner:     scriptCommandRunner,
		progressPublisher:       progressPublisher,
		ptyAllocator:            ptyAllocator,
		logger:                  logger,
		verbose:                 runtimeRequest.Verbose,
		runnerID:                workerRequest.RunnerID,
		worktree:                workerRequest.Worktree,
		skipPermissionsOverride: workerRequest.InvocationSkipPermissionsOverride,
		providerOverride:        providerOverride,
		now:                     clock.Now,
		contentMaterializer:     work.ContentMaterializeFunc(workService.MaterializeContentURL),
		acpIntegrations:         append([]operatorconfig.ACPIntegration(nil), acpIntegrations...),
	}
	base, err := factory(
		fixed.state,
		fixed.modelService,
		fixed.modelsScope,
		fixed.providerCommandRunner,
		fixed.scriptCommandRunner,
		fixed.progressPublisher,
		fixed.ptyAllocator,
		fixed.logger,
		fixed.verbose,
		fixed.runnerID,
		fixed.worktree,
		fixed.skipPermissionsOverride,
		fixed.providerOverride,
		fixed.now,
		fixed.contentMaterializer,
		fixed.acpIntegrations,
	)
	if err != nil {
		return nil, nil, err
	}
	return base, newSessionBuildFactory(fixed, factory, registerSessionBuildRuntime), nil
}

// sessionBuildFixedRuntimeInputs carries the resolved construction inputs a
// SessionBuildFactory closure reuses, unchanged, for every later Factory
// Session build through the same canonical Workers wire construction
// boundary -- the exact values NewWorkerExecution itself resolved, not
// values read back off an already-built RuntimeService.
type sessionBuildFixedRuntimeInputs struct {
	state                   roles.CurrentRuntimeResolver
	modelService            models.Service
	modelsScope             models.RuntimeScopeRef
	providerCommandRunner   workers.CommandRunner
	scriptCommandRunner     workers.CommandRunner
	progressPublisher       workers.ProgressPublisher
	ptyAllocator            workers.PTYAllocator
	logger                  *zap.Logger
	verbose                 bool
	runnerID                string
	worktree                string
	skipPermissionsOverride *bool
	providerOverride        workers.Provider
	now                     func() time.Time
	contentMaterializer     work.ContentMaterializer
	acpIntegrations         []operatorconfig.ACPIntegration
}

// newSessionBuildFactory returns the SessionBuildFactory closure
// NewWorkerExecution hands to its caller. A nil per-build argument preserves
// the matching fixed input instead of falling back to a zero value.
func newSessionBuildFactory(
	fixed sessionBuildFixedRuntimeInputs,
	factory WorkersRuntimeFactory,
	registerSessionBuildRuntime func(workers.RuntimeService) bool,
) workers.SessionBuildFactory {
	return func(
		buildProviderCommandRunner workers.CommandRunner,
		buildScriptCommandRunner workers.CommandRunner,
		buildProgressPublisher workers.ProgressPublisher,
	) (workers.RuntimeService, error) {
		resolvedProviderCommandRunner := buildProviderCommandRunner
		if resolvedProviderCommandRunner == nil {
			resolvedProviderCommandRunner = fixed.providerCommandRunner
		}
		resolvedScriptCommandRunner := buildScriptCommandRunner
		if resolvedScriptCommandRunner == nil {
			resolvedScriptCommandRunner = fixed.scriptCommandRunner
		}
		resolvedProgressPublisher := buildProgressPublisher
		if resolvedProgressPublisher == nil {
			resolvedProgressPublisher = fixed.progressPublisher
		}
		built, err := factory(
			fixed.state,
			fixed.modelService,
			fixed.modelsScope,
			resolvedProviderCommandRunner,
			resolvedScriptCommandRunner,
			resolvedProgressPublisher,
			fixed.ptyAllocator,
			fixed.logger,
			fixed.verbose,
			fixed.runnerID,
			fixed.worktree,
			fixed.skipPermissionsOverride,
			fixed.providerOverride,
			fixed.now,
			fixed.contentMaterializer,
			fixed.acpIntegrations,
		)
		if err != nil {
			return nil, err
		}
		if registerSessionBuildRuntime != nil && !registerSessionBuildRuntime(built) {
			closeErr := built.Close(context.Background())
			return nil, errors.Join(
				fmt.Errorf("construct Worker runtime services: Factory Session is already closed"),
				closeErr,
			)
		}
		return built, nil
	}
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
