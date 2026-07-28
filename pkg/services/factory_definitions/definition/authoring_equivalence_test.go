package factorydefinition_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
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
		factorydefinition.StubActivationGateway(),
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

func TestRootAuthoringEquivalence_CTRDEFSuccessThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	service := newRootAuthoringServiceForPeer(t)
	peerExerciseRootAuthoringRoundTrip(t, service)
}

func TestRootAuthoringEquivalence_PeerExercisesRootWithoutAuthoringLayoutImport(t *testing.T) {
	t.Parallel()

	// Owner-local construction attaches private authoring_layout. The peer
	// exercise helpers accept only factoryroot.Service and root request/result/error
	// types, proving a peer can drive the slice end-to-end without importing
	// authoring_layout or other Definitions internals.
	service := newRootAuthoringServiceForPeer(t)
	peerExerciseRootAuthoringRoundTrip(t, service)
}

func peerExerciseRootAuthoringRoundTrip(t *testing.T, service factoryroot.Service) {
	t.Helper()

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

func TestRootAuthoringSlice_FailedWritePreservationOnRejectedCreate(t *testing.T) {
	t.Parallel()

	service := newRootAuthoringServiceWithCorruptingWriteForPeer(t)
	rootDir := t.TempDir()
	payload := crossPathValidAlphaAuthoringPayload(t)
	ctx := context.Background()

	prepared, err := service.PrepareFactoryLayout(
		ctx,
		factoryroot.PrepareFactoryLayoutRequest{Name: "broken", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}

	_, err = service.CreateNamedFactory(
		ctx,
		factoryroot.CreateNamedFactoryRequest{
			RootDir:  rootDir,
			Name:     "broken",
			Prepared: prepared.Prepared,
		},
	)
	var writeFailure *factoryroot.AtomicFactoryWriteFailure
	if !errors.As(err, &writeFailure) {
		t.Fatalf("CreateNamedFactory error = %v, want AtomicFactoryWriteFailure", err)
	}
	if !writeFailure.PreviousPreserved {
		t.Fatal("AtomicFactoryWriteFailure.PreviousPreserved = false, want true")
	}
	if !errors.Is(err, factoryroot.ErrInvalidNamedFactory) {
		t.Fatalf("CreateNamedFactory error = %v, want invalid factory failure", err)
	}
	for _, want := range []string{
		`validate factory "broken" config`,
		"AGENTS.md missing closing frontmatter delimiter",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CreateNamedFactory() error = %v, want substring %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(statErr) {
		t.Fatalf("failed create left partial target: %v", statErr)
	}
}

func TestRootAuthoringSlice_FailedWritePreservationOnRejectedReplace(t *testing.T) {
	t.Parallel()

	service := newRootAuthoringServiceForPeer(t)
	corruptingService := newRootAuthoringServiceWithCorruptingWriteForPeer(t)
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
	before, err := os.ReadFile(filepath.Join(created.FactoryDir, factoryroot.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json before): %v", err)
	}

	replacePrepared, err := corruptingService.PrepareFactoryLayout(
		ctx,
		factoryroot.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout(replace): %v", err)
	}
	_, err = corruptingService.ReplaceNamedFactory(
		ctx,
		factoryroot.ReplaceNamedFactoryRequest{
			RootDir:  rootDir,
			Name:     "alpha",
			Prepared: replacePrepared.Prepared,
		},
	)
	var writeFailure *factoryroot.AtomicFactoryWriteFailure
	if !errors.As(err, &writeFailure) {
		t.Fatalf("ReplaceNamedFactory error = %v, want AtomicFactoryWriteFailure", err)
	}
	if !writeFailure.PreviousPreserved {
		t.Fatal("AtomicFactoryWriteFailure.PreviousPreserved = false, want true")
	}
	after, err := os.ReadFile(filepath.Join(created.FactoryDir, factoryroot.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json after): %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("factory.json after rejected replace = %q, want %q", after, before)
	}
}

func newRootAuthoringServiceWithCorruptingWriteForPeer(t *testing.T) factoryroot.Service {
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
	persistence := composition.FactoryDefinitionPersistenceWithValidator(validator)
	_, _, pruneRemovedDocs := factorydefinitiontestcomposition.PortableOperations(fileSystem)
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
	materializeFiles := func(targetDir string, config *factoryroot.FactoryConfig) ([]factoryroot.PortableBundledFileReplacement, error) {
		return portableconfig.MaterializeFiles(fileSystem, targetDir, config)
	}
	validateWrites := func(targetDir string, config *factoryroot.FactoryConfig) error {
		return portableconfig.ValidateWrites(fileSystem, targetDir, config)
	}
	writePrepared := func(
		targetDir string,
		prepared *factoryroot.PreparedFactoryLayoutPayload,
		sourcePath string,
	) error {
		if err := writer.WritePrepared(
			targetDir,
			prepared,
			sourcePath,
			materializeFiles,
			pruneRemovedDocs,
		); err != nil {
			return err
		}
		brokenAgentsPath := filepath.Join(
			targetDir,
			factoryroot.WorkstationsDir,
			"process",
			factoryroot.FactoryAgentsFileName,
		)
		if err := os.MkdirAll(filepath.Dir(brokenAgentsPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(brokenAgentsPath, []byte("---\ntype: [\n"), 0o644)
	}
	authoringLayout, err := authoringlayoutwire.NewService(authoringlayout.Dependencies{
		Validator:            validator,
		MapInput:             composition.MapFactoryJSONForPersistence,
		DecodeFactory:        factorymapping.NewFactoryConfigMapper().Expand,
		NormalizeAuthored:    authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		EncodeFactory:        factorymapping.MarshalCanonicalFactoryConfig,
		Write:                writePrepared,
		Validate: func(targetDir string) error {
			return loader.ValidateFactoryDirReadOnly(targetDir, nil, validateWrites)
		},
		Flatten:              composition.FactoryLayoutFlattener(),
		Expand:               persistence.ExpandFactoryLayout,
		FileSystem:           fileSystem,
		RequireDefinitionDir: paths.RequireDefinitionDir,
		Directories:          directoryreplace.Local{},
	})
	if err != nil {
		t.Fatalf("authoringlayoutwire.NewService: %v", err)
	}

	return factorydefinition.NewWithCatalogPackagesValidationInstallationAndAuthoring(
		nil,
		factorydefinition.StubActivationGateway(),
		catalogService,
		nil,
		authoringLayout,
		factoryroot.PackagedFactoryCatalogOperations{},
		factoryroot.PackagedFactoryInstallationOperations{},
	)
}
