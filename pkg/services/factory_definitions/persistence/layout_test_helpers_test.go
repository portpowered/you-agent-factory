package persistence_test

import (
	"context"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

var persistenceTestNamedPaths = func() *factorynamedpaths.Resolver {
	resolver, err := factorynamedpaths.New(platformfilesystem.Local{})
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
	return factoryauthoredlayout.Prepare(
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
