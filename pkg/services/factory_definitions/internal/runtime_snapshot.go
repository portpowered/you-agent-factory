package internal

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type runtimeSnapshotService struct {
	factorydefinitions.Service
	resolve factorydefinitions.RuntimeSnapshotOperation
}

// AttachRuntimeSnapshot attaches the owner-composed runtime snapshot
// operation while preserving the rest of the singular Definitions root.
func AttachRuntimeSnapshot(
	service factorydefinitions.Service,
	resolve factorydefinitions.RuntimeSnapshotOperation,
) (factorydefinitions.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Definitions service is required")
	}
	if resolve == nil {
		return nil, fmt.Errorf("runtime snapshot operation is required")
	}
	return runtimeSnapshotService{Service: service, resolve: resolve}, nil
}

func (s runtimeSnapshotService) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return s.resolve(ctx, request)
}
