package service

import (
	"context"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (o *Root) ListModels(context.Context) (models.List, error) {
	return models.List{}, missingDependencyError("Models runtime binding")
}

func (o *Root) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, missingDependencyError("Models runtime binding")
}

func (o *Root) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, missingDependencyError("Models runtime binding")
}

func (o *Root) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, missingDependencyError("Models runtime binding")
}

func (o *Root) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, missingDependencyError("Models runtime binding")
}

func (o *Root) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return missingDependencyError("Models runtime binding")
}
