// Package http owns HTTP adaptation for Factory Visualization operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Visualization remain here with the owning service.
package http

import (
	"go.uber.org/zap"
)

// Adapter owns the generated HTTP operation implementations for Factory
// Visualization resources.
type Adapter struct {
	visualization VisualizationRoot
	logger        *zap.Logger
}

// Dependencies are the exact injected roles used by the Factory Visualization HTTP
// adapter. They are supplied by composition or focused fake-root tests.
type Dependencies struct {
	VisualizationRoot VisualizationRoot
}

// NewHandler constructs an inert Factory Visualization HTTP adapter.
func NewHandler(deps Dependencies, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		visualization: deps.VisualizationRoot,
		logger:        logger,
	}
}

// Handler is retained as a public alias while handler files stay aligned with
// established transport naming.
type Handler = Adapter

// Server is retained as a private receiver alias while handler files are kept
// mechanically identical to the established public behavior.
type Server = Adapter
