package replay

import (
	"encoding/json"
	"fmt"
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Retained within the replay implementation for package-local compatibility
// while public metadata comparison is owned by the Recordings root.
const (
	metadataFactoryHash       = "factory_hash"
	metadataWorkersHash       = "workers_hash"
	metadataWorkstationsHash  = "workstations_hash"
	metadataRuntimeConfigHash = "runtime_config_hash"
)

func validatedFactorySnapshotFromJSON(
	data []byte,
	decode interfaces.FactorySnapshotJSONDecoder,
) (*interfaces.FactorySnapshot, error) {
	if decode == nil {
		return nil, fmt.Errorf("Factory snapshot decoder is required")
	}
	return decode(data)
}

func factorySnapshotFromJSON(data []byte) (*interfaces.FactorySnapshot, error) {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	snapshot, err := interfaces.NewFactorySnapshot(object)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func runtimeWorkersByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]interfaces.FactoryWorkerConfig {
	workers := make(map[string]interfaces.FactoryWorkerConfig)
	for _, workerCfg := range factoryCfg.Workers {
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if ok && def != nil {
			workers[workerCfg.Name] = interfaces.CloneWorkerConfig(*def)
		}
	}
	return workers
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

func sortedWorkerNames(workers map[string]interfaces.FactoryWorkerConfig) []string {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
