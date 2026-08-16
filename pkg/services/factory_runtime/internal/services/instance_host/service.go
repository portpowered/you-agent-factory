// Package instance_host defines the parent-private Factory Runtime instance
// host that owns hosted-instance execute/pause/resume/replace/terminate
// lifecycle for one Runtime instance.
package instance_host

import (
	"context"
	"errors"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ErrInvalidDependencies classifies instance-host construction failures.
var ErrInvalidDependencies = errors.New("factory runtime instance host dependencies are invalid")

// Dependencies are fixed when Factory Runtime composes the parent-private
// instance host. They never cross the peer-facing Runtime root boundary.
type Dependencies struct {
	Clock factoryruntime.Clock
}

// ReplaceRequest configures hosted-runtime replacement through instance_host.
type ReplaceRequest struct {
	ReadinessContext            context.Context
	ServiceContext              context.Context
	Current                     factoryruntime.RuntimeRun
	Replacement                 factoryruntime.RuntimeRecord
	AttachSidecars              func(context.Context, factoryruntime.RuntimeRun) error
	AttachSidecarsInServiceMode bool
}

// Service owns hosted Runtime instance lifecycle. Only the Factory Runtime
// implementation consumes this parent-private contract.
type Service interface {
	factoryruntime.RuntimeLifecycle
	Pause(context.Context, factoryruntime.RuntimeRun) (factoryruntime.PauseResult, error)
	Resume(context.Context, factoryruntime.RuntimeRun) (factoryruntime.ResumeResult, error)
	Replace(ReplaceRequest) (factoryruntime.RuntimeRun, error)
}
