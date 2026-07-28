// Package agent defines the Workers-parent-private Agent Runner service.
package agent

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const Identity = "agent"

// Service executes one normalized agent attempt through the common Runner
// contract. The parent Runners registry is its only production consumer.
type Service interface {
	Execute(
		context.Context,
		workers.RunnerExecutionRequest,
	) (workers.RunnerExecutionResult, error)
}
