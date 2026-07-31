package persistence_test

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

var factorydefinitioncomposition = func() factorydefinitiontestcomposition.Composition {
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	var composition factorydefinitiontestcomposition.Composition
	composition = factorydefinitiontestcomposition.New(factorydefinitiontestcomposition.Representation{
		DecodeFactory:     factorymapping.ExpandFactoryConfigForRuntimeLoad,
		DecodeAuthored:    mapper.Expand,
		EncodeFactory:     factorymapping.MarshalCanonicalFactoryConfig,
		NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		ParseWorker:       authoredmapping.ParseWorkerConfig,
		ParseWorkstation:  authoredmapping.ParseWorkstationConfig,
		ParseBody:         authoredmapping.ParseAgentsBody,
		RenderWorker:      authoredmapping.RenderWorkerAgentsMarkdown,
		RenderWorkstation: authoredmapping.RenderWorkstationAgentsMarkdown,
		RenderBody:        authoredmapping.RenderAgentsBody,
		SafeLayoutSegment: authoredmapping.SafeFactoryLayoutSegment,
		SafePromptPath:    authoredmapping.SafePromptFilePath,
		MapPersistence: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, composition.LoadCanonicalJSON)
		},
	}, fileSystem, directoryreplace.Local{}, factorydefinitiontestcomposition.Effects{
		Loading:             fileSystem,
		AuthoredReader:      fileSystem,
		AuthoredWriter:      fileSystem,
		Persistence:         fileSystem,
		NamedPaths:          fileSystem,
		NamedFactoryCatalog: fileSystem,
		InboxSentinels:      inboxgitkeep.NewLocal(fileSystem),
	})
	return composition
}()

var persistenceTestNamedPaths = func() *catalognamedpaths.Resolver {
	resolver, err := catalognamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		panic(err)
	}
	return resolver
}()

// These collaborators are kept package-local because this suite verifies the
// persistence implementation's transaction around the authored layout. They
// are not reusable application composition.
func prepareLayoutForPersistenceTest(
	ctx context.Context,
	segment string,
	payload []byte,
	validator factorydefinitions.Validator,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	mapper := factorymapping.NewFactoryConfigMapper()
	return authoringlayoutprepare.FactoryLayout(
		ctx,
		segment,
		payload,
		validator,
		mapper.Expand,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		mapper.Flatten,
	)
}

func writePreparedLayoutForPersistenceTest(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	sourcePath string,
) error {
	fileSystem := platformfilesystem.Local{}
	writer := factoryauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		factoryauthoredlayout.NewAgentsFileWriter(fileSystem),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		inboxgitkeep.NewLocal(fileSystem),
	)
	return writer.WritePrepared(
		targetDir,
		prepared,
		sourcePath,
		portableconfig.NewMaterializer(platformfilesystem.Local{}),
		factorydefinitioncomposition.PruneRemovedDocs,
	)
}

func validateLayoutForPersistenceTest(targetDir string) error {
	_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
	return err
}
