package wire

import (
	"context"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
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
	writer *AuthoredLayoutWriter,
	persistenceFileSystem contracts.PersistenceFileSystem,
	namedPaths contracts.NamedPathResolver,
	replacement contracts.DirectoryReplacementStore,
	decodeFactory contracts.FactoryConfigJSONDecoder,
	encodeFactory contracts.FactoryConfigJSONEncoder,
) (contracts.PackagedFactoryPersistence, error) {
	return NewPersistence(
		validator,
		mapInput,
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator contracts.Validator,
		) (*contracts.PreparedFactoryLayoutPayload, error) {
			return authoringlayoutprepare.FactoryLayout(
				ctx,
				segment,
				payload,
				validator,
				decodeFactory,
				authoredmapping.AuthoredFactoryConfigForExpandedLayout,
				encodeFactory,
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
