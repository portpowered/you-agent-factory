package packagedinstallation

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func packagedInstallationTestPersistence() factorydefinitions.Persistence {
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
			return validationentry.MapFactoryJSONForPersistence(payload, factorydefinitioncomposition.LoadCanonicalJSON)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return factoryauthoredlayout.Prepare(
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

func TestEnsurePackagedFactories_InvalidPayloadDoesNotCommitTarget(t *testing.T) {
	root := t.TempDir()
	definition := factorydefinitions.PackagedDefinition{
		Name: "@test/invalid",
		JSON: []byte(`{"id":"invalid","workers":[`),
	}
	_, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(t.Context(), root, []factorydefinitions.PackagedDefinition{definition})
	if err == nil || !strings.Contains(err.Error(), "install packaged factory") {
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

func TestEnsurePackagedFactories_PreparationFailurePreservesExistingRoot(t *testing.T) {
	root := t.TempDir()
	marker := root + string(os.PathSeparator) + "customer-owned.txt"
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := factorydefinitions.PackagedDefinition{Name: "@test/invalid", JSON: []byte(`{`)}
	if _, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}).
		EnsurePackagedFactories(t.Context(), root, []factorydefinitions.PackagedDefinition{definition}); err == nil {
		t.Fatal("EnsurePackagedFactories() error = nil")
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "keep" {
		t.Fatalf("customer marker = %q, %v", content, err)
	}
}

func TestEnsurePackagedFactories_FailsClosedWithoutFileSystem(t *testing.T) {
	_, err := New(packagedInstallationTestPersistence(), nil).EnsurePackagedFactories(
		t.Context(),
		t.TempDir(),
		[]factorydefinitions.PackagedDefinition{{Name: "@test/missing-filesystem", JSON: []byte(`{}`)}},
	)
	if err == nil || !strings.Contains(err.Error(), "installation filesystem is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
}
