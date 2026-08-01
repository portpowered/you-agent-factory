package wire

import (
	"context"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotscapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/capture"
	snapshotseditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/editable"
	snapshotsmaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/materialize"
	snapshotsportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/portableconfig"
	snapshotsprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/prepare"
	snapshotsreplayconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/replayconfig"
)

func NewJSONDecoder[T any](decode func([]byte) (T, error)) factoryeffects.FactorySnapshotJSONDecoder {
	return snapshotscapture.NewJSONDecoder(decode)
}

func NewLoadedSnapshotCapturer(
	mapObject factorydefinitions.FactorySnapshotObjectMapper,
	mergeRuntime ...func(*factorydefinitions.FactoryConfig, factorydefinitions.RuntimeDefinitionLookup) (*factorydefinitions.FactoryConfig, error),
) factorydefinitions.LoadedFactorySnapshotCapturer {
	return snapshotscapture.NewLoaded(mapObject, mergeRuntime...)
}

func NewExplicitSnapshotCapturer(
	mapObject factorydefinitions.FactorySnapshotObjectMapper,
	mergeRuntime ...func(*factorydefinitions.FactoryConfig, factorydefinitions.RuntimeDefinitionLookup) (*factorydefinitions.FactoryConfig, error),
) factorydefinitions.FactorySnapshotCapturer {
	return snapshotscapture.NewExplicit(mapObject, mergeRuntime...)
}

func NewPortableFactoryConfigPreparer(
	clone factorydefinitions.FactoryConfigCloner,
	applyFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
) factorydefinitions.PortableFactoryConfigPreparer {
	return snapshotsprepare.NewPreparer(clone, applyFiles, applyStarterWork)
}

func NewMaterializer(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesMaterializer {
	return snapshotsmaterialize.NewMaterializer(fileSystem)
}

func NewWritesValidator(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFileWritesValidator {
	return snapshotsmaterialize.NewWritesValidator(fileSystem)
}

func NewPortableBundledFilesApplier(fileSystem portablefiles.FileSystem) (factorydefinitions.PortableBundledFilesApplier, error) {
	return snapshotsportableconfig.NewPortableBundledFilesApplier(fileSystem)
}

func NewFactoryStarterWorkApplier(fileSystem portablefiles.FileSystem) (factorydefinitions.FactoryStarterWorkApplier, error) {
	return snapshotsportableconfig.NewFactoryStarterWorkApplier(fileSystem)
}

func NewPortableBundledDocsPruner(fileSystem portablefiles.FileSystem) (factorydefinitions.PortableBundledDocsPruner, error) {
	return snapshotsportableconfig.NewPortableBundledDocsPruner(fileSystem)
}

func NewPortableBundledFilesMaterializer(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesMaterializer {
	return snapshotsmaterialize.NewMaterializer(fileSystem)
}

func NewPortableBundledFileWritesValidator(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFileWritesValidator {
	return snapshotsmaterialize.NewWritesValidator(fileSystem)
}

func NewPortableBundledFilesCopier(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesCopier {
	return snapshotsportableconfig.NewFilesCopier(fileSystem)
}

func NewPortableBundledFileSourceResolver(fileSystem portablefiles.FileSystem) (factorydefinitions.PortableBundledFileSourceResolver, error) {
	return snapshotsportableconfig.NewSupportedSourceResolver(fileSystem)
}

func MaterializeFiles(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	config *factorydefinitions.FactoryConfig,
) ([]factorydefinitions.PortableBundledFileReplacement, error) {
	return snapshotsmaterialize.MaterializeFiles(fileSystem, targetDir, config)
}

func ValidateWrites(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	config *factorydefinitions.FactoryConfig,
) error {
	return snapshotsmaterialize.ValidateWrites(fileSystem, targetDir, config)
}

func CopySupportedFiles(
	fileSystem portablefiles.FileSystem,
	sourceDir string,
	targetDir string,
	config *factorydefinitions.FactoryConfig,
) error {
	return snapshotsportableconfig.CopySupportedFiles(fileSystem, sourceDir, targetDir, config)
}

func ValidateEditableSnapshot(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
	workstationLoader factorydefinitions.WorkstationLoader,
	mapRequest factorydefinitions.EditableFactoryValidationRequestMapper,
	validate factorydefinitions.DefinitionValidationOperation,
) error {
	return snapshotseditable.ValidateSnapshot(ctx, snapshot, workstationLoader, mapRequest, validate)
}

func PrepareFactorySnapshotImport(
	payload []byte,
	decode factoryeffects.FactorySnapshotJSONDecoder,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	return snapshotsprepare.Import(payload, decode)
}

func DecodeReplayRuntimeConfig(
	snapshot *factorydefinitions.FactorySnapshot,
	decode factorydefinitions.FactoryConfigJSONDecoder,
) (factoryeffects.ReplayRuntimeConfig, error) {
	return snapshotsreplayconfig.Decode(snapshot, decode)
}
