// Package wire constructs the published Operator Settings root Service from the
// parent-private resolution subservice.
package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
)

// NewService constructs one inert Operator Settings root over the private
// resolution capability.
func NewService() (operatorsettings.Service, error) {
	resolutionService, err := resolutionwire.NewService()
	if err != nil {
		return nil, err
	}
	return operatorservice.New(resolutionService)
}
