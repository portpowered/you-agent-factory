package petriconsumer

import runtime "m/pkg/services/factory_runtime"

type Indirect = runtime.PetriMarking // want "exact key `petri-public\\|pkg/petriconsumer\\|Indirect\\|.*petri.Marking`" "petri-reference.*PetriMarking::count=1"
type Interface = runtime.Hook        // want "exact key `petri-public\\|pkg/petriconsumer\\|Interface\\|.*petri.Marking`"
