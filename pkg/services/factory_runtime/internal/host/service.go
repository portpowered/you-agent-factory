package host

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// LifecycleService is the host implementation of the public Factory Runtime
// lifecycle contract.
type LifecycleService struct {
	clock factory.Clock
}

func NewLifecycleService(clock factory.Clock) (*LifecycleService, error) {
	if clock == nil {
		return nil, fmt.Errorf("construct Factory Runtime lifecycle service: clock is required")
	}
	return &LifecycleService{clock: clock}, nil
}

func (*LifecycleService) Start(ctx context.Context, instance factory.HostedInstance) (factory.HostedHandle, error) {
	bundle, ok := instance.(*Bundle)
	if !ok || bundle == nil {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}
	return Start(ctx, bundle), nil
}

func (*LifecycleService) WaitForStart(ctx context.Context, handle factory.HostedHandle) error {
	concrete, ok := handle.(*Handle)
	if !ok || concrete == nil {
		return fmt.Errorf("factory runtime host requires a runtime handle")
	}
	return WaitForStart(ctx, concrete)
}

func (s *LifecycleService) Stop(handle factory.HostedHandle) error {
	concrete, ok := handle.(*Handle)
	if !ok || concrete == nil {
		if handle == nil {
			return nil
		}
		return fmt.Errorf("factory runtime host requires a runtime handle")
	}
	return Stop(concrete, s.clock)
}

func (*LifecycleService) StopSidecars(handle factory.HostedHandle) {
	concrete, _ := handle.(*Handle)
	StopSidecars(concrete)
}

func (s *LifecycleService) PublishReplacement(
	ctx context.Context,
	current factory.HostedHandle,
	replacement factory.HostedInstance,
) error {
	currentHandle, _ := current.(*Handle)
	replacementBundle, ok := replacement.(*Bundle)
	if !ok || replacementBundle == nil {
		return fmt.Errorf("factory runtime host requires a replacement runtime instance")
	}
	return PublishFactoryChange(ctx, currentHandle, replacementBundle, s.clock)
}

var _ factory.Lifecycle = (*LifecycleService)(nil)
