package replay

import (
	"errors"
	"fmt"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

// GeneratedFactoryOption customizes the generated Factory payload captured for
// record/replay serialization.
type GeneratedFactoryOption func(*generatedFactoryOptions)

type generatedFactoryOptions struct {
	sourceDirectory string
	workflowID      string
	metadata        map[string]string
}

// WithGeneratedFactorySourceDirectory records the source factory directory used
// for the run.
func WithGeneratedFactorySourceDirectory(dir string) GeneratedFactoryOption {
	return func(opts *generatedFactoryOptions) {
		opts.sourceDirectory = dir
	}
}

// WithGeneratedFactoryWorkflowID records the workflow identifier associated
// with the run when one is available from the caller.
func WithGeneratedFactoryWorkflowID(workflowID string) GeneratedFactoryOption {
	return func(opts *generatedFactoryOptions) {
		opts.workflowID = workflowID
	}
}

// WithGeneratedFactoryMetadata records caller-owned metadata on the generated
// Factory payload.
func WithGeneratedFactoryMetadata(metadata map[string]string) GeneratedFactoryOption {
	return func(opts *generatedFactoryOptions) {
		opts.metadata = cloneStringMap(metadata)
	}
}

// GeneratedFactoryFromLoadedConfig serializes the already-loaded runtime config
// into the generated Factory API model used at replay, API, and event
// boundaries.
func GeneratedFactoryFromLoadedConfig(loaded *config.LoadedFactoryConfig, opts ...GeneratedFactoryOption) (factoryapi.Factory, error) {
	if loaded == nil {
		return factoryapi.Factory{}, errors.New("loaded factory config is required")
	}
	return GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded, opts...)
}

// GeneratedFactoryFromRuntimeConfig serializes runtime worker and workstation
// definitions into the generated Factory API model without adding a secondary
// config wrapper.
func GeneratedFactoryFromRuntimeConfig(factoryDir string, factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup, opts ...GeneratedFactoryOption) (factoryapi.Factory, error) {
	if factoryCfg == nil {
		return factoryapi.Factory{}, errors.New("factory config is required")
	}
	if runtimeCfg == nil {
		return factoryapi.Factory{}, errors.New("runtime config is required")
	}

	options := generatedFactoryOptions{sourceDirectory: factoryDir}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	generated := generatedFactoryAPIFromConfig(factoryCfg)
	preserveGeneratedResourceUsage(factoryCfg, &generated)

	workers := runtimeWorkersByName(factoryCfg, runtimeCfg)
	workstations := runtimeWorkstationsByName(factoryCfg, runtimeCfg)
	if err := mergeGeneratedWorkers(&generated, workers); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := mergeGeneratedWorkstations(&generated, workstations); err != nil {
		return factoryapi.Factory{}, err
	}

	factoryWithRuntime, err := config.FactoryConfigWithRuntimeDefinitions(factoryCfg, runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("inline runtime factory config: %w", err)
	}
	generated.FactoryDir = stringPtrIfNotEmpty(factoryDir)
	generated.SourceDirectory = stringPtrIfNotEmpty(options.sourceDirectory)
	generated.WorkflowId = stringPtrIfNotEmpty(options.workflowID)
	generated.Metadata = generatedStringMapPtr(generatedFactoryMetadata(
		factoryWithRuntime,
		workers,
		workstations,
		options.metadata,
	))
	return generated, nil
}

func runtimeWorkersByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]interfaces.WorkerConfig {
	workers := make(map[string]interfaces.WorkerConfig)
	for _, workerCfg := range factoryCfg.Workers {
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || def == nil {
			continue
		}
		workers[workerCfg.Name] = config.CloneWorkerConfig(*def)
	}
	return workers
}

func runtimeWorkstationsByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]interfaces.FactoryWorkstationConfig {
	workstations := make(map[string]interfaces.FactoryWorkstationConfig, len(factoryCfg.Workstations))
	for _, workstationCfg := range factoryCfg.Workstations {
		def, ok := runtimeCfg.Workstation(workstationCfg.Name)
		if !ok || def == nil {
			workstations[workstationCfg.Name] = config.CloneWorkstationConfig(workstationCfg)
			continue
		}
		workstations[workstationCfg.Name] = mergeRuntimeWorkstationForGeneratedFactory(workstationCfg, *def)
	}
	return workstations
}

func mergeRuntimeWorkstationForGeneratedFactory(base, runtime interfaces.FactoryWorkstationConfig) interfaces.FactoryWorkstationConfig {
	merged := config.CloneWorkstationConfig(runtime)
	if merged.Name == "" {
		merged.Name = base.Name
	}
	if merged.ID == "" {
		merged.ID = base.ID
	}
	if merged.Type == "" {
		merged.Type = base.Type
	}
	if merged.WorkerTypeName == "" {
		merged.WorkerTypeName = base.WorkerTypeName
	}
	if merged.Cron == nil {
		merged.Cron = base.Cron
	}
	if len(merged.Inputs) == 0 {
		merged.Inputs = base.Inputs
	}
	if len(merged.Outputs) == 0 {
		merged.Outputs = base.Outputs
	}
	if merged.OnContinue == nil {
		merged.OnContinue = base.OnContinue
	}
	if merged.OnRejection == nil {
		merged.OnRejection = base.OnRejection
	}
	if merged.OnFailure == nil {
		merged.OnFailure = base.OnFailure
	}
	if len(merged.Resources) == 0 {
		merged.Resources = base.Resources
	}
	if len(merged.Guards) == 0 {
		merged.Guards = base.Guards
	}
	return config.CloneWorkstationConfig(merged)
}

func generatedFactoryMetadata(
	factoryWithRuntime *interfaces.FactoryConfig,
	workers map[string]interfaces.WorkerConfig,
	workstations map[string]interfaces.FactoryWorkstationConfig,
	metadata map[string]string,
) map[string]string {
	out := cloneStringMap(metadata)
	if out == nil {
		out = make(map[string]string)
	}
	out[metadataReplaySourceFormat] = CurrentSchemaVersion
	out[metadataFactoryHash] = sha256JSON(factoryWithRuntime)
	out[metadataWorkersHash] = sha256JSON(workers)
	out[metadataWorkstationsHash] = sha256JSON(workstations)
	out[metadataRuntimeConfigHash] = sha256JSON(struct {
		Factory      *interfaces.FactoryConfig                      `json:"factory"`
		Workers      map[string]interfaces.WorkerConfig             `json:"workers,omitempty"`
		Workstations map[string]interfaces.FactoryWorkstationConfig `json:"workstations,omitempty"`
	}{
		Factory:      factoryWithRuntime,
		Workers:      workers,
		Workstations: workstations,
	})
	return out
}
