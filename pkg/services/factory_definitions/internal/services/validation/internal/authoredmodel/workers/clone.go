package workerconfig

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

func Clone(def Config) Config { return factorycontracts.Clone(def) }
