package state

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

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

func runtimeWorkstationKind(
	name string,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) interfaces.WorkstationKind {
	workstation, ok := runtimeWorkstation(name, runtimeConfig)
	if !ok {
		return ""
	}
	return workstation.Kind
}
