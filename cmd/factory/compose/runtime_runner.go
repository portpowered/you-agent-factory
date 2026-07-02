package compose

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

// InjectRuntimeRunner composes initializer transport for Run startup. Service-mode
// API hosting (Port > 0) uses InjectAPITransport; batch local runs use InjectCLITransport.
func InjectRuntimeRunner(ctx context.Context, cfg *initializer.Config) (initializer.LocalRuntimeRunner, error) {
	if cfg != nil && cfg.Port > 0 {
		transport, err := InjectAPITransport(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if transport == nil {
			return nil, errors.New("initializer API transport missing transport bundle")
		}
		return transport, nil
	}

	transport, err := InjectCLITransport(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, errors.New("initializer CLI transport missing transport bundle")
	}
	runner := transport.Runner()
	if runner == nil {
		return nil, errors.New("initializer CLI transport missing runtime runner")
	}
	return runner, nil
}
