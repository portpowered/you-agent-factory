// Package service implements the Factory Definitions compilation nested
// subservice behind compilation.Service.
package service

import (
	"context"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
)

// Service is the private compilation implementation for authored/canonical →
// normalized effective-source compile under the CTR-DEF compilation slice.
type Service struct{}

var _ compilation.Service = (*Service)(nil)

// New constructs the compilation subservice implementation.
func New() *Service {
	return &Service{}
}

// CompileEffectiveFactorySource normalizes authored/canonical Factory source
// into one EffectiveFactorySource with deterministic ContentIdentity and
// detached FactoryDir / RuntimeBaseDir facts. Equivalent authored inputs that
// differ only by insignificant surrounding whitespace share the same identity.
func (Service) CompileEffectiveFactorySource(
	_ context.Context,
	request factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	canonical := strings.TrimSpace(string(request.Canonical))
	if canonical == "" {
		return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrInvalidAuthoredFactorySource
	}

	factoryDir := request.FactoryDir
	return factorydefinitions.CompileEffectiveFactorySourceResult{
		Effective: factorydefinitions.EffectiveFactorySource{
			FactoryDir:      factoryDir,
			RuntimeBaseDir:  factoryDir,
			ContentIdentity: canonical,
		},
	}, nil
}
