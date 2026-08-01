// Package http owns HTTP adaptation for Factory Visualization operations.
//
// The top-level HTTP transport registers the generated routes and composes this
// handler with adapters owned by other services. Request decoding, generated
// contract mapping, service invocation, error mapping, and streaming policy for
// Factory Visualization remain here with the owning service.
package http

import (
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/binding"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/lifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/observe"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/presentation"
	"go.uber.org/zap"
)

// Adapter owns the generated HTTP operation implementations for Factory
// Visualization resources.
type Adapter struct {
	rootBinding  *binding.Handler
	lifecycle    *lifecycle.Adapter
	observe      *observe.Adapter
	presentation *presentation.Adapter
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
	rootBinding := binding.New(deps.VisualizationRoot)
	return &Adapter{
		rootBinding:  rootBinding,
		lifecycle:    lifecycle.NewHandler(rootBinding, logger),
		observe:      observe.NewHandler(rootBinding, logger),
		presentation: presentation.NewHandler(rootBinding, logger),
	}
}

// Handler is retained as a public alias while handler files stay aligned with
// established transport naming.
type Handler = Adapter

// Server is retained as a private receiver alias while handler files are kept
// mechanically identical to the established public behavior.
type Server = Adapter

// Keep the owner-local HTTP request and response vocabulary available at the
// established package path while the implementations live in focused child
// packages.
type ActivateHTTPRequest = lifecycle.ActivateHTTPRequest
type LifecycleHTTPResponse = lifecycle.LifecycleHTTPResponse
type ObserveHTTPRequest = observe.ObserveHTTPRequest
type ObserveReconnectCursorHTTPRequest = observe.ObserveReconnectCursorHTTPRequest
type ObserveHTTPResponse = observe.ObserveHTTPResponse
type ObserveHTTPProjectedView = observe.ObserveHTTPProjectedView
type OpenPresentationHTTPRequest = presentation.OpenPresentationHTTPRequest
type OpenPresentationHTTPResponse = presentation.OpenPresentationHTTPResponse
type PresentProgressHTTPRequest = presentation.PresentProgressHTTPRequest
type ProgressRecordHTTPRequest = presentation.ProgressRecordHTTPRequest
type PresentProgressHTTPResponse = presentation.PresentProgressHTTPResponse
type FinalizePresentationHTTPRequest = presentation.FinalizePresentationHTTPRequest
type TerminalWriteHTTPRequest = presentation.TerminalWriteHTTPRequest
type FinalizePresentationHTTPResponse = presentation.FinalizePresentationHTTPResponse
type ClosePresentationHTTPRequest = presentation.ClosePresentationHTTPRequest
type ClosePresentationHTTPResponse = presentation.ClosePresentationHTTPResponse

var _ factoryvisualization.Root = (*binding.Handler)(nil)
