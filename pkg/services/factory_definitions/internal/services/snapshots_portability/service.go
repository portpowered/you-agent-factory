// Package snapshots_portability defines the parent-private Factory Definitions
// capture/import/materialize capability behind the published CTR-DEF root
// snapshot slice. Cross-service peers must use the Factory Definitions root
// Service rather than importing this private subservice.
//
// The public surface exposes only CTR-DEF snapshot/portability vocabulary. It
// does not declare Runtime/Petri types, peer service implementations, or
// Wire/root construction ownership, and it does not take catalog,
// authoring_layout, compilation, validation, or distribution leases. External
// effects required for capture/import/materialize are accepted only as exact
// injected ports at construction time; today detached CTR-DEF snapshot ops
// need none, so wire.NewService takes no peer/Wire collaborators.
package snapshots_portability

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns detached Factory snapshot capture, import/prepare, and
// materialize using the CTR-DEF root request/result vocabulary.
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
