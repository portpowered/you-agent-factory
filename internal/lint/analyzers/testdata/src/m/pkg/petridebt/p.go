package petridebt

import runtime "m/pkg/services/factory_runtime"

type Listed = runtime.PetriMarking
type Unlisted = runtime.PetriMarking // want "exact key `petri-public\\|pkg/petridebt\\|Unlisted\\|.*petri.Marking`"
