package wire

import (
	"context"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilityeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	snapshotsportabilityreplayconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// FactorySnapshotJSONDecoder binds the canonical public representation decoder
// to Factory Definitions snapshot capture.
func FactorySnapshotJSONDecoder() contracts.FactorySnapshotJSONDecoder {
	return snapshotsportabilitycapture.NewJSONDecoder(
		factorymapping.GeneratedFactoryFromOpenAPIJSON,
	)
}

// LoadedFactorySnapshotCapturer binds canonical snapshot representation
// mapping to the Factory Definitions capture implementation.
func LoadedFactorySnapshotCapturer() contracts.LoadedFactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewLoaded(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotCapturer binds canonical representation mapping to explicit
// Factory Definition snapshot capture.
func FactorySnapshotCapturer() contracts.FactorySnapshotCapturer {
	return snapshotsportabilitycapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)
}

// FactorySnapshotDirectoryLoader composes authored Factory loading and
// snapshot capture for Recordings import paths.
func FactorySnapshotDirectoryLoader(
	loader *compilationloading.Loader,
) contracts.FactorySnapshotDirectoryLoader {
	captureLoaded := LoadedFactorySnapshotCapturer()
	return func(factoryDir string) (*contracts.FactorySnapshot, error) {
		loaded, err := loader.LoadSourceFromFactoryDir(factoryDir, nil)
		if err != nil {
			return nil, err
		}
		return captureLoaded(loaded, loaded.FactoryDir(), nil)
	}
}

// PortableFactoryConfigPreparer binds the canonical Factory Definition
// authored-file operations to portable preparation.
func PortableFactoryConfigPreparer(
	applySupportedFiles contracts.PortableBundledFilesApplier,
	applyStarterWork contracts.FactoryStarterWorkApplier,
) contracts.PortableFactoryConfigPreparer {
	return snapshotsportabilityprepare.NewPreparer(
		contracts.CloneFactoryConfig,
		applySupportedFiles,
		applyStarterWork,
	)
}

// ReplayRuntimeConfigDecoder binds replay lookup reconstruction to the
// canonical Factory representation decoder.
func ReplayRuntimeConfigDecoder() contracts.ReplayRuntimeConfigDecoder {
	return func(
		snapshot *contracts.FactorySnapshot,
	) (contracts.ReplayRuntimeConfig, error) {
		return snapshotsportabilityreplayconfig.Decode(snapshot, factorymapping.FactoryConfigFromOpenAPIJSON)
	}
}

// ValidateEditableSnapshot applies detached editable-snapshot validation through
// snapshots_portability-owned implementation.
func ValidateEditableSnapshot(
	ctx context.Context,
	snapshot *contracts.FactorySnapshot,
	workstationLoader contracts.WorkstationLoader,
	mapInput contracts.EditableFactoryValidationRequestMapper,
	validator contracts.DefinitionValidationOperation,
) error {
	return snapshotsportabilityeditable.ValidateSnapshot(
		ctx,
		snapshot,
		workstationLoader,
		mapInput,
		validator,
	)
}

// PrepareFactorySnapshotImport decodes one detached snapshot payload through
// snapshots_portability-owned prepare-import logic.
func PrepareFactorySnapshotImport(
	payload []byte,
) (contracts.PrepareFactorySnapshotImportResult, error) {
	return snapshotsportabilityprepare.Import(payload, FactorySnapshotJSONDecoder())
}

// NewPortableBundledFilesApplier binds portable authored-file discovery to an
// injected filesystem.
func NewPortableBundledFilesApplier(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledFilesApplier, error) {
	return internalportableconfig.NewPortableBundledFilesApplier(fileSystem)
}

// NewFactoryStarterWorkApplier binds starter-Work discovery to an injected
// filesystem.
func NewFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (contracts.FactoryStarterWorkApplier, error) {
	return internalportableconfig.NewFactoryStarterWorkApplier(fileSystem)
}

// NewPortableBundledDocsPruner binds obsolete authored-document cleanup to an
// injected filesystem.
func NewPortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledDocsPruner, error) {
	return internalportableconfig.NewPortableBundledDocsPruner(fileSystem)
}

// NewPortableBundledFilesMaterializer binds portable file writes to an injected
// filesystem.
func NewPortableBundledFilesMaterializer(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFilesMaterializer {
	return internalportableconfig.NewMaterializer(fileSystem)
}

// NewPortableBundledFileWritesValidator binds portable write checks to an
// injected filesystem.
func NewPortableBundledFileWritesValidator(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFileWritesValidator {
	return internalportableconfig.NewWritesValidator(fileSystem)
}

// NewPortableBundledFilesCopier binds portable file copy to an injected
// filesystem.
func NewPortableBundledFilesCopier(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFilesCopier {
	return internalportableconfig.NewFilesCopier(fileSystem)
}

// NewPortableBundledFileSourceResolver binds portable source resolution to an
// injected filesystem.
func NewPortableBundledFileSourceResolver(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledFileSourceResolver, error) {
	return internalportableconfig.NewSupportedSourceResolver(fileSystem)
}
