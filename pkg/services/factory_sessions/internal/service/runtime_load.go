package service

import (
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service/operatordefaults"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

// RuntimeLoad contains the immutable Factory Runtime inputs selected while
// opening a Factory Session.
type RuntimeLoad struct {
	LoadedFactoryCfg       factorydefinitions.MutableLoadedFactorySource
	ReplayArtifact         *factorydefinitions.ReplayArtifact
	PortableRecording      *recording.PortableRecording
	HistoricalReplay       *recordingreplay.RecordingReplayProjection
	ReplayMetadataWarnings []recording.MetadataMismatchWarning
	SessionLogger          *zap.Logger
}

type invocationSensitiveLoadedFactory struct {
	factorydefinitions.MutableLoadedFactorySource
	pointers         []string
	promptProvenance []factorydefinitions.RuntimePromptProvenance
}

func (source *invocationSensitiveLoadedFactory) WorkerPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkerPromptSource(name)
}

func (source *invocationSensitiveLoadedFactory) WorkstationPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkstationPromptSource(name)
}

func (source *invocationSensitiveLoadedFactory) InvocationSensitiveJSONPointers() []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source.pointers...)
}

func (source *invocationSensitiveLoadedFactory) WorkerPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, true)
}

func (source *invocationSensitiveLoadedFactory) WorkstationPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, false)
}

type invocationSensitiveSpanLoadedFactory struct {
	factorydefinitions.MutableLoadedFactorySource
	spans            []factorydefinitions.InvocationSensitiveJSONSpan
	promptProvenance []factorydefinitions.RuntimePromptProvenance
}

func (source *invocationSensitiveSpanLoadedFactory) WorkerPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkerPromptSource(name)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkstationPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkstationPromptSource(name)
}

func (source *invocationSensitiveSpanLoadedFactory) InvocationSensitiveJSONSpans() []factorydefinitions.InvocationSensitiveJSONSpan {
	if source == nil {
		return nil
	}
	return append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), source.spans...)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkerPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, true)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkstationPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, false)
}

func lookupRuntimePromptProvenance(
	source factorydefinitions.MutableLoadedFactorySource,
	provenance []factorydefinitions.RuntimePromptProvenance,
	name string,
	worker bool,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	for _, candidate := range provenance {
		if candidate.Name == name {
			return candidate, true
		}
	}
	lookup, ok := source.(factorydefinitions.RuntimePromptProvenanceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.RuntimePromptProvenance{}, false
	}
	if worker {
		return lookup.WorkerPromptProvenance(name)
	}
	return lookup.WorkstationPromptProvenance(name)
}

type runtimeReplayLoad struct {
	legacyArtifact    *factorydefinitions.ReplayArtifact
	portableRecording *recording.PortableRecording
	historicalReplay  *recordingreplay.RecordingReplayProjection
}

// RuntimeInputLoadRequest carries only selected invocation facts and detached artifacts.
type RuntimeInputLoadRequest struct {
	Dir, ExecutionBaseDir, ReplayPath, FactoryRootDir, SessionID string
	OperatorDefaults                                             operatorconfig.ResolvedDefaults
	ResolvedSnapshot                                             *factorydefinitions.RuntimeSnapshot
	PreloadedReplayInput                                         *recording.LoadReplayInputResult
	HistoricalInspection                                         bool
}

// RuntimeInputLoading owns the fixed loading behavior shared by session openings.
// Selected inputs and mutable configuration remain local to each Load call.
type RuntimeInputLoading struct {
	loadFactory                  factorydefinitions.LoadedFactoryLoader
	newLoadedFactory             factorydefinitions.LoadedFactorySourceFactory
	decodeReplayConfig           factorydefinitions.ReplayRuntimeConfigDecoder
	replayInputs                 recording.ReplayInputLoader
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer
	newSessionLogger             factoryruntime.SessionLoggerFactory
	baseLogger                   *zap.Logger
}

func NewRuntimeInputLoading(
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recording.ReplayInputLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	baseLogger *zap.Logger,
) *RuntimeInputLoading {
	return &RuntimeInputLoading{
		loadFactory: loadFactory, newLoadedFactory: newLoadedFactory,
		decodeReplayConfig: decodeReplayConfig, replayInputs: replayInputs,
		captureLoadedFactorySnapshot: captureLoadedFactorySnapshot,
		newSessionLogger:             newSessionLogger, baseLogger: baseLogger,
	}
}

func (loader *RuntimeInputLoading) Load(request RuntimeInputLoadRequest) (RuntimeLoad, error) {
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	logger := loader.newSessionLogger(loader.baseLogger, sessionID, request.FactoryRootDir, request.Dir)
	if logger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory returned nil")
	}
	replayLoad, err := loader.loadRuntimeReplay(request.ReplayPath, request.PreloadedReplayInput, sessionID, request.HistoricalInspection)
	if err != nil {
		return RuntimeLoad{}, err
	}
	if replayLoad.historicalReplay != nil {
		return loader.loadHistoricalRuntime(request.Dir, request.ReplayPath, request.OperatorDefaults, replayLoad, logger)
	}
	return loader.loadConfiguredRuntime(request.Dir, request.ExecutionBaseDir, request.ReplayPath,
		request.OperatorDefaults, request.ResolvedSnapshot, replayLoad, logger)
}

func (loader *RuntimeInputLoading) loadHistoricalRuntime(
	dir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	replayLoad runtimeReplayLoad,
	logger *zap.Logger,
) (RuntimeLoad, error) {
	replayMetadataWarnings := []recording.MetadataMismatchWarning(nil)
	if replayLoad.legacyArtifact != nil {
		replayMetadataWarnings = loader.warnReplayMetadataMismatches(
			dir,
			replayPath,
			replayLoad.legacyArtifact,
			logger,
			operatorDefaults,
		)
	}
	return RuntimeLoad{
		ReplayArtifact:         replayLoad.legacyArtifact,
		PortableRecording:      replayLoad.portableRecording,
		HistoricalReplay:       replayLoad.historicalReplay,
		ReplayMetadataWarnings: replayMetadataWarnings,
		SessionLogger:          logger,
	}, nil
}

func (loader *RuntimeInputLoading) loadConfiguredRuntime(
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	resolvedSnapshot *factorydefinitions.RuntimeSnapshot,
	replayLoad runtimeReplayLoad,
	logger *zap.Logger,
) (RuntimeLoad, error) {
	logger.Info("loading factory config", zap.String("dir", dir))
	loaded, artifact, err := loader.loadRuntimeConfig(
		dir,
		executionBaseDir,
		replayPath,
		operatorDefaults,
		replayLoad.legacyArtifact,
		resolvedSnapshot,
	)
	if err != nil {
		logger.Error("failed to load factory config", zap.Error(err))
		return RuntimeLoad{}, fmt.Errorf("load factory config: %w", err)
	}
	if loaded != nil {
		factoryruntime.WarnPortableBundledReplacementReport(
			logger,
			"runtime config load replaced portable bundled files",
			loaded.PortableBundledFileReplacements(),
		)
		if config := loaded.FactoryConfig(); config != nil {
			if paths := config.IgnoredJSONPaths(); len(paths) > 0 {
				logger.Warn(
					"ignored unknown Factory Definition fields",
					zap.String("code", factorydefinitions.FactoryConfigIgnoredFieldWarningCode),
					zap.Strings("ignored_json_paths", paths),
				)
			}
		}
	}
	replayMetadataWarnings := loader.warnReplayMetadataMismatches(
		dir,
		replayPath,
		artifact,
		logger,
		operatorDefaults,
	)
	return RuntimeLoad{
		LoadedFactoryCfg:       loaded,
		ReplayArtifact:         artifact,
		ReplayMetadataWarnings: replayMetadataWarnings,
		SessionLogger:          logger,
	}, nil
}

func (loader *RuntimeInputLoading) loadRuntimeReplay(
	replayPath string,
	preloadedReplayInput *recording.LoadReplayInputResult,
	sessionID string,
	historicalInspection bool,
) (runtimeReplayLoad, error) {
	if replayPath == "" {
		return runtimeReplayLoad{}, nil
	}
	var result recording.LoadReplayInputResult
	var err error
	if preloadedReplayInput != nil {
		result = *preloadedReplayInput
	} else {
		result, err = loader.replayInputs.LoadReplayInput(recording.LoadReplayInputRequest{Path: replayPath})
	}
	if err != nil {
		var inputErr *recording.ReplayInputError
		if errors.As(err, &inputErr) && inputErr.Family == recording.ReplayInputFamilyLegacy {
			return runtimeReplayLoad{}, fmt.Errorf("load factory config: %w", err)
		}
		return runtimeReplayLoad{}, fmt.Errorf("load portable replay: %w", err)
	}
	if result.Portable == nil {
		if result.Legacy == nil {
			return runtimeReplayLoad{}, fmt.Errorf("load legacy replay: replay artifact is required")
		}
		if !historicalInspection {
			return runtimeReplayLoad{legacyArtifact: result.Legacy}, nil
		}
		if !selectsHistoricalReplayInspection(result) {
			return runtimeReplayLoad{legacyArtifact: result.Legacy}, nil
		}
		return loader.loadHistoricalLegacyReplay(result.Legacy, sessionID)
	}
	projection, err := recordingreplay.ReplayRecording(*result.Portable)
	if err != nil {
		return runtimeReplayLoad{}, fmt.Errorf("load portable replay: inspect historical recording: %w", err)
	}
	return runtimeReplayLoad{
		portableRecording: result.Portable,
		historicalReplay:  &projection,
	}, nil
}

func (loader *RuntimeInputLoading) loadHistoricalLegacyReplay(
	artifact *recording.ReplayArtifact,
	sessionID string,
) (runtimeReplayLoad, error) {
	reconstructor, ok := loader.replayInputs.(interface {
		ReconstructCanonicalFactoryWorldState([]recording.FactoryEvent, int) (recording.FactoryWorldState, error)
	})
	if !ok {
		return runtimeReplayLoad{}, fmt.Errorf("load legacy replay: canonical Factory projection capability is required")
	}
	selectedTick := legacyReplaySelectedTick(artifact.Events)
	state, err := reconstructor.ReconstructCanonicalFactoryWorldState(artifact.Events, selectedTick)
	if err != nil {
		return runtimeReplayLoad{}, fmt.Errorf("load legacy replay: reconstruct Factory projection: %w", err)
	}
	projection, err := recordingreplay.ReplayLegacyRecording(*artifact, sessionID, state)
	if err != nil {
		return runtimeReplayLoad{}, fmt.Errorf("load legacy replay: inspect historical recording: %w", err)
	}
	return runtimeReplayLoad{
		legacyArtifact:   artifact,
		historicalReplay: &projection,
	}, nil
}

func legacyReplaySelectedTick(events []recording.FactoryEvent) int {
	selected := 0
	for _, event := range events {
		if event.Context.Tick > selected {
			selected = event.Context.Tick
		}
	}
	return selected
}

func (loader *RuntimeInputLoading) loadRuntimeConfig(
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	artifact *factorydefinitions.ReplayArtifact,
	resolvedSnapshot *factorydefinitions.RuntimeSnapshot,
) (factorydefinitions.MutableLoadedFactorySource, *factorydefinitions.ReplayArtifact, error) {
	if replayPath == "" {
		if resolvedSnapshot != nil {
			loaded, err := loader.loadRuntimeSnapshot(
				resolvedSnapshot,
				executionBaseDir,
				operatorDefaults,
			)
			return loaded, nil, err
		}
		loaded, err := loader.loadFactory(dir, nil)
		if loaded != nil {
			loaded.SetRuntimeBaseDir(executionBaseDir)
		}
		if err != nil {
			return loaded, nil, err
		}
		if err := applyOperatorDefaults(loaded, operatorDefaults); err != nil {
			return nil, nil, err
		}
		return loaded, nil, nil
	}
	if artifact == nil {
		return nil, nil, fmt.Errorf("replay artifact is required")
	}
	runtimeConfig, err := loader.decodeReplayConfig(artifact.Factory)
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded replay config: %w", err)
	}
	loaded, err := loader.newLoadedFactory(
		runtimeConfig.FactoryDir(),
		runtimeConfig.FactoryConfig(),
		runtimeConfig,
		nil,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build embedded replay config: %w", err)
	}
	loaded.SetRuntimeBaseDir(executionBaseDir)
	return loaded, artifact, nil
}

func (loader *RuntimeInputLoading) loadRuntimeSnapshot(
	resolved *factorydefinitions.RuntimeSnapshot,
	executionBaseDir string,
	operatorDefaults operatorconfig.ResolvedDefaults,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved Factory Definition snapshot is required")
	}
	snapshot, err := resolved.Clone()
	if err != nil {
		return nil, fmt.Errorf("clone resolved Factory Definition snapshot: %w", err)
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		return nil, fmt.Errorf("resolved Factory Definition snapshot directory is required")
	}
	if strings.TrimSpace(snapshot.EffectiveFactory.Name) == "" {
		return nil, fmt.Errorf("resolved Factory Definition snapshot Factory name is required")
	}
	lookup := newRuntimeSnapshotLookup(&snapshot)
	config := snapshot.EffectiveFactory
	attachRuntimeSnapshotPromptSources(&config, snapshot.PromptSources)
	loaded, err := loader.newLoadedFactory(
		snapshot.FactoryDir,
		&config,
		lookup,
		snapshot.BundledFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("build loaded Factory from resolved snapshot: %w", err)
	}
	if loaded == nil {
		return nil, fmt.Errorf("Factory Definitions loaded-source factory returned no source")
	}
	baseDir := strings.TrimSpace(executionBaseDir)
	if baseDir == "" {
		baseDir = snapshot.RuntimeBaseDir
	}
	loaded.SetRuntimeBaseDir(baseDir)
	if err := applyOperatorDefaults(loaded, operatorDefaults); err != nil {
		return nil, err
	}
	if len(snapshot.InvocationSensitiveJSONSpans) > 0 {
		loaded = &invocationSensitiveSpanLoadedFactory{
			MutableLoadedFactorySource: loaded,
			spans:                      append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), snapshot.InvocationSensitiveJSONSpans...),
			promptProvenance:           append([]factorydefinitions.RuntimePromptProvenance(nil), snapshot.PromptProvenance...),
		}
	} else if len(snapshot.InvocationSensitiveJSONPointers) > 0 || len(snapshot.PromptProvenance) > 0 {
		loaded = &invocationSensitiveLoadedFactory{
			MutableLoadedFactorySource: loaded,
			pointers:                   append([]string(nil), snapshot.InvocationSensitiveJSONPointers...),
			promptProvenance:           append([]factorydefinitions.RuntimePromptProvenance(nil), snapshot.PromptProvenance...),
		}
	}
	return loaded, nil
}

type runtimeSnapshotLookup struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func newRuntimeSnapshotLookup(
	snapshot *factorydefinitions.RuntimeSnapshot,
) factorydefinitions.RuntimeDefinitionLookup {
	lookup := &runtimeSnapshotLookup{
		workers:      make(map[string]*factorydefinitions.FactoryWorkerConfig, len(snapshot.Workers)),
		workstations: make(map[string]*factorydefinitions.FactoryWorkstationConfig, len(snapshot.Workstations)),
	}
	for _, worker := range snapshot.Workers {
		cloned := factorydefinitions.CloneWorkerConfig(worker)
		lookup.workers[cloned.Name] = &cloned
	}
	for _, workstation := range snapshot.Workstations {
		cloned := factorydefinitions.CloneWorkstationConfig(workstation)
		lookup.workstations[cloned.Name] = &cloned
	}
	return lookup
}

func (lookup *runtimeSnapshotLookup) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if lookup == nil {
		return nil, false
	}
	worker, ok := lookup.workers[name]
	return worker, ok
}

func (lookup *runtimeSnapshotLookup) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if lookup == nil {
		return nil, false
	}
	workstation, ok := lookup.workstations[name]
	return workstation, ok
}

func attachRuntimeSnapshotPromptSources(
	config *factorydefinitions.FactoryConfig,
	sources []factorydefinitions.RuntimePromptSource,
) {
	if config == nil {
		return
	}
	for _, source := range sources {
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(source.Role)) {
		case "worker":
			for index := range config.Workers {
				if config.Workers[index].Name == source.Name {
					config.Workers[index].PromptSourcePath = source.Path
					break
				}
			}
		case "workstation":
			for index := range config.Workstations {
				if config.Workstations[index].Name == source.Name {
					config.Workstations[index].PromptSourcePath = source.Path
					config.Workstations[index].PromptSourceIsTemplate = source.IsTemplate
					break
				}
			}
		}
	}
}

func applyOperatorDefaults(
	loaded factorydefinitions.MutableLoadedFactorySource,
	operatorDefaults operatorconfig.ResolvedDefaults,
) error {
	if loaded == nil {
		return nil
	}
	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorDefaults); err != nil {
		return fmt.Errorf("apply operator defaults: %w", err)
	}
	return nil
}

func (loader *RuntimeInputLoading) warnReplayMetadataMismatches(
	dir string,
	replayPath string,
	artifact *factorydefinitions.ReplayArtifact,
	logger *zap.Logger,
	operatorDefaults operatorconfig.ResolvedDefaults,
) []recording.MetadataMismatchWarning {
	if artifact == nil ||
		dir == "" ||
		replayPath == "" {
		return nil
	}
	current, err := loader.loadFactory(dir, nil)
	if err != nil || current == nil {
		return nil
	}
	if err := applyOperatorDefaults(current, operatorDefaults); err != nil {
		return nil
	}
	currentSnapshot, err := loader.captureLoadedFactorySnapshot(
		current,
		current.FactoryDir(),
		nil,
	)
	if err != nil {
		return nil
	}
	warnings := recording.FactoryMetadataWarnings(artifact.Factory, currentSnapshot)
	for _, warning := range warnings {
		if logger != nil {
			logger.Warn(
				"replay artifact metadata differs from current checkout",
				zap.String("category", recording.DivergenceCategoryConfigMismatch),
				zap.String("metadata_key", warning.Key),
				zap.String("artifact", warning.Artifact),
				zap.String("current", warning.Current),
			)
		}
	}
	return warnings
}

func legacyReplayArtifactHasCanonicalEventShape(artifact recording.ReplayArtifact) bool {
	if len(artifact.Events) == 0 {
		return false
	}
	for _, event := range artifact.Events {
		if event.SchemaVersion == "" || strings.TrimSpace(event.Id) == "" ||
			event.Type == "" || event.Context.EventTime.IsZero() {
			return false
		}
	}
	return true
}
