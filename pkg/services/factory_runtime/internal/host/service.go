package host

import (
	"context"
	"fmt"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// LifecycleService is the host implementation of the public Factory Runtime
// lifecycle contract.
type LifecycleService struct {
	clock     factory.Clock
	scheduler platformclock.TimerSource
}

func NewLifecycleService(clock factory.Clock, scheduler platformclock.TimerSource) (*LifecycleService, error) {
	return &LifecycleService{clock: clock, scheduler: scheduler}, nil
}

func (*LifecycleService) Start(ctx context.Context, instance factory.RuntimeRecord) (factory.RuntimeRun, error) {
	bundle, ok := instance.(*Bundle)
	if !ok || bundle == nil {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}
	return Start(ctx, bundle), nil
}

func (s *LifecycleService) WaitForStart(ctx context.Context, handle factory.RuntimeRun) error {
	concrete, ok := handle.(*Handle)
	if !ok || concrete == nil {
		return fmt.Errorf("factory runtime host requires a runtime handle")
	}
	return WaitForStart(ctx, concrete, s.scheduler)
}

func (s *LifecycleService) Stop(handle factory.RuntimeRun) error {
	return s.StopWithClock(handle, s.clock)
}

// StopWithClock finalizes artifacts with the selected invocation fact clock.
func (s *LifecycleService) StopWithClock(handle factory.RuntimeRun, clock factory.Clock) error {
	if clock == nil {
		clock = s.clock
	}
	concrete, ok := handle.(*Handle)
	if !ok || concrete == nil {
		if handle == nil {
			return nil
		}
		return fmt.Errorf("factory runtime host requires a runtime handle")
	}
	return Stop(concrete, clock)
}

func (*LifecycleService) StopSidecars(handle factory.RuntimeRun) {
	concrete, _ := handle.(*Handle)
	StopSidecars(concrete)
}

func (s *LifecycleService) PublishReplacement(
	ctx context.Context,
	current factory.RuntimeRun,
	replacement factory.RuntimeRecord,
) error {
	return s.PublishReplacementWithClock(ctx, current, replacement, s.clock)
}

// PublishReplacementWithClock keeps replacement facts in the invocation scope.
func (s *LifecycleService) PublishReplacementWithClock(ctx context.Context, current factory.RuntimeRun, replacement factory.RuntimeRecord, clock factory.Clock) error {
	currentHandle, _ := current.(*Handle)
	replacementBundle, ok := replacement.(*Bundle)
	if !ok || replacementBundle == nil {
		return fmt.Errorf("factory runtime host requires a replacement runtime instance")
	}
	return PublishFactoryChange(ctx, currentHandle, replacementBundle, clock)
}

var _ factory.RuntimeLifecycle = (*LifecycleService)(nil)
