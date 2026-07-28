// Package cli defines the System Bootstrap service-owned CLI adapter.
package cli

import (
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// Service exposes System Bootstrap CLI command operations to Cobra composition.
type Service interface {
	Initialize(InitializeConfig) (systeminitialization.Result, error)
}

type service struct {
	root systeminitialization.Service
}

// New constructs the System Bootstrap CLI service from the accepted Bootstrap
// root contract.
func New(root systeminitialization.Service) Service {
	if root == nil {
		return nil
	}
	return &service{root: root}
}
