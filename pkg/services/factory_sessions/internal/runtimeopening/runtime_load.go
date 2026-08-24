package runtimeopening

import (
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/operatordefaults"
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
	pointers []string
}

func (source *invocationSensitiveLoadedFactory) InvocationSensitiveJSONPointers() []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source.pointers...)
}

type runtimeReplayLoad struct {
	legacyArtifact    *factorydefinitions.ReplayArtifact
	portableRecording *recording.PortableRecording
	historicalReplay  *recordingreplay.RecordingReplayProjection
}

func LoadRuntime(
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	workstationLoader factorydefinitions.WorkstationLoader,
	root RuntimeRoot,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recording.ReplayInputLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
) (RuntimeLoad, error) {
	return loadRuntime(
		dir,
		executionBaseDir,
		replayPath,
		operatorDefaults,
		workstationLoader,
		root,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		replayInputs,
		captureLoadedFactorySnapshot,
		newSessionLogger,
		nil,
		nil,
		factorysessions.DefaultSessionID,
	)
}

func loadRuntime(
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	workstationLoader factorydefinitions.WorkstationLoader,
	root RuntimeRoot,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recording.ReplayInputLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	resolvedSnapshot *factorydefinitions.RuntimeSnapshot,
	preloadedReplayInput *recording.LoadReplayInputResult,
	sessionID string,
) (RuntimeLoad, error) {
	if newSessionLogger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	logger := newSessionLogger(
		root.BaseLogger,
		sessionID,
		root.FactoryRootDir,
		dir,
	)
	if logger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory returned nil")
	}
	replayLoad, err := loadRuntimeReplay(
		replayPath,
		replayInputs,
		preloadedReplayInput,
	)
	if err != nil {
		return RuntimeLoad{}, err
	}
	if replayLoad.portableRecording != nil {
		return RuntimeLoad{
			PortableRecording: replayLoad.portableRecording,
			HistoricalReplay:  replayLoad.historicalReplay,
			SessionLogger:     logger,
		}, nil
	}

	logger.Info("loading factory config", zap.String("dir", dir))
	loaded, artifact, err := loadRuntimeConfig(
		dir,
		executionBaseDir,
		replayPath,
		operatorDefaults,
		workstationLoader,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
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
	replayMetadataWarnings := reportRuntimeReplayMetadata(
		dir,
		replayPath,
		workstationLoader,
		artifact,
		logger,
		loadFactory,
		captureLoadedFactorySnapshot,
		operatorDefaults,
	)
	return RuntimeLoad{
		LoadedFactoryCfg:       loaded,
		ReplayArtifact:         artifact,
		ReplayMetadataWarnings: replayMetadataWarnings,
		SessionLogger:          logger,
	}, nil
}

func loadRuntimeReplay(
	replayPath string,
	replayInputs recording.ReplayInputLoader,
	preloadedReplayInput *recording.LoadReplayInputResult,
) (runtimeReplayLoad, error) {
	if replayPath == "" {
		return runtimeReplayLoad{}, nil
	}
	if replayInputs == nil {
		return runtimeReplayLoad{}, fmt.Errorf("Factory Session replay input capability is required")
	}
	var result recording.LoadReplayInputResult
	var err error
	if preloadedReplayInput != nil {
		result = *preloadedReplayInput
	} else {
		result, err = replayInputs.LoadReplayInput(recording.LoadReplayInputRequest{Path: replayPath})
	}
	if err != nil {
		var inputErr *recording.ReplayInputError
		if errors.As(err, &inputErr) && inputErr.Family == recording.ReplayInputFamilyLegacy {
			return runtimeReplayLoad{}, fmt.Errorf("load factory config: %w", err)
		}
		return runtimeReplayLoad{}, fmt.Errorf("load portable replay: %w", err)
	}
	if result.Portable == nil {
		return runtimeReplayLoad{legacyArtifact: result.Legacy}, nil
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

func reportRuntimeReplayMetadata(
	dir string,
	replayPath string,
	workstationLoader factorydefinitions.WorkstationLoader,
	artifact *factorydefinitions.ReplayArtifact,
	logger *zap.Logger,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	operatorDefaults operatorconfig.ResolvedDefaults,
) []recording.MetadataMismatchWarning {
	return warnReplayMetadataMismatches(
		dir,
		replayPath,
		workstationLoader,
		artifact,
		logger,
		loadFactory,
		captureLoadedFactorySnapshot,
		operatorDefaults,
	)
}

func loadRuntimeConfig(
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	artifact *factorydefinitions.ReplayArtifact,
	resolvedSnapshot *factorydefinitions.RuntimeSnapshot,
) (factorydefinitions.MutableLoadedFactorySource, *factorydefinitions.ReplayArtifact, error) {
	if replayPath == "" {
		if resolvedSnapshot != nil {
			loaded, err := loadRuntimeSnapshot(
				resolvedSnapshot,
				executionBaseDir,
				operatorDefaults,
				newLoadedFactory,
			)
			return loaded, nil, err
		}
		if loadFactory == nil {
			return nil, nil, fmt.Errorf("Factory Definitions loader is required")
		}
		loaded, err := loadFactory(dir, workstationLoader)
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
	if decodeReplayConfig == nil {
		return nil, nil, fmt.Errorf("replay Factory Definition decoder is required")
	}
	if newLoadedFactory == nil {
		return nil, nil, fmt.Errorf("Factory Definitions loaded-source factory is required")
	}
	runtimeConfig, err := decodeReplayConfig(artifact.Factory)
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded replay config: %w", err)
	}
	loaded, err := newLoadedFactory(
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

func loadRuntimeSnapshot(
	resolved *factorydefinitions.RuntimeSnapshot,
	executionBaseDir string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved Factory Definition snapshot is required")
	}
	if newLoadedFactory == nil {
		return nil, fmt.Errorf("Factory Definitions loaded-source factory is required")
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
	loaded, err := newLoadedFactory(
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
	if len(snapshot.InvocationSensitiveJSONPointers) > 0 {
		loaded = &invocationSensitiveLoadedFactory{
			MutableLoadedFactorySource: loaded,
			pointers:                   append([]string(nil), snapshot.InvocationSensitiveJSONPointers...),
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

func warnReplayMetadataMismatches(
	dir string,
	replayPath string,
	workstationLoader factorydefinitions.WorkstationLoader,
	artifact *factorydefinitions.ReplayArtifact,
	logger *zap.Logger,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	operatorDefaults operatorconfig.ResolvedDefaults,
) []recording.MetadataMismatchWarning {
	if artifact == nil ||
		dir == "" ||
		replayPath == "" ||
		loadFactory == nil ||
		captureLoadedFactorySnapshot == nil {
		return nil
	}
	current, err := loadFactory(dir, workstationLoader)
	if err != nil {
		return nil
	}
	if err := applyOperatorDefaults(current, operatorDefaults); err != nil {
		return nil
	}
	currentSnapshot, err := captureLoadedFactorySnapshot(
		current,
		current.FactoryDir(),
		nil,
	)
	if err != nil {
		return nil
	}
	warnings := recording.FactoryMetadataWarnings(artifact.Factory, currentSnapshot)
	for _, warning := range warnings {
		logger.Warn(
			"replay artifact metadata differs from current checkout",
			zap.String("category", recording.DivergenceCategoryConfigMismatch),
			zap.String("metadata_key", warning.Key),
			zap.String("artifact", warning.Artifact),
			zap.String("current", warning.Current),
		)
	}
	return warnings
}
