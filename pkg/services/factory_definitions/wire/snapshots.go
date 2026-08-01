package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// NewFactorySnapshotJSONDecoder binds the canonical public representation decoder
// to Factory Definitions snapshot capture.
func NewFactorySnapshotJSONDecoder() factoryeffect.FactorySnapshotJSONDecoder {
	return snapshotsportabilitywire.NewJSONDecoder(
		factorymapping.GeneratedFactoryFromOpenAPIJSON,
	)
}

// LoadedFactorySnapshotCapturer binds canonical snapshot representation
// mapping to the Factory Definitions capture implementation.
func LoadedFactorySnapshotCapturer() contracts.LoadedFactorySnapshotCapturer {
	return snapshotsportabilitywire.NewLoadedSnapshotCapturer(
		factorysnapshot.ObjectFromFactoryConfig,
		compilationwire.MergeRuntimeConfig,
	)
}

// FactorySnapshotCapturer binds canonical representation mapping to explicit
// Factory Definition snapshot capture.
func FactorySnapshotCapturer() contracts.FactorySnapshotCapturer {
	return snapshotsportabilitywire.NewExplicitSnapshotCapturer(
		factorysnapshot.ObjectFromFactoryConfig,
		compilationwire.MergeRuntimeConfig,
	)
}

// CaptureInitialSnapshot captures the portable Factory Definition stored with
// a newly created runtime recording.
func CaptureInitialSnapshot(
	loaded contracts.LoadedFactorySource,
	preparePortableFactoryConfig contracts.PortableFactoryConfigPreparer,
	captureFactorySnapshot contracts.FactorySnapshotCapturer,
) (*contracts.FactorySnapshot, error) {
	return factorydefinitionsinternal.CaptureInitialSnapshot(
		loaded,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
	)
}

// NewFactorySnapshotDirectoryLoader composes authored Factory loading and
// snapshot capture for Recordings import paths.
func NewFactorySnapshotDirectoryLoader(
	loader *compilationwire.Loader,
) factoryeffect.FactorySnapshotDirectoryLoader {
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
	return snapshotsportabilitywire.NewPortableFactoryConfigPreparer(
		contracts.CloneFactoryConfig,
		applySupportedFiles,
		applyStarterWork,
	)
}

// NewReplayRuntimeConfigDecoder binds replay lookup reconstruction to the
// canonical Factory representation decoder.
func NewReplayRuntimeConfigDecoder() factoryeffect.ReplayRuntimeConfigDecoder {
	return func(
		snapshot *contracts.FactorySnapshot,
	) (factoryeffect.ReplayRuntimeConfig, error) {
		return snapshotsportabilitywire.DecodeReplayRuntimeConfig(snapshot, factorymapping.FactoryConfigFromOpenAPIJSON)
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
	return snapshotsportabilitywire.ValidateEditableSnapshot(
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
	return snapshotsportabilitywire.PrepareFactorySnapshotImport(payload, NewFactorySnapshotJSONDecoder())
}

// NewPortableBundledFilesApplier binds portable authored-file discovery to an
// injected filesystem.
func NewPortableBundledFilesApplier(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledFilesApplier, error) {
	return snapshotsportabilitywire.NewPortableBundledFilesApplier(fileSystem)
}

// NewFactoryStarterWorkApplier binds starter-Work discovery to an injected
// filesystem.
func NewFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (contracts.FactoryStarterWorkApplier, error) {
	return snapshotsportabilitywire.NewFactoryStarterWorkApplier(fileSystem)
}

// NewPortableBundledDocsPruner binds obsolete authored-document cleanup to an
// injected filesystem.
func NewPortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledDocsPruner, error) {
	return snapshotsportabilitywire.NewPortableBundledDocsPruner(fileSystem)
}

// NewPortableBundledFilesMaterializer binds portable file writes to an injected
// filesystem.
func NewPortableBundledFilesMaterializer(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFilesMaterializer {
	return snapshotsportabilitywire.NewPortableBundledFilesMaterializer(fileSystem)
}

// NewPortableBundledFileWritesValidator binds portable write checks to an
// injected filesystem.
func NewPortableBundledFileWritesValidator(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFileWritesValidator {
	return snapshotsportabilitywire.NewPortableBundledFileWritesValidator(fileSystem)
}

// NewPortableBundledFilesCopier binds portable file copy to an injected
// filesystem.
func NewPortableBundledFilesCopier(
	fileSystem portablefiles.FileSystem,
) contracts.PortableBundledFilesCopier {
	return snapshotsportabilitywire.NewPortableBundledFilesCopier(fileSystem)
}

// NewPortableBundledFileSourceResolver binds portable source resolution to an
// injected filesystem.
func NewPortableBundledFileSourceResolver(
	fileSystem portablefiles.FileSystem,
) (contracts.PortableBundledFileSourceResolver, error) {
	return snapshotsportabilitywire.NewPortableBundledFileSourceResolver(fileSystem)
}
