package service

import (
	"context"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/workersettings"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service/operatordefaults"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type preparedRuntime struct {
	Definition                  factorydefinitions.RuntimeSelection
	DefinitionSnapshot          *factorydefinitions.RuntimeSnapshot
	Runtime                     factoryruntime.RuntimeSelection
	Session                     factorysessions.SessionStartRequest
	CanonicalSessionIDGenerated bool
	Workers                     workers.RuntimeSelection
	Recordings                  recordings.RuntimeSelection
	ModelCacheDirectory         string
	OperatorDefaults            operatorconfig.ResolvedDefaults
}

func sessionRuntimeSelection(request *factorysessions.SessionStartRequest) *factorysessions.SessionRuntimeSelection {
	if request.RuntimeSelection == nil {
		request.RuntimeSelection = &factorysessions.SessionRuntimeSelection{}
	}
	return request.RuntimeSelection
}

// RuntimePreparation retains fixed collaborators; each call carries selected facts and effects.
type RuntimePreparation struct {
	load                      func(RuntimeInputLoadRequest) (RuntimeLoad, error)
	resolveCurrentDir         factorydefinitions.CurrentFactoryDirectoryResolver
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator
	resolveHome               factorysessions.HomeDirectoryResolver
	ensureBackendScope        operatorconfig.BackendScopeEnsurer
	providerIdentities        factorysessions.ProviderIdentityResolver
	validator                 factorydefinitions.Validator
	replayClock               func(*factorydefinitions.ReplayArtifact) recordings.Clock
	resolveClock              factoryruntime.ClockResolver
}

func NewRuntimePreparation(
	load func(RuntimeInputLoadRequest) (RuntimeLoad, error),
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	ensureBackendScope operatorconfig.BackendScopeEnsurer,
	providerIdentities factorysessions.ProviderIdentityResolver,
	validator factorydefinitions.Validator,
	replayClock func(*factorydefinitions.ReplayArtifact) recordings.Clock,
	resolveClock factoryruntime.ClockResolver,
) *RuntimePreparation {
	return &RuntimePreparation{load: load, resolveCurrentDir: resolveCurrentDir,
		generateRuntimeInstanceID: generateRuntimeInstanceID, resolveHome: resolveHome,
		ensureBackendScope: ensureBackendScope, providerIdentities: providerIdentities,
		validator: validator, replayClock: replayClock, resolveClock: resolveClock}
}

func (p *RuntimePreparation) Prepare(
	ctx context.Context,
	definitionRequest factorydefinitions.RuntimeSelection,
	runtimeRequest factoryruntime.RuntimeSelection,
	sessionRequest factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	workerRequest workers.RuntimeSelection,
	recordingRequest recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	baseLogger *zap.Logger,
	selectedClock factoryruntime.Clock,
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
	prepared = preparedRuntime{
		Definition: definitionRequest, Runtime: runtimeRequest, Session: sessionRequest,
		CanonicalSessionIDGenerated: canonicalSessionIDGenerated,
		Workers:                     workerRequest, Recordings: recordingRequest, ModelCacheDirectory: modelCacheDirectory,
		OperatorDefaults: operatorDefaults, DefinitionSnapshot: definitionSnapshot,
	}
	root, err = ResolveRuntimeRoot(prepared.Definition.Directory, baseLogger, prepared.Runtime.RuntimeInstanceID, p.generateRuntimeInstanceID, p.resolveHome)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	prepared.Definition.Directory = root.FactoryRootDir
	prepared.Runtime.RuntimeInstanceID = root.RuntimeInstanceID
	selectedDefinitionPath, err := resolveDefinitionPath(
		&prepared.Definition,
		prepared.Recordings.ReplayPath,
		p.resolveCurrentDir,
		p.resolveHome,
	)
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	load, err = p.load(RuntimeInputLoadRequest{
		Dir: selectedDefinitionPath, ExecutionBaseDir: prepared.Definition.ExecutionBaseDir,
		ReplayPath: prepared.Recordings.ReplayPath, OperatorDefaults: prepared.OperatorDefaults,
		FactoryRootDir: root.FactoryRootDir, ResolvedSnapshot: prepared.DefinitionSnapshot,
		PreloadedReplayInput: replayInput, SessionID: prepared.Session.SessionID,
		HistoricalInspection: sessionRuntimeSelection(&prepared.Session).Host.Port <= 0,
	})
	if err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	if load.HistoricalReplay != nil {
		return prepared, root, load, nil, load.SessionLogger, nil
	}
	if err := ensureBackendScope(p.ensureBackendScope, &prepared.Session, root.BaseLogger); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
	}
	if err := operatordefaultsruntime.ResolveConcreteProviderSelections(
		load.LoadedFactoryCfg,
		p.providerIdentities,
	); err != nil {
		return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, fmt.Errorf(
			"validate Factory provider selections: %w",
			err,
		)
	}
	if load.LoadedFactoryCfg != nil {
		result := p.validator.ValidateBlockingLoad(ctx, load.LoadedFactoryCfg.FactoryConfig())
		if err := factorydefinitions.NewBlockingFactoryLoadError(result); err != nil {
			return preparedRuntime{}, RuntimeRoot{}, RuntimeLoad{}, nil, nil, err
		}
	}
	selectedClock, clockErr := p.clockForOpening(selectedClock, load.ReplayArtifact)
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

// DurableOpening owns the fixed collaborators used by live and replay acquisition.
// Each Open call retains only its request's settings and acquired resource owner.
type DurableOpening struct {
	loadOperatorConfig operatorconfig.ConfigLoader
	acquire            durableexecution.ScopeAcquisition
	providerReachable  bool
	providerIdentities factorysessions.ProviderIdentityResolver
}

func NewDurableOpening(
	loadOperatorConfig operatorconfig.ConfigLoader,
	acquire durableexecution.ScopeAcquisition,
	providerIdentities factorysessions.ProviderIdentityResolver,
	providerReachable bool,
) *DurableOpening {
	return &DurableOpening{loadOperatorConfig: loadOperatorConfig, acquire: acquire, providerIdentities: providerIdentities, providerReachable: providerReachable}
}

func (opening *DurableOpening) Open(
	ctx context.Context,
	sessionID string,
	definitionRequest factorydefinitions.RuntimeSelection,
	persistence factorysessions.PersistencePolicy,
	systemConfigHome string,
	systemConfigPath string,
	resolvedDefaults operatorconfig.ResolvedDefaults,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride providers.Service,
	mockWorkersConfig *workers.MockWorkersConfig,
) (DurableExecution, error) {
	projectRoot := firstNonEmpty(definitionRequest.ExecutionBaseDir, definitionRequest.Directory, root.FactoryRootDir)
	configPath, err := operatorConfigPath(systemConfigPath, systemConfigHome)
	if err != nil {
		return DurableExecution{}, err
	}
	operatorConfig, err := opening.loadOperatorConfig(configPath)
	if err != nil {
		return DurableExecution{}, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	workerPresetIDs, workerSettings, err := opening.resolveWorkerSettings(operatorConfig, resolvedDefaults)
	if err != nil {
		return DurableExecution{}, err
	}
	mode := factorysessions.ChildExecutorModeFake
	mockAllowsLive := mockWorkersConfig == nil || mockWorkersConfig.UnmatchedDispatchPolicy.PassthroughUnmatched()
	if providerOverride != nil || (mockAllowsLive && opening.providerReachable) {
		mode = factorysessions.ChildExecutorModeLive
	}
	execution, release, err := opening.acquire(ctx, durableexecution.ScopeFacts{
		FactorySessionID: sessionID, RuntimeID: root.RuntimeInstanceID,
		ProjectRoot: projectRoot, Persistence: persistence, ChildExecutorMode: mode,
		WorkerPresetIDs: workerPresetIDs, WorkerSettings: workerSettings,
	}, clock, root.BaseLogger)
	if err != nil {
		// The opener can acquire an owner before failing. Sessions registers
		// its cleanup before checking this error, so preserve that partial
		// ownership without publishing successful execution settings.
		return DurableExecution{Release: release}, fmt.Errorf("compose durable session persistence: %w", err)
	}
	return DurableExecution{
		Service:         execution,
		Release:         release,
		WorkerSettings:  workersettings.Clone(&workerSettings),
		ACPIntegrations: append([]operatorconfig.ACPIntegration(nil), operatorConfig.Workers.ACP.Integrations...),
		OperatorModels:  projectOperatorModelOverlays(operatorConfig.Models),
	}, nil
}

func (opening *DurableOpening) resolveWorkerSettings(
	operatorConfig operatorconfig.Config,
	resolvedDefaults operatorconfig.ResolvedDefaults,
) (map[string]struct{}, factoryruntime.JavaScriptWorkerSettings, error) {
	if opening.providerIdentities == nil {
		return nil, factoryruntime.JavaScriptWorkerSettings{}, fmt.Errorf("compose durable session worker presets: provider identity resolver is required")
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]factoryruntime.JavaScriptWorkerPreset, len(operatorConfig.WorkerPresets))
	for index, preset := range operatorConfig.WorkerPresets {
		canonicalProvider, err := opening.providerIdentities(preset.ModelProvider)
		if err != nil {
			return nil, factoryruntime.JavaScriptWorkerSettings{}, fmt.Errorf(
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
		canonicalProvider, err := opening.providerIdentities(defaultProvider)
		if err != nil {
			return nil, factoryruntime.JavaScriptWorkerSettings{}, fmt.Errorf(
				"compose durable session worker presets: defaults.workerModelProvider: %w",
				err,
			)
		}
		defaultProvider = canonicalProvider
	}
	return workerPresetIDs, factoryruntime.JavaScriptWorkerSettings{
		Presets: workerPresets, DefaultModelProvider: defaultProvider,
		DefaultModel: firstNonEmpty(resolvedDefaults.WorkerModel, operatorConfig.Defaults.WorkerModel),
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
	definition *factorydefinitions.RuntimeSelection,
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

func operatorConfigPath(configPath, home string) (string, error) {
	if strings.TrimSpace(configPath) != "" {
		return strings.TrimSpace(configPath), nil
	}
	homeDir := strings.TrimSpace(home)
	if homeDir == "" {
		return "", fmt.Errorf("operator config home is required")
	}
	return operatorconfig.DefaultConfigPath(homeDir), nil
}

func ensureBackendScope(ensure operatorconfig.BackendScopeEnsurer, request *factorysessions.SessionStartRequest, logger *zap.Logger) error {
	if request == nil {
		return fmt.Errorf("Factory Session request is required to resolve backend scope")
	}
	selection := sessionRuntimeSelection(request)
	if strings.TrimSpace(selection.BackendScopeID) != "" {
		return nil
	}
	if ensure == nil {
		return fmt.Errorf("Operator Settings backend-scope ensurer is required")
	}
	configPath, err := operatorConfigPath(selection.SystemConfigPath, selection.SystemConfigHome)
	if err != nil {
		return err
	}
	resolved, err := ensure(configPath)
	if err != nil {
		return err
	}
	selection.BackendScopeID = resolved.BackendScopeID
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

// Live opening consumes the clock required by construction. Replay retains its
// compatibility selection when no explicit clock was supplied.
func (p *RuntimePreparation) clockForOpening(selectedClock factoryruntime.Clock, artifact *factorydefinitions.ReplayArtifact) (factoryruntime.Clock, error) {
	if artifact == nil {
		return selectedClock, nil
	}
	return clockForReplay(selectedClock, artifact, p.replayClock, p.resolveClock)
}

func clockForReplay(
	clock factoryruntime.Clock,
	artifact *factorydefinitions.ReplayArtifact,
	replayClock func(*factorydefinitions.ReplayArtifact) recordings.Clock,
	resolveClock factoryruntime.ClockResolver,
) (factoryruntime.Clock, error) {
	if clock != nil {
		return clock, nil
	}
	if artifact != nil && replayClock != nil {
		clock = replayClock(artifact)
	}
	if clock != nil {
		return clock, nil
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
