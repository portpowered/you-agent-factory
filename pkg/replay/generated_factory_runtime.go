package replay

import (
	"errors"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// RuntimeConfigFromGeneratedFactory rebuilds the canonical runtime lookup
// contract from a generated Factory payload carried by RUN_REQUEST. Replay
// uses this path so artifacts remain self-contained without a secondary config
// schema, and relative runtime paths resolve from the embedded factory root
// because replay does not persist a separate runtime-base override.
func RuntimeConfigFromGeneratedFactory(generated factoryapi.Factory) (*EmbeddedRuntimeConfig, error) {
	if !generatedFactoryHasConfig(generated) {
		return nil, errors.New("replay artifact factory is required")
	}

	factoryCopy, err := factoryConfigFromGeneratedAPI(generated)
	if err != nil {
		return nil, err
	}
	restoreReplayResourceUsage(generated, factoryCopy)

	runtimeCfg := &EmbeddedRuntimeConfig{
		Factory:          factoryCopy,
		FactoryDirPath:   stringValue(generated.FactoryDirectory),
		WorkerConfigs:    make(map[string]*interfaces.WorkerConfig),
		Workstations:     make(map[string]*interfaces.FactoryWorkstationConfig),
		WorkersByID:      make(map[string]*interfaces.WorkerConfig),
		WorkstationsByID: make(map[string]*interfaces.FactoryWorkstationConfig),
	}

	if generated.Workers != nil {
		for _, worker := range *generated.Workers {
			if !generatedWorkerHasRuntimeDefinition(worker) {
				continue
			}
			converted, err := config.WorkerConfigFromOpenAPI(worker)
			if err != nil {
				return nil, fmt.Errorf("convert worker %q: %w", worker.Name, err)
			}
			if converted.Name == "" {
				converted.Name = worker.Name
			}
			if converted.ExecutorProvider != "" {
				converted.ExecutorProvider = normalizeReplayWorkerProvider(converted.ExecutorProvider)
			}
			defCopy := config.CloneWorkerConfig(converted)
			runtimeCfg.WorkerConfigs[converted.Name] = &defCopy
			runtimeCfg.WorkersByID[converted.Name] = &defCopy
		}
	}

	if generated.Workstations != nil {
		for _, workstation := range *generated.Workstations {
			cfg, err := workstationConfigFromGeneratedAPI(workstation)
			if err != nil {
				return nil, err
			}
			defCopy := config.CloneWorkstationConfig(cfg)
			runtimeCfg.Workstations[workstation.Name] = &defCopy
			if cfg.ID != "" {
				runtimeCfg.WorkstationsByID[cfg.ID] = &defCopy
			}
		}
	}

	return runtimeCfg, nil
}

// FactoryMetadataWarnings compares replay Factory metadata against the current
// checkout's generated Factory metadata. Replay callers should warn but still
// allow replay because artifacts are authoritative for runtime configuration.
func FactoryMetadataWarnings(artifactFactory, currentFactory factoryapi.Factory) []MetadataMismatchWarning {
	artifactMetadata := stringMapValue(artifactFactory.Metadata)
	currentMetadata := stringMapValue(currentFactory.Metadata)
	keys := []string{
		metadataFactoryHash,
		metadataWorkersHash,
		metadataWorkstationsHash,
		metadataRuntimeConfigHash,
	}
	warnings := make([]MetadataMismatchWarning, 0, len(keys))
	for _, key := range keys {
		artifactValue := artifactMetadata[key]
		currentValue := currentMetadata[key]
		if artifactValue == "" || currentValue == "" || artifactValue == currentValue {
			continue
		}
		warnings = append(warnings, MetadataMismatchWarning{
			Key:      key,
			Artifact: artifactValue,
			Current:  currentValue,
		})
	}
	return warnings
}

func generatedFactoryHasConfig(generated factoryapi.Factory) bool {
	return generated.WorkTypes != nil ||
		generated.Resources != nil ||
		generated.Workers != nil ||
		generated.Workstations != nil ||
		generated.InputTypes != nil ||
		generated.Id != nil ||
		generated.FactoryDirectory != nil ||
		generated.SourceDirectory != nil ||
		generated.Metadata != nil
}

func generatedWorkerHasRuntimeDefinition(worker factoryapi.Worker) bool {
	return worker.Type != nil ||
		worker.Command != nil ||
		worker.Model != nil ||
		worker.ModelProvider != nil ||
		worker.ExecutorProvider != nil ||
		worker.SkipPermissions != nil ||
		worker.StopToken != nil ||
		worker.Timeout != nil ||
		worker.Body != nil ||
		worker.Args != nil ||
		worker.Resources != nil
}

func normalizeReplayWorkerProvider(value string) string {
	return interfaces.PermissivePublicFactoryWorkerProvider(strings.TrimSpace(value))
}
