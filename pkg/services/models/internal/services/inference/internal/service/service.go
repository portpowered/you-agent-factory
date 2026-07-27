package service

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	scopes      runtimescopes.Service
	catalog     modelcatalog.Service
	runtimeHost runtimehost.Service
	clock       func() time.Time
}

var _ inference.Service = (*service)(nil)

// New constructs an inert Inference owner that validates and retains injected
// effects without launching subprocesses, opening listeners, or starting
// application lifecycle.
func New(
	scopes runtimescopes.Service,
	catalog modelcatalog.Service,
	runtimeHost runtimehost.Service,
	clock func() time.Time,
) inference.Service {
	return &service{
		scopes:      scopes,
		catalog:     catalog,
		runtimeHost: runtimeHost,
		clock:       clock,
	}
}

func (s *service) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if s == nil {
		return models.InvokeModelResult{}, models.ErrUnsupportedOperation
	}
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (s *service) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	if s == nil {
		return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
	}
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}
