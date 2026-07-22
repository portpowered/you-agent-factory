package subsystems

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

const defaultWorkstationMaxRetries = 3

func runtimeWorkstation(
	name string,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) (*interfaces.FactoryWorkstationConfig, bool) {
	if runtimeConfig == nil || name == "" {
		return nil, false
	}
	workstation, ok := runtimeConfig.Workstation(name)
	if !ok || workstation == nil {
		return nil, false
	}
	return workstation, true
}

func runtimeWorkstationMaxRetries(
	name string,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) int {
	workstation, ok := runtimeWorkstation(name, runtimeConfig)
	if !ok {
		return 0
	}
	if workstation.Limits.MaxRetries > 0 {
		return workstation.Limits.MaxRetries
	}
	return defaultWorkstationMaxRetries
}
