package factorydefinitions_test

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (fakeDefinitionsPeer) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return factorydefinitions.UnimplementedService{}.ResolveRuntimeSnapshot(ctx, request)
}
