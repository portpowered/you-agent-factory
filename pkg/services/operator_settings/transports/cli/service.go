// Package cli defines the Operator Settings service-owned CLI adapter.
package cli

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Service exposes Operator Settings CLI command operations to Cobra composition.
type Service interface {
	Configure(ConfigureConfig) error
	ResolveOperatorDefaults(ResolveOperatorDefaultsConfig) (operatorsettings.ResolvedDefaults, error)
}

type service struct {
	root operatorsettings.Service
}

// New constructs the Operator Settings CLI service from the accepted Settings
// root contract.
func New(root operatorsettings.Service) Service {
	if root == nil {
		return nil
	}
	return &service{root: root}
}
