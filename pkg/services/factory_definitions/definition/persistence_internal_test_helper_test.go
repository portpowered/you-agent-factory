package factorydefinition

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func persistNamedFactoryForTest(
	rootDir string,
	name string,
	payload []byte,
	validator factorydefinitions.Validator,
) (string, error) {
	persistence := definitionPersistenceForTest(validator)
	prepared, err := persistence.PrepareFactoryLayout(
		context.Background(),
		name,
		payload,
	)
	if err != nil {
		return "", err
	}
	return persistence.CreateNamedFactory(rootDir, name, prepared)
}

func persistPreparedNamedFactoryForTest(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return definitionPersistenceForTest(factoryvalidation.New(nil)).
		CreateNamedFactory(rootDir, name, prepared)
}

func replacePreparedFactoryLayoutForTest(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return definitionPersistenceForTest(factoryvalidation.New(nil)).
		ReplaceFactoryLayout(targetDir, prepared)
}

func definitionPersistenceForTest(
	validator factorydefinitions.Validator,
) factorydefinitions.Persistence {
	persistence, err := factorypersistence.New(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(
				payload,
				func(
					payload []byte,
					loader factorydefinitions.WorkstationLoader,
				) (factorydefinitions.MutableLoadedFactorySource, error) {
					return factorydefinitioncomposition.LoadCanonicalJSON(
						payload,
						loader)

				},
			)
		},
		prepareFactoryLayoutForDefinitionTest,
		writePreparedLayoutForDefinitionTest,
		validateLayoutForDefinitionTest,
		nil,
		nil,
		definitionTestNamedPaths.WriteCurrentPointer,
		platformfilesystem.Local{},
		definitionTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func prepareFactoryLayoutForDefinitionTest(
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

func writePreparedLayoutForDefinitionTest(
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

func validateLayoutForDefinitionTest(targetDir string) error {
	_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
	return err
}
