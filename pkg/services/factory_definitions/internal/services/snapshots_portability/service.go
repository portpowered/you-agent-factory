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
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

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

// Dependencies are the exact collaborator ports required by snapshots_portability.
// They are supplied by Factory Definitions composition and never selected here:
// snapshots_portability does not choose host filesystem adapters, boundary
// codecs, or Wire/root constructors.
type Dependencies struct {
	LoadCanonical             factorydefinitions.CanonicalFactoryJSONLoader
	CaptureLoaded             factorydefinitions.LoadedFactorySnapshotCapturer
	PreparePortable           factorydefinitions.PortableFactoryConfigPreparer
	DecodeSnapshot            factoryeffects.FactorySnapshotJSONDecoder
	MaterializePortableFiles  factorydefinitions.PortableBundledFilesMaterializer
	ValidateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator
}
