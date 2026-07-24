// Package service implements the Factory Definitions compilation nested
// subservice behind compilation.Service.
package service

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
)

// Service is the private compilation implementation. Later IMP-DEF-03 stories
// relocate normalize-and-compile behavior here; the shell returns the published
// typed invalid-source failure until that ownership lands.
type Service struct{}

var _ compilation.Service = (*Service)(nil)

// New constructs the inert compilation subservice implementation.
func New() *Service {
	return &Service{}
}

// CompileEffectiveFactorySource returns ErrInvalidAuthoredFactorySource until
// equivalent-input identity and typed failure ownership land in later stories.
func (Service) CompileEffectiveFactorySource(
	context.Context,
	factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrInvalidAuthoredFactorySource
}
