package internal

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func (service *boundRuntimeService) CleanInvocationSnapshot(ctx context.Context) (factoryruntime.CleanInvocationSnapshot, error) {
	target, ok := service.target().(factoryruntime.CleanInvocationSnapshotProvider)
	if !ok {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return target.CleanInvocationSnapshot(ctx)
}
