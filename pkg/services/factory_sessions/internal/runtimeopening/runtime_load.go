package runtimeopening

import (
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	SessionLogger     *zap.Logger
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
	if newSessionLogger == nil {
		return RuntimeLoad{}, fmt.Errorf("Factory Runtime session logger factory is required")
	}
	logger := newSessionLogger(
		root.BaseLogger,
		factorysessions.DefaultSessionID,
		root.FactoryRootDir,
		dir,
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
			return RuntimeLoad{
				PortableRecording: result.Portable,
				SessionLogger:     logger,
			}, nil
		}
		legacyArtifact = result.Legacy
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
		dir,
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
	dir string,
	executionBaseDir string,
	replayPath string,
	operatorDefaults operatorconfig.ResolvedDefaults,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	artifact *factorydefinitions.ReplayArtifact,
) (factorydefinitions.MutableLoadedFactorySource, *factorydefinitions.ReplayArtifact, error) {
	if replayPath == "" {
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
