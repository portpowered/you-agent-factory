package factorydefinition_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func newRootAuthoringServiceForPeer(t *testing.T) factoryroot.Service {
	t.Helper()

	composition := newAuthoringEquivalenceComposition(t)
	validator := factoryvalidation.New(nil)
	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}

	loader := composition.Loader()
	_, _, pruneRemovedDocs := factorydefinitiontestcomposition.PortableOperations(fileSystem)
	materializeFiles := func(targetDir string, config *factoryroot.FactoryConfig) ([]factoryroot.PortableBundledFileReplacement, error) {
		return portableconfig.MaterializeFiles(fileSystem, targetDir, config)
	}
	validateWrites := func(targetDir string, config *factoryroot.FactoryConfig) error {
		return portableconfig.ValidateWrites(fileSystem, targetDir, config)
	}
	copySupportedFiles := func(sourceDir, targetDir string, config *factoryroot.FactoryConfig) error {
		return portableconfig.CopySupportedFiles(fileSystem, sourceDir, targetDir, config)
	}
	authoringLayout, err := factorydefinitionswire.NewAuthoringLayoutService(factorydefinitionswire.AuthoringLayoutDependencies{
		Validator:          validator,
		MapInput:           composition.MapFactoryJSONForPersistence,
		Loader:             loader,
		MaterializeFiles:   materializeFiles,
		ValidateWrites:     validateWrites,
		PruneRemovedDocs:   pruneRemovedDocs,
		CopySupportedFiles: copySupportedFiles,
		AuthoredWriterFS:   fileSystem,
		EnsureInbox:        inboxgitkeep.NewLocal(fileSystem),
		PersistenceFS:      fileSystem,
		NamedPaths:           paths,
		Directories:          directoryreplace.Local{},
	})
	if err != nil {
		t.Fatalf("NewAuthoringLayoutService: %v", err)
	}

	return factorydefinition.NewWithCatalogPackagesValidationInstallationAndAuthoring(
		nil,
		catalogService,
		nil,
		authoringLayout,
		factoryroot.PackagedFactoryCatalogOperations{},
		factoryroot.PackagedFactoryInstallationOperations{},
	)
}

func newAuthoringEquivalenceComposition(t *testing.T) factorydefinitiontestcomposition.Composition {
	t.Helper()

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
		MapPersistence: func(payload []byte) (factoryroot.DefinitionValidationRequest, error) {
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
	}, mustAuthoringEquivalenceRequiredToolChecker())
	return composition
}

func mustAuthoringEquivalenceRequiredToolChecker() factoryroot.RequiredToolChecker {
	checker, err := factoryloading.NewPathRequiredToolChecker(
		exec.LookPath,
		func(path string, args ...string) ([]byte, error) {
			return exec.Command(path, args...).CombinedOutput()
		},
	)
	if err != nil {
		panic(err)
	}
	return checker
}

func crossPathValidAlphaAuthoringPayload(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(alpha factory): %v", err)
	}
	return payload
}

func TestRootAuthoringSlice_PrepareFlattenExpandCreateReplaceRoundTrip(t *testing.T) {
	t.Parallel()

	service := newRootAuthoringServiceForPeer(t)
	rootDir := t.TempDir()
	payload := crossPathValidAlphaAuthoringPayload(t)
	ctx := context.Background()

	prepared, err := service.PrepareFactoryLayout(
		ctx,
		factoryroot.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if len(prepared.Prepared.Canonical) == 0 {
		t.Fatal("PrepareFactoryLayout returned empty canonical payload")
	}

	created, err := service.CreateNamedFactory(
		ctx,
		factoryroot.CreateNamedFactoryRequest{
			RootDir:  rootDir,
			Name:     "alpha",
			Prepared: prepared.Prepared,
		},
	)
	if err != nil {
		t.Fatalf("CreateNamedFactory: %v", err)
	}
	if created.Name != "alpha" {
		t.Fatalf("CreateNamedFactory name = %q, want alpha", created.Name)
	}
	if _, err := os.Stat(created.FactoryDir); err != nil {
		t.Fatalf("CreateNamedFactory factoryDir missing: %v", err)
	}

	flattened, err := service.FlattenFactoryLayout(
		ctx,
		factoryroot.FlattenFactoryLayoutRequest{Path: created.FactoryDir},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if len(flattened.Canonical) == 0 {
		t.Fatal("FlattenFactoryLayout returned empty canonical payload")
	}

	canonicalPath := filepath.Join(t.TempDir(), "alpha.canonical.json")
	if err := os.WriteFile(canonicalPath, flattened.Canonical, 0o600); err != nil {
		t.Fatalf("WriteFile(canonical): %v", err)
	}

	expanded, err := service.ExpandFactoryLayout(
		ctx,
		factoryroot.ExpandFactoryLayoutRequest{Path: canonicalPath},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir == "" {
		t.Fatal("ExpandFactoryLayout returned empty factoryDir")
	}

	replacedPrepared, err := service.PrepareFactoryLayout(
		ctx,
		factoryroot.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout(replace): %v", err)
	}
	replaced, err := service.ReplaceNamedFactory(
		ctx,
		factoryroot.ReplaceNamedFactoryRequest{
			RootDir:  rootDir,
			Name:     "alpha",
			Prepared: replacedPrepared.Prepared,
		},
	)
	if err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}
	if replaced.FactoryDir != created.FactoryDir {
		t.Fatalf("ReplaceNamedFactory factoryDir = %q, want %q", replaced.FactoryDir, created.FactoryDir)
	}
}

func TestRootAuthoringSlice_RejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	service := newRootAuthoringServiceForPeer(t)
	_, err := service.PrepareFactoryLayout(
		context.Background(),
		factoryroot.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte("{")},
	)
	if !errors.Is(err, factoryroot.ErrInvalidNamedFactory) {
		t.Fatalf("PrepareFactoryLayout malformed error = %v, want %v", err, factoryroot.ErrInvalidNamedFactory)
	}
}
