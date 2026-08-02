package http

import (
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

// RootBinding binds the HTTP adapter to one injected Models root.
type RootBinding struct {
	Models models.Service
	Scope  models.RuntimeScopeRef
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Models root. Tests inject a focused fake implementing models.Service without
// constructing real catalog assemblers, asset caches, host supervisors, lease
// managers, inference runtimes, or service-local Wire graphs.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	var scopes []models.RuntimeScopeRef
	if !binding.Scope.IsZero() {
		scopes = append(scopes, binding.Scope)
	}
	return NewHandler(
		NewAdapter(binding.Models, scopes...),
		logger,
	)
}
