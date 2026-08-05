package runtimeopening

import (
	"context"
	"errors"
	"fmt"

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
	LoadedFactoryCfg  factorydefinitions.MutableLoadedFactorySource
	ReplayArtifact    *factorydefinitions.ReplayArtifact
	PortableRecording *recording.PortableRecording
	HistoricalReplay  *recordingreplay.RecordingReplayProjection
	SessionLogger     *zap.Logger
}

// LoadRuntime preserves the existing internal Runtime-loading entry point for
// replay callers. Runtime opening uses LoadRuntimeFromDefinition so authored
// definitions always cross the focused Factory Definitions boundary.
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
		context.Background(),
		factorydefinitions.RuntimeOpeningRequest{
			Directory:        dir,
			ExecutionBaseDir: executionBaseDir,
		},
		replayPath,
		operatorDefaults,
		workstationLoader,
		root,
		nil,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		replayInputs,
		captureLoadedFactorySnapshot,
		newSessionLogger,
	)
}

// LoadRuntimeFromDefinition loads a live authored Factory through the narrow
// Factory Definitions-owned capability. Replay construction retains its
// existing snapshot path and therefore does not invoke the authored loader.
func LoadRuntimeFromDefinition(
	ctx context.Context,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	root RuntimeRoot,
	authoredDefinitionLoader factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recording.ReplayInputLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
) (RuntimeLoad, error) {
	return loadRuntime(
		ctx,
		definitionRequest,
		replayPath,
		operatorDefaults,
		nil,
		root,
		authoredDefinitionLoader,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		replayInputs,
		captureLoadedFactorySnapshot,
		newSessionLogger,
	)
}

func loadRuntime(
	ctx context.Context,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	workstationLoader factorydefinitions.WorkstationLoader,
	root RuntimeRoot,
	authoredDefinitionLoader factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recording.ReplayInputLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
) (RuntimeLoad, error) {
	if newSessionLogger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory is required")
	}
	logger := newSessionLogger(
		root.BaseLogger,
		factorysessions.DefaultSessionID,
		root.FactoryRootDir,
		runtimeDefinitionLogPath(definitionRequest),
	)
	if logger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory returned nil")
	}
	var legacyArtifact *factorydefinitions.ReplayArtifact
	if replayPath != "" {
		if replayInputs == nil {
			return RuntimeLoad{}, fmt.Errorf("Factory Session replay input capability is required")
		}
		result, err := replayInputs.LoadReplayInput(recording.LoadReplayInputRequest{Path: replayPath})
		if err != nil {
			var inputErr *recording.ReplayInputError
			if errors.As(err, &inputErr) && inputErr.Family == recording.ReplayInputFamilyLegacy {
				return RuntimeLoad{}, fmt.Errorf("load factory config: %w", err)
			}
			return RuntimeLoad{}, fmt.Errorf("load portable replay: %w", err)
		}
		if result.Portable != nil {
			projection, err := recordingreplay.ReplayRecording(*result.Portable)
			if err != nil {
				return RuntimeLoad{}, fmt.Errorf("load portable replay: inspect historical recording: %w", err)
			}
			return RuntimeLoad{
				PortableRecording: result.Portable,
				HistoricalReplay:  &projection,
				SessionLogger:     logger,
			}, nil
		}
		legacyArtifact = result.Legacy
	}

	logger.Info("loading factory config", zap.String("dir", runtimeDefinitionLogPath(definitionRequest)))
	loaded, artifact, err := loadRuntimeConfig(
		ctx,
		definitionRequest,
		replayPath,
		operatorDefaults,
		authoredDefinitionLoader,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		legacyArtifact,
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
	}
	warnReplayMetadataMismatches(
		definitionRequest.Directory,
		replayPath,
		workstationLoader,
		artifact,
		logger,
		loadFactory,
		captureLoadedFactorySnapshot,
	)
	return RuntimeLoad{
		LoadedFactoryCfg: loaded,
		ReplayArtifact:   artifact,
		SessionLogger:    logger,
	}, nil
}

func loadRuntimeConfig(
	ctx context.Context,
	definitionRequest factorydefinitions.RuntimeOpeningRequest,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	authoredDefinitionLoader factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	artifact *factorydefinitions.ReplayArtifact,
) (factorydefinitions.MutableLoadedFactorySource, *factorydefinitions.ReplayArtifact, error) {
	if replayPath == "" {
		loaded, err := loadAuthoredRuntimeConfig(
			ctx,
			definitionRequest,
			operatorDefaults,
			authoredDefinitionLoader,
			newLoadedFactory,
		)
		return loaded, nil, err
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
	loaded.SetRuntimeBaseDir(definitionRequest.ExecutionBaseDir)
	return loaded, artifact, nil
}

func loadAuthoredRuntimeConfig(
	ctx context.Context,
	request factorydefinitions.RuntimeOpeningRequest,
	operatorDefaults operatorconfig.ResolvedDefaults,
	loader factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if loader == nil {
		return nil, fmt.Errorf("validated Factory Definitions loader is required")
	}
	result, err := loader.LoadValidatedAuthoredFactoryDefinition(
		ctx,
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
			Directory:        request.Directory,
			SourcePath:       request.SourcePath,
			ExecutionBaseDir: request.ExecutionBaseDir,
		},
	)
	if err != nil {
		return nil, err
	}
	if result.Definition == nil {
		return nil, fmt.Errorf("validated Factory Definitions loader returned no definition")
	}
	if newLoadedFactory == nil {
		return nil, fmt.Errorf("Factory Definitions loaded-source factory is required")
	}
	loaded, err := newLoadedFactory(
		result.FactoryDir,
		result.Definition,
		alreadyEffectiveRuntimeDefinitions{},
		result.BundledFileReplacements,
	)
	if err != nil {
		return nil, fmt.Errorf("build validated Factory Definition source: %w", err)
	}
	if loaded == nil {
		return nil, fmt.Errorf("Factory Definitions loaded-source factory returned nil")
	}
	loaded.SetRuntimeBaseDir(result.RuntimeBaseDir)
	if receiver, ok := loaded.(factorydefinitions.AuthoredFactorySourceIdentityReceiver); ok {
		receiver.SetAuthoredFactorySourceIdentity(result.Source)
	}
	if err := applyOperatorDefaults(loaded, operatorDefaults); err != nil {
		return nil, err
	}
	return loaded, nil
}

// alreadyEffectiveRuntimeDefinitions preserves the effective authored result
// returned by Factory Definitions while the existing mutable Runtime source is
// assembled. The source factory clones and normalizes that detached result;
// it must not rediscover authored split files through Runtime.
type alreadyEffectiveRuntimeDefinitions struct{}

func (alreadyEffectiveRuntimeDefinitions) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (alreadyEffectiveRuntimeDefinitions) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func runtimeDefinitionLogPath(request factorydefinitions.RuntimeOpeningRequest) string {
	if request.SourcePath != "" {
		return request.SourcePath
	}
	return request.Directory
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
) {
	if artifact == nil ||
		dir == "" ||
		replayPath == "" ||
		loadFactory == nil ||
		captureLoadedFactorySnapshot == nil {
		return
	}
	current, err := loadFactory(dir, workstationLoader)
	if err != nil {
		return
	}
	currentSnapshot, err := captureLoadedFactorySnapshot(
		current,
		current.FactoryDir(),
		nil,
	)
	if err != nil {
		return
	}
	for _, warning := range recording.FactoryMetadataWarnings(artifact.Factory, currentSnapshot) {
		logger.Warn(
			"replay artifact metadata differs from current checkout",
			zap.String("category", recording.DivergenceCategoryConfigMismatch),
			zap.String("metadata_key", warning.Key),
			zap.String("artifact", warning.Artifact),
			zap.String("current", warning.Current),
		)
	}
}
