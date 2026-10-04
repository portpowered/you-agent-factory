package factoryruntime

import "m/pkg/services/factory_runtime/internal/orchestrators/petri"

type PetriMarking = petri.Marking // want "exact key `petri-public\\|pkg/services/factory_runtime\\|PetriMarking\\|.*petri.Marking`"
type Hook interface {             // want "exact key `petri-public\\|pkg/services/factory_runtime\\|Hook\\|.*petri.Marking`"
	Observe(petri.Marking)
}
