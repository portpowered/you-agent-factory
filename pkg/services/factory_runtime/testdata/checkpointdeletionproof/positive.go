package checkpointdeletionproof

import (
	"context"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func callExistingServiceMethod(ctx context.Context, svc factory.Service) {
	_, _ = svc.ControlPause(ctx, factory.PauseRequest{})
}
