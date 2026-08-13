package root

import (
	"context"
	"fmt"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// BuildProcess constructs the reusable application process. Production passes
// an empty edge set; functional tests replace only their external boundaries.
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
	applicationProcess, err := wire.InjectBundle(ctx, edges)
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	return applicationProcess, nil
}

// BuildStatelessWorkers constructs the standalone Workers Execute root. It
// intentionally does not construct or open a Factory Runtime or Factory
// Session, so callers can submit one detached attempt directly.
func BuildStatelessWorkers(
	ctx context.Context,
	edges serviceedges.Edges,
) (workers.Service, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build stateless Workers: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build stateless Workers: %w", err)
	}
	service, err := wire.BuildStatelessWorkers(ctx, edges)
	if err != nil {
		return nil, fmt.Errorf("build stateless Workers: %w", err)
	}
	return service, nil
}
