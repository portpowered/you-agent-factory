package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// ModelsRoot is the accepted Models root contract used by the HTTP adapter.
// Adapter-owned operations invoke this surface rather than Models internal
// packages.
type ModelsRoot = models.Service

// RootBinding binds the HTTP adapter to one injected Models root.
type RootBinding struct {
	Models  ModelsRoot
	Invoker workers.ModelInvoker
	Content work.ContentPreparation
	Scope   models.RuntimeScopeRef
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Models root. Tests inject a focused fake implementing ModelsRoot without
// constructing real catalog assemblers, asset caches, host supervisors, lease
// managers, inference runtimes, or service-local Wire graphs.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Handler {
	if binding.Invoker == nil {
		binding.Invoker = noopModelInvoker{}
	}
	if binding.Content == nil {
		binding.Content = noopContentPreparation{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	var scopes []models.RuntimeScopeRef
	if !binding.Scope.IsZero() {
		scopes = append(scopes, binding.Scope)
	}
	return NewHandler(
		NewAdapter(binding.Models, binding.Invoker, binding.Content, scopes...),
		logger,
	)
}

type noopModelInvoker struct{}

func (noopModelInvoker) InvokeModel(
	_ context.Context,
	_ string,
	_ models.Request,
) (models.Result, error) {
	return models.Result{}, models.ErrUnsupportedOperation
}

type noopContentPreparation struct{}

func (noopContentPreparation) PrepareWorkContent(
	_ context.Context,
	content []work.WorkContentPart,
) ([]work.WorkContentPart, error) {
	return content, nil
}
