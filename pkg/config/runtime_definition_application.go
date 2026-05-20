package config

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func loadRuntimeDefinitionLookupMapsFromFactoryConfig(factoryDir string, cfg *interfaces.FactoryConfig, opts InlineRuntimeDefinitionOptions) (*runtimeDefinitionLookupMaps, error) {
	if cfg == nil {
		return newRuntimeDefinitionLookupMaps(0, 0), nil
	}

	runtimeDefs := newRuntimeDefinitionLookupMaps(len(cfg.Workers), len(cfg.Workstations))

	for _, workstation := range cfg.Workstations {
		def, err := runtimeWorkstationDefinition(factoryDir, workstation, opts.RequireSplitDefinitions, opts.WorkstationLoader)
		if err != nil {
			return nil, fmt.Errorf("load workstation %q config: %w", workstation.Name, err)
		}
		if def != nil {
			runtimeDefs.workstations[workstation.Name] = def
		}
	}

	for _, worker := range cfg.Workers {
		def, err := runtimeWorkerDefinition(factoryDir, worker, opts.RequireSplitDefinitions)
		if err != nil {
			return nil, fmt.Errorf("load worker %q config: %w", worker.Name, err)
		}
		if def != nil {
			runtimeDefs.workers[worker.Name] = def
		}
	}

	return runtimeDefs, nil
}

// applyRuntimeDefinitionsToClonedFactoryConfig mutates a cloned factory config
// in place so runtime worker and workstation definitions follow one merge path.
func applyRuntimeDefinitionsToClonedFactoryConfig(cfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) error {
	if cfg == nil {
		return nil
	}

	for i := range cfg.Workers {
		if runtimeCfg == nil {
			continue
		}
		def, ok := runtimeCfg.Worker(cfg.Workers[i].Name)
		if !ok || def == nil {
			continue
		}
		applyWorkerRuntimeDefinition(&cfg.Workers[i], def)
	}

	for i := range cfg.Workstations {
		normalizeCanonicalWorkstationRuntime(&cfg.Workstations[i])
		if runtimeCfg == nil {
			continue
		}
		def, ok := runtimeCfg.Workstation(cfg.Workstations[i].Name)
		if !ok || def == nil {
			continue
		}
		if err := applyWorkstationRuntimeDefinition(&cfg.Workstations[i], def); err != nil {
			return fmt.Errorf("normalize workstation %q config: %w", cfg.Workstations[i].Name, err)
		}
	}

	return nil
}
