package config

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
