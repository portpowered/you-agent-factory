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
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
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
func (c *EmbeddedRuntimeConfig) Worker(name string) (*workerconfig.Config, bool) {
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
	workstationList := workstationConfigsFromMap(workstations)
	if err := mergeGeneratedWorkers(&generated, workers, workstationList); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := mergeGeneratedWorkstations(&generated, workstations, workers); err != nil {
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
		WorkerConfigs:    make(map[string]*workerconfig.Config),
		Workstations:     make(map[string]*interfaces.FactoryWorkstationConfig),
		WorkersByID:      make(map[string]*workerconfig.Config),
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

// RuntimeConfigFromFactorySnapshot rebuilds the runtime lookup from the
// Factory-owned snapshot hydrated from a replay artifact.
func RuntimeConfigFromFactorySnapshot(snapshot *interfaces.FactorySnapshot) (*EmbeddedRuntimeConfig, error) {
	generated, err := generatedFactoryFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return RuntimeConfigFromGeneratedFactory(generated)
}

func generatedFactoryFromSnapshot(snapshot *interfaces.FactorySnapshot) (factoryapi.Factory, error) {
	if snapshot == nil {
		return factoryapi.Factory{}, errors.New("replay artifact factory is required")
	}
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("decode replay artifact factory: %w", err)
	}
	return generated, nil
}

func validatedFactorySnapshotFromOpenAPIJSON(data []byte) (*interfaces.FactorySnapshot, error) {
	generated, err := config.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return interfaces.NewFactorySnapshot(generated)
}

func generatedFactoryAPIFromConfig(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	return config.FactoryConfigToOpenAPI(cfg)
}

func generatedWorkstationAPIFromConfig(name string, cfg interfaces.FactoryWorkstationConfig, workerType string) factoryapi.Workstation {
	workstation := config.WorkstationConfigToOpenAPIWithWorkerType(cfg, workerType)
	if workstation.Name == "" {
		workstation.Name = name
	}
	return workstation
}

func generatedWorkerAPIFromConfig(name string, cfg workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) factoryapi.Worker {
	worker := config.WorkerConfigToOpenAPIWithFactoryUsage(cfg, workstations)
	if worker.Name == "" {
		worker.Name = name
	}
	return worker
}

func mergeGeneratedWorkers(factory *factoryapi.Factory, runtimeWorkers map[string]workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) error {
	if len(runtimeWorkers) == 0 {
		return nil
	}
	workers, err := mergeGeneratedEntries(
		generatedWorkerSlice(factory.Workers), generatedWorkerIndexes, sortedWorkerNames(runtimeWorkers),
		func(name string) (factoryapi.Worker, error) {
			return generatedWorkerFromReplayConfig(name, runtimeWorkers[name], workstations)
		},
		func(worker factoryapi.Worker) string { return worker.Name },
	)
	if err != nil {
		return err
	}
	factory.Workers = slicePtr(workers)
	return nil
}

func generatedWorkerFromReplayConfig(name string, worker workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) (factoryapi.Worker, error) {
	generated := generatedWorkerAPIFromConfig(name, worker, workstations)
	if generated.Name == "" {
		generated.Name = name
	}
	return generated, nil
}

func mergeGeneratedWorkstations(factory *factoryapi.Factory, workstationsByName map[string]interfaces.FactoryWorkstationConfig, runtimeWorkers map[string]workerconfig.Config) error {
	if len(workstationsByName) == 0 {
		return nil
	}
	workstations, err := mergeGeneratedEntries(
		generatedWorkstationSlice(factory.Workstations), generatedWorkstationIndexes, sortedWorkstationNames(workstationsByName),
		func(name string) (factoryapi.Workstation, error) {
			return generatedWorkstationFromReplayConfig(name, workstationsByName[name], runtimeWorkers)
		},
		func(workstation factoryapi.Workstation) string { return workstation.Name },
	)
	if err != nil {
		return err
	}
	factory.Workstations = slicePtr(workstations)
	return nil
}

func mergeGeneratedEntries[T any](generated []T, indexes func([]T) map[string]int, sortedNames []string, build func(string) (T, error), name func(T) string) ([]T, error) {
	byName := indexes(generated)
	for _, entryName := range sortedNames {
		entry, err := build(entryName)
		if err != nil {
			return nil, err
		}
		if index, ok := byName[name(entry)]; ok {
			generated[index] = entry
			continue
		}
		byName[name(entry)] = len(generated)
		generated = append(generated, entry)
	}
	return generated, nil
}

func generatedWorkstationFromReplayConfig(name string, cfg interfaces.FactoryWorkstationConfig, runtimeWorkers map[string]workerconfig.Config) (factoryapi.Workstation, error) {
	workerType := ""
	if worker, ok := runtimeWorkers[cfg.WorkerTypeName]; ok {
		workerType = worker.Type
	}
	generated := generatedWorkstationAPIFromConfig(name, cfg, workerType)
	preserveGeneratedWorkstationResources(cfg.Resources, &generated)
	if generated.Name == "" {
		generated.Name = name
	}
	if generated.Inputs == nil {
		generated.Inputs = []factoryapi.WorkstationIO{}
	}
	if generated.Outputs == nil {
		generated.Outputs = &[]factoryapi.WorkstationIO{}
	}
	return generated, nil
}

func preserveGeneratedWorkstationResources(resources []factoryresource.Config, target *factoryapi.Workstation) {
	if len(resources) == 0 || target == nil {
		return
	}
	usage := make([]factoryapi.ResourceRequirement, 0, len(resources))
	for _, resource := range resources {
		usage = append(usage, factoryapi.ResourceRequirement{Name: resource.Name, Capacity: resource.Capacity})
	}
	target.Resources = slicePtr(usage)
}

func generatedWorkerSlice(workers *[]factoryapi.Worker) []factoryapi.Worker {
	if workers == nil {
		return nil
	}
	return append([]factoryapi.Worker(nil), (*workers)...)
}

func generatedWorkerIndexes(workers []factoryapi.Worker) map[string]int {
	indexes := make(map[string]int, len(workers))
	for i, worker := range workers {
		indexes[worker.Name] = i
	}
	return indexes
}

func sortedWorkerNames(workers map[string]workerconfig.Config) []string {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func generatedWorkstationSlice(workstations *[]factoryapi.Workstation) []factoryapi.Workstation {
	if workstations == nil {
		return nil
	}
	return append([]factoryapi.Workstation(nil), (*workstations)...)
}

func generatedWorkstationIndexes(workstations []factoryapi.Workstation) map[string]int {
	indexes := make(map[string]int, len(workstations))
	for i, workstation := range workstations {
		indexes[workstation.Name] = i
	}
	return indexes
}

func sortedWorkstationNames(workstations map[string]interfaces.FactoryWorkstationConfig) []string {
	names := make([]string, 0, len(workstations))
	for name := range workstations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func preserveGeneratedResourceUsage(source *interfaces.FactoryConfig, target *factoryapi.Factory) {
	if source == nil || target == nil || target.Workstations == nil {
		return
	}
	byName := make(map[string][]factoryresource.Config, len(source.Workstations))
	for _, workstation := range source.Workstations {
		byName[workstation.Name] = workstation.Resources
	}
	for i := range *target.Workstations {
		resources := byName[(*target.Workstations)[i].Name]
		if len(resources) == 0 {
			continue
		}
		usage := make([]factoryapi.ResourceRequirement, 0, len(resources))
		for _, resource := range resources {
			usage = append(usage, factoryapi.ResourceRequirement{Name: resource.Name, Capacity: resource.Capacity})
		}
		(*target.Workstations)[i].Resources = slicePtr(usage)
	}
}

func restoreReplayResourceUsage(source factoryapi.Factory, target *interfaces.FactoryConfig) {
	if target == nil || source.Workstations == nil {
		return
	}
	byName := make(map[string][]factoryresource.Config, len(*source.Workstations))
	for _, workstation := range *source.Workstations {
		if workstation.Resources == nil {
			continue
		}
		resources := make([]factoryresource.Config, 0, len(*workstation.Resources))
		for _, usage := range *workstation.Resources {
			resources = append(resources, factoryresource.Config{Name: usage.Name, Capacity: usage.Capacity})
		}
		byName[workstation.Name] = resources
	}
	for i := range target.Workstations {
		if resources := byName[target.Workstations[i].Name]; len(resources) > 0 {
			target.Workstations[i].Resources = resources
		}
	}
}

func workstationConfigsFromMap(workstations map[string]interfaces.FactoryWorkstationConfig) []interfaces.FactoryWorkstationConfig {
	if len(workstations) == 0 {
		return nil
	}
	names := make([]string, 0, len(workstations))
	for name := range workstations {
		names = append(names, name)
	}
	sort.Strings(names)
	configs := make([]interfaces.FactoryWorkstationConfig, 0, len(names))
	for _, name := range names {
		configs = append(configs, workstations[name])
	}
	return configs
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

func runtimeWorkersByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]workerconfig.Config {
	workers := make(map[string]workerconfig.Config)
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
	workers map[string]workerconfig.Config,
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
		Workers      map[string]workerconfig.Config                 `json:"workers,omitempty"`
		Workstations map[string]interfaces.FactoryWorkstationConfig `json:"workstations,omitempty"`
	}{
		Factory:      factoryWithRuntime,
		Workers:      workers,
		Workstations: workstations,
	})
	return out
}

func generatedStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(cloneStringMap(values))
	return &converted
}
