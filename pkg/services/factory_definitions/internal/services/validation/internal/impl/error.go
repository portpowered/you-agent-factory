package impl

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

type TopologyError = factorydefinitions.ValidationTopologyError

func NewTopologyError(message string, targets []Target) *TopologyError {
	return factorydefinitions.NewValidationTopologyError(message, targets)
}
