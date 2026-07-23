package factorydefinitions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
)

func TestLoaderLoadsDirectoryAndCanonicalRepresentations(t *testing.T) {
	t.Parallel()

	fixture := filepath.Join(testutil.MustRepoRoot(t), "examples", "basic", "factory")
	factoryDir := testutil.CopyFixtureDir(t, fixture)
	fileSystem := platformfilesystem.Local{}
	loader := newTestLoader(t, fileSystem)
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

func TestLoaderUsesSameYAMLRootSelectionForRuntimeAndReadOnlyValidation(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	yamlPath := filepath.Join(factoryDir, "factory.yaml")
	if err := os.WriteFile(yamlPath, []byte("name: yaml-root\n"), 0o644); err != nil {
		t.Fatalf("write YAML Factory root: %v", err)
	}
	loader := newTestLoader(t, platformfilesystem.Local{})
	if _, err := loader.LoadSourceFromFactoryDir(factoryDir, nil); err != nil {
		t.Fatalf("runtime load YAML Factory directory: %v", err)
	}
	if err := loader.ValidateFactoryDirReadOnly(factoryDir, nil, nil); err != nil {
		t.Fatalf("read-only validate YAML Factory directory: %v", err)
	}

	jsonPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	if err := os.WriteFile(jsonPath, []byte(`{"name":"conflict"}`), 0o644); err != nil {
		t.Fatalf("write conflicting JSON Factory root: %v", err)
	}
	_, runtimeErr := loader.LoadSourceFromFactoryDir(factoryDir, nil)
	validationErr := loader.ValidateFactoryDirReadOnly(factoryDir, nil, nil)
	for operation, err := range map[string]error{
		"runtime load":         runtimeErr,
		"read-only validation": validationErr,
	} {
		if err == nil || !strings.Contains(err.Error(), "ambiguous roots") {
			t.Fatalf("%s error = %v, want ambiguity", operation, err)
		}
		for _, path := range []string{jsonPath, yamlPath} {
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("%s error = %q, want conflicting root %q", operation, err, path)
			}
		}
	}
}

func newTestLoader(
	t *testing.T,
	fileSystem platformfilesystem.Local,
) *factoryloading.Loader {
	t.Helper()

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
	return wirefactorydefinitions.Loader(
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
}

func mustSourceResolver(t *testing.T, fileSystem platformfilesystem.Local) factorydefinitions.PortableBundledFileSourceResolver {
	t.Helper()
	resolver, err := portableconfig.NewSupportedSourceResolver(fileSystem)
	if err != nil {
		t.Fatalf("construct portable source resolver: %v", err)
	}
	return resolver
}
