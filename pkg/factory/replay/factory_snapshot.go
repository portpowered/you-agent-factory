package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

// FactorySnapshotOption customizes the Factory snapshot captured for replay.
type FactorySnapshotOption func(*factorySnapshotOptions)

type factorySnapshotOptions struct {
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

// EmbeddedRuntimeConfig is the canonical runtime lookup reconstructed from an
// artifact's embedded configuration. It intentionally avoids filesystem reads.
type EmbeddedRuntimeConfig struct {
	Factory          *interfaces.FactoryConfig
	FactoryDirPath   string
	WorkerConfigs    map[string]*workerconfig.Config
	Workstations     map[string]*interfaces.FactoryWorkstationConfig
	WorkersByID      map[string]*workerconfig.Config
	WorkstationsByID map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeConfigLookup = (*EmbeddedRuntimeConfig)(nil)
var _ interfaces.RuntimeFactoryConfigLookup = (*EmbeddedRuntimeConfig)(nil)

func (c *EmbeddedRuntimeConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return c.Factory
}

func (c *EmbeddedRuntimeConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.FactoryDirPath
}

func (c *EmbeddedRuntimeConfig) RuntimeBaseDir() string { return c.FactoryDir() }

func (c *EmbeddedRuntimeConfig) Worker(name string) (*workerconfig.Config, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.WorkerConfigs[name]
	return def, ok
}

func (c *EmbeddedRuntimeConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.Workstations[name]
	return def, ok
}

func WithFactorySnapshotSourceDirectory(dir string) FactorySnapshotOption {
	return func(opts *factorySnapshotOptions) { opts.sourceDirectory = dir }
}

func WithFactorySnapshotWorkflowID(workflowID string) FactorySnapshotOption {
	return func(opts *factorySnapshotOptions) { opts.workflowID = workflowID }
}

func WithFactorySnapshotMetadata(metadata map[string]string) FactorySnapshotOption {
	return func(opts *factorySnapshotOptions) { opts.metadata = cloneStringMap(metadata) }
}

// FactorySnapshotFromLoadedConfig captures the already-loaded runtime config
// without exposing an HTTP-generated Factory contract to replay policy.
func FactorySnapshotFromLoadedConfig(loaded *config.LoadedFactoryConfig, opts ...FactorySnapshotOption) (*interfaces.FactorySnapshot, error) {
	if loaded == nil {
		return nil, errors.New("loaded factory config is required")
	}
	return FactorySnapshotFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded, opts...)
}

// FactorySnapshotFromRuntimeConfig captures runtime definitions and replay
// metadata in the canonical public JSON shape retained by replay artifacts.
func FactorySnapshotFromRuntimeConfig(factoryDir string, factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup, opts ...FactorySnapshotOption) (*interfaces.FactorySnapshot, error) {
	if factoryCfg == nil {
		return nil, errors.New("factory config is required")
	}
	if runtimeCfg == nil {
		return nil, errors.New("runtime config is required")
	}

	options := factorySnapshotOptions{sourceDirectory: factoryDir}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	workers := runtimeWorkersByName(factoryCfg, runtimeCfg)
	workstations := runtimeWorkstationsByName(factoryCfg, runtimeCfg)
	factoryWithRuntime, err := config.FactoryConfigWithRuntimeDefinitions(factoryCfg, runtimeCfg)
	if err != nil {
		return nil, fmt.Errorf("inline runtime factory config: %w", err)
	}
	generated := config.FactoryConfigToOpenAPI(factoryWithRuntime)
	payload, err := json.Marshal(generated)
	if err != nil {
		return nil, fmt.Errorf("marshal replay factory config: %w", err)
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, fmt.Errorf("decode replay factory config: %w", err)
	}
	if factoryDir != "" {
		object["factoryDirectory"] = factoryDir
	}
	if options.sourceDirectory != "" {
		object["sourceDirectory"] = options.sourceDirectory
	}
	object["metadata"] = replayFactorySnapshotMetadata(factoryWithRuntime, workers, workstations, options.metadata)
	return interfaces.NewFactorySnapshot(object)
}

// RuntimeConfigFromFactorySnapshot rebuilds the runtime lookup from the
// Factory-owned snapshot hydrated from a replay artifact.
func RuntimeConfigFromFactorySnapshot(snapshot *interfaces.FactorySnapshot) (*EmbeddedRuntimeConfig, error) {
	if snapshot == nil {
		return nil, errors.New("replay artifact factory is required")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode replay artifact factory: %w", err)
	}
	factoryCopy, err := config.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		FactoryDirectory string `json:"factoryDirectory"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode replay artifact metadata: %w", err)
	}
	runtimeCfg := &EmbeddedRuntimeConfig{
		Factory:          factoryCopy,
		FactoryDirPath:   envelope.FactoryDirectory,
		WorkerConfigs:    make(map[string]*workerconfig.Config),
		Workstations:     make(map[string]*interfaces.FactoryWorkstationConfig),
		WorkersByID:      make(map[string]*workerconfig.Config),
		WorkstationsByID: make(map[string]*interfaces.FactoryWorkstationConfig),
	}
	for _, worker := range factoryCopy.Workers {
		worker.ExecutorProvider = normalizeReplayWorkerProvider(worker.ExecutorProvider)
		defCopy := config.CloneWorkerConfig(worker)
		runtimeCfg.WorkerConfigs[worker.Name] = &defCopy
		runtimeCfg.WorkersByID[worker.Name] = &defCopy
	}
	for _, workstation := range factoryCopy.Workstations {
		defCopy := config.CloneWorkstationConfig(workstation)
		runtimeCfg.Workstations[workstation.Name] = &defCopy
		if workstation.ID != "" {
			runtimeCfg.WorkstationsByID[workstation.ID] = &defCopy
		}
	}
	return runtimeCfg, nil
}

func validatedFactorySnapshotFromOpenAPIJSON(data []byte) (*interfaces.FactorySnapshot, error) {
	generated, err := config.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return interfaces.NewFactorySnapshot(generated)
}

func runtimeWorkersByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]workerconfig.Config {
	workers := make(map[string]workerconfig.Config)
	for _, workerCfg := range factoryCfg.Workers {
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if ok && def != nil {
			workers[workerCfg.Name] = config.CloneWorkerConfig(*def)
		}
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
		workstations[workstationCfg.Name] = mergeRuntimeWorkstationForFactorySnapshot(workstationCfg, *def)
	}
	return workstations
}

func normalizeReplayWorkerProvider(value string) string {
	return interfaces.PermissivePublicFactoryWorkerProvider(strings.TrimSpace(value))
}

func mergeRuntimeWorkstationForFactorySnapshot(base, runtime interfaces.FactoryWorkstationConfig) interfaces.FactoryWorkstationConfig {
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

func replayFactorySnapshotMetadata(factoryWithRuntime *interfaces.FactoryConfig, workers map[string]workerconfig.Config, workstations map[string]interfaces.FactoryWorkstationConfig, metadata map[string]string) map[string]string {
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
		Workers      map[string]workerconfig.Config                 `json:"workers,omitempty"`
		Workstations map[string]interfaces.FactoryWorkstationConfig `json:"workstations,omitempty"`
	}{factoryWithRuntime, workers, workstations})
	return out
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

func sortedWorkerNames(workers map[string]workerconfig.Config) []string {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
