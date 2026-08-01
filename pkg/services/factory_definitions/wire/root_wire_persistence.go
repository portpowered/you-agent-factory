package wire

import (
	"context"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

// Persistence constructs the Factory Definitions persistence implementation
// from the validation boundary selected by its caller and the concrete authored
// layout adapters selected by Wire.
func Persistence(
	validator contracts.Validator,
	mapInput contracts.FactoryLayoutPayloadMapper,
	loader *Loader,
	pruneRemovedDocs contracts.PortableBundledDocsPruner,
	materializeFiles contracts.PortableBundledFilesMaterializer,
	validateWrites contracts.PortableBundledFileWritesValidator,
	copySupportedFiles contracts.PortableBundledFilesCopier,
	fileSystem factoryeffect.AuthoredLayoutWriterFileSystem,
	ensureInbox factoryeffect.InputInboxSentinelEnsurer,
	persistenceFileSystem factoryeffect.PersistenceFileSystem,
	namedPaths factoryeffect.NamedPathResolver,
	replacement factoryeffect.DirectoryReplacementStore,
	representation Representation,
) (contracts.Persistence, error) {
	writer := authoringlayoutwire.NewWriter(
		representation.RenderWorker,
		representation.RenderWorkstation,
		representation.RenderAgentsBody,
		authoringlayoutwire.NewAgentsFileWriter(fileSystem),
		representation.SafeLayoutSegment,
		representation.SafePromptPath,
		fileSystem,
		ensureInbox,
		compilationwire.NormalizeCanonicalWorkstationRuntime,
	)
	return NewPersistence(
		validator,
		mapInput,
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator contracts.Validator,
		) (*contracts.PreparedFactoryLayoutPayload, error) {
			return authoringlayoutwire.PrepareFactoryLayout(
				ctx,
				segment,
				payload,
				validator,
				representation.DecodeFactory,
				representation.NormalizeAuthored,
				representation.EncodeFactory,
			)
		},
		func(
			targetDir string,
			prepared *contracts.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				materializeFiles,
				pruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			return loader.ValidateFactoryDirReadOnly(
				targetDir,
				nil,
				validateWrites,
			)
		},
		loader.FlattenFactoryConfig,
		func(path string) (string, contracts.LayoutExpansionReport, error) {
			targetDir, sourceDir, sourcePath, factoryConfig, canonical, err :=
				loader.PrepareFactoryLayoutExpansion(path)
			if err != nil {
				return "", contracts.LayoutExpansionReport{}, err
			}
			report, err := writer.Expand(
				targetDir,
				sourceDir,
				sourcePath,
				factoryConfig,
				canonical,
				validateWrites,
				materializeFiles,
				copySupportedFiles,
			)
			return targetDir, report, err
		},
		namedPaths.WriteCurrentPointer,
		persistenceFileSystem,
		namedPaths.RequireDefinitionDir,
		replacement,
	)
}
