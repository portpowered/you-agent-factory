package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimeconfig"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

// NewLoaded binds representation mapping and runtime-definition merging at the
// composition boundary. The returned operation consumes only Factory
// Definitions root contracts.
func NewLoaded(
	mapSnapshotObject snapshotscontracts.FactorySnapshotObjectMapper,
) snapshotscontracts.LoadedFactorySnapshotCapturer {
	return func(
		source snapshotscontracts.FactorySnapshotSource,
		sourceDirectory string,
		metadata map[string]string,
	) (*factorydefinitions.FactorySnapshot, error) {
		return CaptureLoaded(
			source,
			sourceDirectory,
			metadata,
			mapSnapshotObject,
		)
	}
}

// NewExplicit adapts explicit Factory Definition values to the snapshot
// capturer used by Factory Definitions persistence.
func NewExplicit(
	mapSnapshotObject snapshotscontracts.FactorySnapshotObjectMapper,
) snapshotscontracts.FactorySnapshotCapturer {
	captureLoaded := NewLoaded(mapSnapshotObject)
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		runtimeConfig snapshotscontracts.RuntimeDefinitionLookup,
		sourceDirectory string,
		metadata map[string]string,
	) (*factorydefinitions.FactorySnapshot, error) {
		return captureLoaded(
			NewExplicitSource(factoryDir, factoryConfig, runtimeConfig),
			sourceDirectory,
			metadata,
		)
	}
}

// NewExplicitSource adapts explicit Factory Definition values for loaded capture.
func NewExplicitSource(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig snapshotscontracts.RuntimeDefinitionLookup,
) snapshotscontracts.FactorySnapshotSource {
	return explicitFactorySnapshotSource{
		factoryDir:    factoryDir,
		factoryConfig: factoryConfig,
		runtimeConfig: runtimeConfig,
	}
}

// CaptureLoaded captures a portable snapshot without requiring the loaded
// source to know about transport representations.
func CaptureLoaded(
	source snapshotscontracts.FactorySnapshotSource,
	sourceDirectory string,
	metadata map[string]string,
	mapSnapshotObject snapshotscontracts.FactorySnapshotObjectMapper,
) (*factorydefinitions.FactorySnapshot, error) {
	if source == nil {
		return nil, errors.New("loaded factory config is required")
	}
	if mapSnapshotObject == nil {
		return nil, errors.New("Factory snapshot object mapper is required")
	}
	factoryConfig := source.FactoryConfig()
	if factoryConfig == nil {
		return nil, errors.New("factory config is required")
	}
	if strings.TrimSpace(sourceDirectory) == "" {
		sourceDirectory = source.FactoryDir()
	}

	workers := snapshotRuntimeWorkers(factoryConfig, source)
	workstations := snapshotRuntimeWorkstations(factoryConfig, source)
	factoryWithRuntime, err := runtimeconfig.Merge(factoryConfig, source)
	if err != nil {
		return nil, fmt.Errorf("inline runtime factory config: %w", err)
	}
	object, err := mapSnapshotObject(factoryWithRuntime)
	if err != nil {
		return nil, fmt.Errorf("encode factory snapshot: %w", err)
	}
	if factoryDir := source.FactoryDir(); factoryDir != "" {
		object["factoryDirectory"] = factoryDir
	}
	if sourceDirectory != "" {
		object["sourceDirectory"] = sourceDirectory
	}
	object["metadata"] = factorySnapshotMetadata(
		factoryWithRuntime,
		workers,
		workstations,
		metadata,
	)
	return factorydefinitions.NewFactorySnapshot(object)
}

type explicitFactorySnapshotSource struct {
	factoryDir    string
	factoryConfig *factorydefinitions.FactoryConfig
	runtimeConfig snapshotscontracts.RuntimeDefinitionLookup
}

func (s explicitFactorySnapshotSource) FactoryDir() string {
	return s.factoryDir
}

func (s explicitFactorySnapshotSource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return s.factoryConfig
}

func (s explicitFactorySnapshotSource) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if s.runtimeConfig == nil {
		return nil, false
	}
	return s.runtimeConfig.Worker(name)
}

func (s explicitFactorySnapshotSource) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if s.runtimeConfig == nil {
		return nil, false
	}
	return s.runtimeConfig.Workstation(name)
}

func snapshotRuntimeWorkers(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig snapshotscontracts.RuntimeDefinitionLookup,
) map[string]workerconfig.Config {
	workers := make(map[string]workerconfig.Config)
	for _, workerConfig := range factoryConfig.Workers {
		if definition, ok := runtimeConfig.Worker(workerConfig.Name); ok && definition != nil {
			workers[workerConfig.Name] = cloneSnapshotValue(*definition)
		}
	}
	return workers
}

func snapshotRuntimeWorkstations(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig snapshotscontracts.RuntimeDefinitionLookup,
) map[string]factorydefinitions.FactoryWorkstationConfig {
	workstations := make(
		map[string]factorydefinitions.FactoryWorkstationConfig,
		len(factoryConfig.Workstations),
	)
	for _, workstationConfig := range factoryConfig.Workstations {
		definition, ok := runtimeConfig.Workstation(workstationConfig.Name)
		if !ok || definition == nil {
			workstations[workstationConfig.Name] = cloneSnapshotValue(workstationConfig)
			continue
		}
		workstations[workstationConfig.Name] = mergeSnapshotRuntimeWorkstation(
			workstationConfig,
			*definition,
		)
	}
	return workstations
}

func mergeSnapshotRuntimeWorkstation(
	base factorydefinitions.FactoryWorkstationConfig,
	runtime factorydefinitions.FactoryWorkstationConfig,
) factorydefinitions.FactoryWorkstationConfig {
	merged := cloneSnapshotValue(runtime)
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
		merged.Cron = cloneSnapshotPointer(base.Cron)
	}
	if len(merged.Inputs) == 0 {
		merged.Inputs = cloneSnapshotValue(base.Inputs)
	}
	if len(merged.Outputs) == 0 {
		merged.Outputs = cloneSnapshotValue(base.Outputs)
	}
	if merged.OnContinue == nil {
		merged.OnContinue = cloneSnapshotValue(base.OnContinue)
	}
	if merged.OnRejection == nil {
		merged.OnRejection = cloneSnapshotValue(base.OnRejection)
	}
	if merged.OnFailure == nil {
		merged.OnFailure = cloneSnapshotValue(base.OnFailure)
	}
	if len(merged.Resources) == 0 {
		merged.Resources = cloneSnapshotValue(base.Resources)
	}
	if len(merged.Guards) == 0 {
		merged.Guards = cloneSnapshotValue(base.Guards)
	}
	return cloneSnapshotValue(merged)
}

func factorySnapshotMetadata(
	factoryWithRuntime *factorydefinitions.FactoryConfig,
	workers map[string]workerconfig.Config,
	workstations map[string]factorydefinitions.FactoryWorkstationConfig,
	metadata map[string]string,
) map[string]string {
	result := cloneSnapshotStringMap(metadata)
	if result == nil {
		result = make(map[string]string)
	}
	result["source_format"] = factorydefinitions.ReplayV1SourceFormat
	result["factory_hash"] = snapshotSHA256(factoryWithRuntime)
	result["workers_hash"] = snapshotSHA256(workers)
	result["workstations_hash"] = snapshotSHA256(workstations)
	result["runtime_config_hash"] = snapshotSHA256(struct {
		Factory      *factorydefinitions.FactoryConfig                      `json:"factory"`
		Workers      map[string]workerconfig.Config                         `json:"workers,omitempty"`
		Workstations map[string]factorydefinitions.FactoryWorkstationConfig `json:"workstations,omitempty"`
	}{factoryWithRuntime, workers, workstations})
	return result
}

func snapshotSHA256(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "sha256:error"
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:]))
}

func cloneSnapshotValue[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var clone T
	if err := json.Unmarshal(data, &clone); err != nil {
		return value
	}
	return clone
}

func cloneSnapshotPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := cloneSnapshotValue(*value)
	return &cloned
}

func cloneSnapshotStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
