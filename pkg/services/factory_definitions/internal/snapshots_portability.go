package internal

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
)

type snapshotsPortabilityService struct {
	factorydefinitions.Service
	snapshots snapshotsportability.Service
}

// AttachSnapshotsPortability returns the Factory Definitions service with
// detached snapshot capture, prepare-import, and materialize delegated to the
// nested snapshots_portability owner while preserving every other root operation.
func AttachSnapshotsPortability(
	service factorydefinitions.Service,
	snapshots snapshotsportability.Service,
) (factorydefinitions.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Definitions service is required")
	}
	if snapshots == nil {
		return nil, fmt.Errorf("snapshots portability subservice is required")
	}
	return snapshotsPortabilityService{Service: service, snapshots: snapshots}, nil
}

func (s snapshotsPortabilityService) CaptureFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	return s.snapshots.CaptureFactorySnapshot(ctx, request)
}

func (s snapshotsPortabilityService) PrepareFactorySnapshotImport(
	ctx context.Context,
	request factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	return s.snapshots.PrepareFactorySnapshotImport(ctx, request)
}

func (s snapshotsPortabilityService) MaterializeFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	return s.snapshots.MaterializeFactorySnapshot(ctx, request)
}
