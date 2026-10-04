package petriallowed

import "m/pkg/services/factory_runtime/internal/orchestrators/petri"

type Allowed = petri.Marking

func Public(petri.Token) {}
