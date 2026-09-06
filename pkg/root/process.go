package root

import (
	"context"
	"fmt"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
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
	applicationProcess, err := wire.InjectBundle(ctx, serviceedges.Merge(
		serviceedges.Edges{ModelInvocationGRPCDialer: platformgrpc.NetworkDialer{}},
		edges,
	))
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	return applicationProcess, nil
}
