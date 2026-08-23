package stress_test

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

func guardedLoopBreakerWorkstation(name, watchedWorkstation string, maxVisits int, source, target interfaces.IOConfig) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:    name,
		Type:    interfaces.WorkstationTypeLogical,
		Inputs:  []interfaces.IOConfig{source},
		Outputs: []interfaces.IOConfig{target},
		Guards: []interfaces.GuardConfig{{
			Type:        interfaces.GuardTypeVisitCount,
			Workstation: watchedWorkstation,
			MaxVisits:   maxVisits,
		}},
	}
}
