// Package snapshots_portability defines the Factory Definitions-owned private
// snapshot portability capability for detached Factory snapshot capture,
// prepare-import, and bundled-asset materialize behind the CTR-DEF root
// snapshot slice.
//
// Consumers outside Factory Definitions use the outer Factory Definitions root
// Service instead of this private subservice contract.
//
// The public surface exposes only CTR-DEF snapshot vocabulary and exact
// injected host-effect ports. It does not declare Runtime/Recordings types,
// peer service implementations, Wire/root construction ownership, filesystem
// effect concrete types, or sibling catalog/authoring_layout/compilation/
// validation/distribution APIs.
package snapshotsportability

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns detached Factory snapshot capture, prepare-import, and
// materialize behind the CTR-DEF root snapshot slice.
type Service interface {
	CaptureFactorySnapshot(
		context.Context,
		factorydefinitions.CaptureFactorySnapshotRequest,
	) (factorydefinitions.CaptureFactorySnapshotResult, error)
	PrepareFactorySnapshotImport(
		context.Context,
		factorydefinitions.PrepareFactorySnapshotImportRequest,
	) (factorydefinitions.PrepareFactorySnapshotImportResult, error)
	MaterializeFactorySnapshot(
		context.Context,
		factorydefinitions.MaterializeFactorySnapshotRequest,
	) (factorydefinitions.MaterializeFactorySnapshotResult, error)
}
