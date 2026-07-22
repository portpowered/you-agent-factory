package factorydefinitions_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
)

func TestLoaderLoadsDirectoryAndCanonicalRepresentations(t *testing.T) {
	t.Parallel()

	fixture := filepath.Join(testutil.MustRepoRoot(t), "examples", "basic", "factory")
	factoryDir := testutil.CopyFixtureDir(t, fixture)
	fileSystem := platformfilesystem.Local{}
	applySupportedFiles, err := portableconfig.NewPortableBundledFilesApplier(fileSystem)
	if err != nil {
		t.Fatalf("construct bundled-files applier: %v", err)
	}
	applyStarterWork, err := portableconfig.NewFactoryStarterWorkApplier(fileSystem)
	if err != nil {
		t.Fatalf("construct starter-Work applier: %v", err)
	}
	materializeFiles := func(
		targetDir string,
		config *factorydefinitions.FactoryConfig,
	) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return portableconfig.MaterializeFiles(
			fileSystem,
			targetDir,
			config,
		)
	}
	namedPaths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("construct named-path resolver: %v", err)
	}
	loader := wirefactorydefinitions.Loader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		fileSystem,
		namedPaths,
		fileSystem,
		mustSourceResolver(t, fileSystem),
		fileSystem,
		nil,
	)
	directorySource, err := loader.
		LoadSourceFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("load Factory directory: %v", err)
	}
	payload, err := loader.FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("flatten Factory directory: %v", err)
	}
	canonicalSource, err := loader.
		LoadSourceFromCanonicalJSON(payload, nil)
	if err != nil {
		t.Fatalf("load canonical Factory: %v", err)
	}
	if _, ok := directorySource.Worker("processor"); !ok {
		t.Fatal("directory Factory worker processor not loaded")
	}
	if _, ok := directorySource.Workstation("process"); !ok {
		t.Fatal("directory Factory workstation process not loaded")
	}
	if _, ok := canonicalSource.Worker("processor"); !ok {
		t.Fatal("canonical Factory worker processor not loaded")
	}
	if _, ok := canonicalSource.Workstation("process"); !ok {
		t.Fatal("canonical Factory workstation process not loaded")
	}
}

func mustSourceResolver(t *testing.T, fileSystem platformfilesystem.Local) factorydefinitions.PortableBundledFileSourceResolver {
	t.Helper()
	resolver, err := portableconfig.NewSupportedSourceResolver(fileSystem)
	if err != nil {
		t.Fatalf("construct portable source resolver: %v", err)
	}
	return resolver
}
