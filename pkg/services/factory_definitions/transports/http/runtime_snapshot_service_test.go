package http_test

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

func (*httpDefinitionsRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}

func (*blockingCurrentFactoryRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}

func (*packagedFactoryCatalogRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}

func (*capturingCurrentFactoryRootFake) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return unavailableRuntimeSnapshot(ctx, request)
}
