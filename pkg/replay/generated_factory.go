package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// GeneratedFactoryOption customizes the generated Factory payload captured for
// record/replay serialization.
type GeneratedFactoryOption func(*generatedFactoryOptions)

type generatedFactoryOptions struct {
	sourceDirectory string
	workflowID      string
	metadata        map[string]string
}

const (
	metadataFactoryHash        = "factory_hash"
	metadataWorkersHash        = "workers_hash"
	metadataWorkstationsHash   = "workstations_hash"
	metadataRuntimeConfigHash  = "runtime_config_hash"
	metadataReplaySourceFormat = "source_format"
)

// MetadataMismatchWarning describes replay artifact metadata that differs from
// the current checkout's loadable runtime config.
type MetadataMismatchWarning struct {
	Key      string
	Artifact string
	Current  string
}

// EmbeddedRuntimeConfig is the canonical runtime lookup reconstructed from an
// artifact's embedded configuration. It intentionally avoids filesystem reads.
type EmbeddedRuntimeConfig struct {
	Factory          *interfaces.FactoryConfig
	FactoryDirPath   string
	WorkerConfigs    map[string]*interfaces.WorkerConfig
	Workstations     map[string]*interfaces.FactoryWorkstationConfig
	WorkersByID      map[string]*interfaces.WorkerConfig
	WorkstationsByID map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeConfigLookup = (*EmbeddedRuntimeConfig)(nil)

// FactoryConfig returns the embedded canonical public factory configuration.
func (c *EmbeddedRuntimeConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return c.Factory
}

// FactoryDir returns the authored factory root embedded in the replay artifact.
func (c *EmbeddedRuntimeConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.FactoryDirPath
}

// RuntimeBaseDir returns the effective execution base for relative runtime
// paths during replay-backed execution. Replay artifacts do not carry a
// separate runtime-base override, so relative runtime paths fall back to the
// embedded factory root.
func (c *EmbeddedRuntimeConfig) RuntimeBaseDir() string {
	return c.FactoryDir()
}

// Worker returns the embedded worker definition for the configured worker name.
func (c *EmbeddedRuntimeConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.WorkerConfigs[name]
	return def, ok
}

// Workstation returns the embedded workstation definition for the configured workstation name.
func (c *EmbeddedRuntimeConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.Workstations[name]
	return def, ok
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
	generated.FactoryDirectory = stringPtrIfNotEmpty(factoryDir)
	generated.SourceDirectory = stringPtrIfNotEmpty(options.sourceDirectory)
	generated.Metadata = generatedStringMapPtr(generatedFactoryMetadata(
		factoryWithRuntime,
		workers,
		workstations,
		options.metadata,
	))
	return generated, nil
}

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

func generatedFactoryAPIFromConfig(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	return config.FactoryConfigToOpenAPI(cfg)
}

func generatedWorkstationAPIFromConfig(name string, cfg interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	workstation := config.WorkstationConfigToOpenAPI(cfg)
	if workstation.Name == "" {
		workstation.Name = name
	}
	return workstation
}

func generatedWorkerAPIFromConfig(name string, cfg interfaces.WorkerConfig) factoryapi.Worker {
	worker := config.WorkerConfigToOpenAPI(cfg)
	if worker.Name == "" {
		worker.Name = name
	}
	return worker
}

func factoryConfigFromGeneratedAPI(generated factoryapi.Factory) (*interfaces.FactoryConfig, error) {
	cfg, err := config.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func workstationConfigFromGeneratedAPI(workstation factoryapi.Workstation) (interfaces.FactoryWorkstationConfig, error) {
	return config.WorkstationConfigFromOpenAPI(workstation)
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

func sha256JSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "sha256:error"
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:]))
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
