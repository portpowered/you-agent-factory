package factorydefinition_test

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func unavailableRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return factorydefinitions.UnimplementedService{}.ResolveRuntimeSnapshot(ctx, request)
}

func (*mcpDefinitionsRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}

func (*mcpCapturingCurrentFactoryRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}

func (*mcpCapturingInstallRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}
