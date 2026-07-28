// Package http owns HTTP adaptation for Factory Definitions operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Definitions remain here with the owning service.
package http

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// Adapter owns the generated HTTP operation implementations for Factory
// Definitions resources.
type Adapter struct {
	definitionsRoot factorydefinitions.Service
	validation      factorydefinitions.SubmittedDefinitionValidationOperation
	logger          *zap.Logger
}

// Dependencies are the exact injected roles used by the Factory Definitions HTTP
// adapter. They are supplied by composition or focused fake-root tests.
type Dependencies struct {
	DefinitionsRoot factorydefinitions.Service
	Validation      factorydefinitions.SubmittedDefinitionValidationOperation
}

// NewHandler constructs an inert Factory Definitions HTTP adapter.
func NewHandler(deps Dependencies, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		definitionsRoot: deps.DefinitionsRoot,
		validation:      deps.Validation,
		logger:          logger,
	}
}

// Handler is retained as a public alias while handler files stay aligned with
// established transport naming.
type Handler = Adapter

// Server is retained as a private receiver alias while handler files are kept
// mechanically identical to the established public behavior.
type Server = Adapter
