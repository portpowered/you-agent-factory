package service_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func packagedInstallerTestPersistence() factorydefinitions.Persistence {
	validator := factoryvalidation.New(nil)
	mapper := factorymapping.NewFactoryConfigMapper()
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
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(
				payload,
				factorydefinitioncomposition.LoadCanonicalJSON,
			)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
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
		platformfilesystem.Local{},
		factorydefinitioncomposition.NamedPaths().RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func publishedPackagedDefinition(t *testing.T, name string) factorydefinitions.PackagedDefinition {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup(name)
	if !ok {
		t.Fatalf("published catalog is missing %s", name)
	}
	return definition
}

func TestNewPackagedFactoryInstaller_CreatesPackagedFactory(t *testing.T) {
	root := t.TempDir()
	definition := publishedPackagedDefinition(t, "@you/goal")
	installer := factorydefinitionsservice.NewPackagedFactoryInstaller(
		packagedInstallerTestPersistence(),
		platformfilesystem.Local{},
	)

	results, err := installer.EnsurePackagedFactories(
		t.Context(),
		root,
		[]factorydefinitions.PackagedDefinition{definition},
	)
	if err != nil {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one entry", results)
	}
	if results[0].Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("outcome = %q, want created", results[0].Outcome)
	}
	if results[0].FactoryDir == "" {
		t.Fatal("factory dir is empty")
	}
	if _, err := os.Stat(results[0].FactoryDir); err != nil {
		t.Fatalf("installed factory dir missing: %v", err)
	}
}

func TestNewPackagedFactoryInstaller_FailsClosedWithoutFileSystem(t *testing.T) {
	root := t.TempDir()
	installer := factorydefinitionsservice.NewPackagedFactoryInstaller(
		packagedInstallerTestPersistence(),
		nil,
	)
	_, err := installer.EnsurePackagedFactories(
		t.Context(),
		root,
		[]factorydefinitions.PackagedDefinition{
			{Name: "@test/missing-filesystem", JSON: []byte(`{}`)},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "installation filesystem is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("committed entries = %v, want none", entries)
	}
}

func TestNewPackagedFactoryInstaller_FailsClosedWithoutPersistence(t *testing.T) {
	root := t.TempDir()
	installer := factorydefinitionsservice.NewPackagedFactoryInstaller(
		nil,
		platformfilesystem.Local{},
	)
	_, err := installer.EnsurePackagedFactories(
		t.Context(),
		root,
		[]factorydefinitions.PackagedDefinition{
			{Name: "@test/missing-persistence", JSON: []byte(`{}`)},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "persistence service is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("committed entries = %v, want none", entries)
	}
}
