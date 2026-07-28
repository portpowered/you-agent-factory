package factorydefinition_test

import (
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

var externalDefinitionTestNamedPaths = func() *factorynamedpaths.Resolver {
	resolver, err := factorynamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		panic(err)
	}
	return resolver
}()

func persistNamedFactoryForTest(
	rootDir string,
	name string,
	payload []byte,
	validator factorydefinitions.Validator,
) (string, error) {
	return factorydefinitioncomposition.PersistNamedFactory(
		rootDir,
		name,
		payload,
		validator,
	)
}

func persistPreparedNamedFactoryForTest(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return preparedLayoutPersistence().
		CreateNamedFactory(rootDir, name, prepared)
}

func replacePreparedFactoryLayoutForTest(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return preparedLayoutPersistence().
		ReplaceFactoryLayout(targetDir, prepared)
}

func preparedLayoutPersistence() factorydefinitions.Persistence {
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
	persistence, err := factorypersistence.New(
		nil,
		nil,
		nil,
		func(
			targetDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				portableconfig.NewMaterializer(platformfilesystem.Local{}),
				factorydefinitioncomposition.PruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
			return err
		},
		nil,
		nil,
		nil,
		fileSystem,
		externalDefinitionTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}
