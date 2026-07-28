package wire

import (
	"context"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// Persistence constructs the Factory Definitions persistence implementation
// from the validation boundary selected by its caller and the concrete authored
// layout adapters selected by Wire.
func Persistence(
	validator contracts.Validator,
	mapInput contracts.FactoryLayoutPayloadMapper,
	loader *factoryloading.Loader,
	pruneRemovedDocs contracts.PortableBundledDocsPruner,
	materializeFiles contracts.PortableBundledFilesMaterializer,
	validateWrites contracts.PortableBundledFileWritesValidator,
	copySupportedFiles contracts.PortableBundledFilesCopier,
	fileSystem contracts.AuthoredLayoutWriterFileSystem,
	ensureInbox contracts.InputInboxSentinelEnsurer,
	persistenceFileSystem contracts.PersistenceFileSystem,
	namedPaths contracts.NamedPathResolver,
	replacement contracts.DirectoryReplacementStore,
) (contracts.Persistence, error) {
	mapper := factorymapping.NewFactoryConfigMapper()
	writer := factoryauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		factoryauthoredlayout.NewAgentsFileWriter(fileSystem),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		ensureInbox,
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
			return authoringlayoutprepare.FactoryLayout(
				ctx,
				segment,
				payload,
				validator,
				mapper.Expand,
				authoredmapping.AuthoredFactoryConfigForExpandedLayout,
				mapper.Flatten,
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
		FactoryLayoutFlattener(loader),
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
