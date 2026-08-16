package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution"
)

// ExecuteWithModelRuntimeScope keeps direct managed-model invocation on the
// normal Execute path while supplying the opened Models scope to the private
// inference runner for this attempt only.
func (s *Service) ExecuteWithModelRuntimeScope(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	worker models.LocalWorker,
	resources []models.LocalResource,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	return s.Execute(workerexecution.WithModelRuntimeProjection(
		ctx, scope, worker, resources,
	), request)
}
