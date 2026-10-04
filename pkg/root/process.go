package root

import (
	"context"
	"fmt"
	"reflect"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// BuildProcess constructs the reusable application process. Production passes
// an empty edge set; functional tests replace only their external boundaries.
// The policy-free network transport is the production default for the pinned
// model protocol, while caller-provided edges remain authoritative.
func BuildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (*initializerapplication.Process, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build application process: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	clock, scheduler, err := normalizeProcessTime(edges.Clock, edges.ProcessScheduler)
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	edges.Clock, edges.ProcessScheduler = clock, scheduler
	applicationProcess, err := wire.InjectBundle(ctx, serviceedges.Merge(
		serviceedges.Edges{ModelInvocationGRPCDialer: platformgrpc.NetworkDialer{}},
		edges,
	))
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	return applicationProcess, nil
}

// normalizeProcessTime selects process time once at the caller boundary.
// Legacy Now-only clocks control facts while a documented wall scheduler
// controls deadlines. Specialized owner overrides are left to their providers.
func normalizeProcessTime(
	clock platformclock.Source,
	scheduler platformclock.TimerSource,
) (platformclock.Source, platformclock.TimerSource, error) {
	if isTypedNilProcessTime(clock) {
		return nil, nil, fmt.Errorf("Clock must not be typed-nil; omit it to select the default")
	}
	if isTypedNilProcessTime(scheduler) {
		return nil, nil, fmt.Errorf("ProcessScheduler must not be typed-nil; omit it to select the default")
	}
	if clock == nil {
		clock = platformclock.Real{}
	}
	if scheduler == nil {
		if timerSource, ok := clock.(platformclock.TimerSource); ok {
			scheduler = timerSource
		} else {
			scheduler = platformclock.Real{}
		}
	}
	return clock, scheduler, nil
}

func isTypedNilProcessTime(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
