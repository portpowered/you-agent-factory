package petriconsumer

import runtime "m/pkg/services/factory_runtime"

type Indirect = runtime.PetriMarking // want "exact key `petri-public\\|pkg/petriconsumer\\|Indirect\\|.*petri.Marking`"
type Interface = runtime.Hook        // want "exact key `petri-public\\|pkg/petriconsumer\\|Interface\\|.*petri.Marking`"
