package service_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	documentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func TestRootDelegatesResolveEffectiveToPrivateOwner(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	documentService := documentwire.NewService(
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		nil,
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	configPath := "/home/operator/.you-agent-factory/config.json"
	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}
}

func TestNew_RejectsNilDocument(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}

	service, err := operatorservice.New(nil, resolutionService, nil, nil, nil, nil, nil, nil)
	if err == nil || service != nil {
		t.Fatalf("New(nil, resolution) = (%v, %v), want error", service, err)
	}
}

func TestNew_RejectsNilResolution(t *testing.T) {
	t.Parallel()

	documentService := documentwire.NewService(
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		rootTestProviderCatalog,
	)

	service, err := operatorservice.New(documentService, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || service != nil {
		t.Fatalf("New(document, nil) = (%v, %v), want error", service, err)
	}
}

func TestRootEnsureLocalBackendScopeGeneratesAndThenReuses(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")

	first, err := root.EnsureLocalBackendScope(path)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() = %v", err)
	}
	if first.Outcome != operatorsettings.BackendScopeOutcomeGenerated {
		t.Fatalf("first outcome = %q, want generated", first.Outcome)
	}
	if !root.IsLocalBackendScopeID(first.BackendScopeID) {
		t.Fatalf("first backend scope = %q, want local UUID", first.BackendScopeID)
	}

	second, err := root.EnsureLocalBackendScope(path)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() second call = %v", err)
	}
	if second.Outcome != operatorsettings.BackendScopeOutcomeReused || second.BackendScopeID != first.BackendScopeID {
		t.Fatalf("second result = %#v, want reuse of %#v", second, first)
	}
}

func TestRootEnsureLocalBackendScopeRejectsShortWriteWithoutReplacement(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return shortWriteTemporaryFile{name: filepath.Join(dir, pattern)}, nil
	})
	path := filepath.Join(t.TempDir(), "config.json")

	_, err := root.EnsureLocalBackendScope(path)
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("EnsureLocalBackendScope() = %v, want short-write failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("config path stat error = %v, want destination to remain absent", statErr)
	}
}

func TestRootACPConfigurationAddsDeletesAndMaterializesDefaults(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	integration := operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}

	added, err := root.ConfigureACPIntegrationAdd(context.Background(), path, integration)
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() = %v", err)
	}
	if len(added.Workers.ACP.Integrations) != 1 || added.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("added document integrations = %#v, want %#v", added.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{integration})
	}

	deleted, err := root.ConfigureACPIntegrationDelete(context.Background(), path, " cursor-acp ")
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationDelete() = %v", err)
	}
	if len(deleted.Workers.ACP.Integrations) != 0 {
		t.Fatalf("deleted document integrations = %#v, want empty", deleted.Workers.ACP.Integrations)
	}
	if _, err := root.ConfigureACPIntegrationDelete(context.Background(), path, "missing"); !errors.Is(err, operatorsettings.ErrACPIntegrationNotFound) {
		t.Fatalf("ConfigureACPIntegrationDelete(missing) = %v, want ErrACPIntegrationNotFound", err)
	}

	defaultsPath := filepath.Join(t.TempDir(), "config.json")
	defaults := []operatorsettings.ACPIntegration{integration}
	materialized, err := root.EnsurePackagedACPIntegrations(context.Background(), defaultsPath, defaults)
	if err != nil {
		t.Fatalf("EnsurePackagedACPIntegrations() = %v", err)
	}
	if len(materialized.Workers.ACP.Integrations) != 1 || materialized.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("materialized integrations = %#v, want %#v", materialized.Workers.ACP.Integrations, defaults)
	}

	preserved, err := root.EnsurePackagedACPIntegrations(context.Background(), defaultsPath, []operatorsettings.ACPIntegration{{ID: "other", Name: "other"}})
	if err != nil {
		t.Fatalf("EnsurePackagedACPIntegrations() second call = %v", err)
	}
	if len(preserved.Workers.ACP.Integrations) != 1 || preserved.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("preserved integrations = %#v, want original defaults", preserved.Workers.ACP.Integrations)
	}
}

func newFilesystemRoot(t *testing.T, createTemp operatorsettings.CreateTemporaryFile) operatorsettings.Service {
	t.Helper()

	files := platformfilesystem.Local{}
	documentService := documentwire.NewService(
		files,
		createTemp,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(internaltestproviders.StandardCatalog())
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		files,
		createTemp,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		nil,
	)
	if err != nil {
		t.Fatalf("operatorservice.New() = %v", err)
	}
	return root
}

func testCreateTemporaryFile(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

type shortWriteTemporaryFile struct {
	name string
}

func (file shortWriteTemporaryFile) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return len(data) - 1, nil
}

func (file shortWriteTemporaryFile) Name() string { return file.name }

func (shortWriteTemporaryFile) Sync() error  { return nil }
func (shortWriteTemporaryFile) Close() error { return nil }

type rootTestFileSystem struct{}

func (rootTestFileSystem) ReadFile(string) ([]byte, error) {
	panic("filesystem read during root service test")
}

func (rootTestFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("filesystem mkdir during root service test")
}

func (rootTestFileSystem) Remove(string) error {
	panic("filesystem remove during root service test")
}

func (rootTestFileSystem) Chmod(string, fs.FileMode) error {
	panic("filesystem chmod during root service test")
}

func (rootTestFileSystem) Rename(string, string) error {
	panic("filesystem rename during root service test")
}

func rootTestCreateTemporaryFile(string, string) (operatorsettings.TemporaryFile, error) {
	panic("temp-file creation during root service test")
}

func rootTestConfigDecoder([]byte) (operatorsettings.Config, error) {
	panic("config decode during root service test")
}

func rootTestConfigEncoder(operatorsettings.Config) ([]byte, error) {
	panic("config encode during root service test")
}

func rootTestProviderCatalog(string) (string, bool) {
	panic("provider catalog during root service test")
}
