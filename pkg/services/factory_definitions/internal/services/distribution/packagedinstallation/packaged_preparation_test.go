package packagedinstallation

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type recordingPackagedFactoryPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
	packagedPrepareCalls int
	ordinaryPrepareCalls int
}

func (persistence *recordingPackagedFactoryPersistence) PreparePackagedFactoryLayout(
	ctx context.Context,
	name string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	persistence.packagedPrepareCalls++
	return persistence.PackagedFactoryPersistence.PreparePackagedFactoryLayout(ctx, name, payload)
}

func (persistence *recordingPackagedFactoryPersistence) PrepareFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	persistence.ordinaryPrepareCalls++
	return nil, fmt.Errorf("ordinary Factory preparation must not be used for packaged installation")
}

func TestInstallPackagedFactory_CreateAndReplaceUseExplicitPackagedPreparation(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	persistence := &recordingPackagedFactoryPersistence{
		PackagedFactoryPersistence: packagedInstallationTestPersistence(),
	}
	installer := New(persistence, platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		Definition:         definition,
		Format:             factorydefinitions.PackagedFactoryFormatJSON,
	}
	created, err := installer.InstallPackagedFactory(t.Context(), params)
	if err != nil {
		t.Fatalf("create InstallPackagedFactory() error = %v", err)
	}
	if created.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("create outcome = %q, want created", created.Outcome)
	}

	params.Replace = true
	replaced, err := installer.InstallPackagedFactory(t.Context(), params)
	if err != nil {
		t.Fatalf("replace InstallPackagedFactory() error = %v", err)
	}
	if replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replace outcome = %q, want replaced", replaced.Outcome)
	}
	if persistence.packagedPrepareCalls != 2 {
		t.Fatalf("packaged preparation calls = %d, want one for create and replace", persistence.packagedPrepareCalls)
	}
	if persistence.ordinaryPrepareCalls != 0 {
		t.Fatalf("ordinary preparation calls = %d, want zero", persistence.ordinaryPrepareCalls)
	}
}
