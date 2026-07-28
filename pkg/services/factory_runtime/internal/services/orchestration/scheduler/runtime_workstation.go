package scheduler

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

func runtimeWorkstationKind(
	name string,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) interfaces.WorkstationKind {
	if runtimeConfig == nil || name == "" {
		return ""
	}
	workstation, ok := runtimeConfig.Workstation(name)
	if !ok || workstation == nil {
		return ""
	}
	return workstation.Kind
}
