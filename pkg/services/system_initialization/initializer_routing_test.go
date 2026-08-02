package systeminitialization_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
)

type routingOperatorSettings struct {
	operatorsettings.Service
	loadCalls   []string
	ensureCalls []string
}

func (routingOperatorSettings) DefaultConfigPath(homeDir string) string {
	return filepath.Join(homeDir, "settings-owned", "config.json")
}

func (settings *routingOperatorSettings) LoadFileConfig(path string) (operatorsettings.Config, error) {
	settings.loadCalls = append(settings.loadCalls, path)
	return operatorsettings.Config{}, nil
}

func (settings *routingOperatorSettings) EnsureLocalBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	settings.ensureCalls = append(settings.ensureCalls, path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return operatorsettings.ResolvedBackendScope{}, err
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		return operatorsettings.ResolvedBackendScope{}, err
	}
	return operatorsettings.ResolvedBackendScope{}, nil
}

type routingPackagedInstaller struct {
	called bool
}

func (installer *routingPackagedInstaller) install(
	context.Context,
	factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	installer.called = true
	return factorydefinitions.InstallPackagedFactoryResult{}, nil
}

func routingDefinitionsService(installer *routingPackagedInstaller) factorydefinitions.Service {
	definition := factorydefinitions.PackagedDefinition{Name: "@you/goal"}
	return &definitionsService{
		listFn: func(context.Context, factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{Name: definition.Name}}}, nil
		},
		resolveFn: func(context.Context, factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{Definition: definition}, nil
		},
		installFn: installer.install,
	}
}

type localMigrationFileSystem struct{}

func (localMigrationFileSystem) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }
func (localMigrationFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localMigrationFileSystem) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (localMigrationFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localMigrationFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// TestRootService_InitializeRoutesThroughInternalWorkflow proves the published
// root Service.Initialize seam is fulfilled through internal workflow ownership
// rather than a second peer-facing Bootstrap authority interface.
func TestRootService_InitializeRoutesThroughInternalWorkflow(t *testing.T) {
	t.Parallel()

	settings := &routingOperatorSettings{}
	installer := &routingPackagedInstaller{}
	service, err := systeminitializationwire.NewService(
		settings,
		routingDefinitionsService(installer),
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var rootService systeminitialization.Service = service
	homeDir := t.TempDir()
	result, err := rootService.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.HomeDir != homeDir || result.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("Initialize() result = %#v", result)
	}
	wantConfigPath := settings.DefaultConfigPath(homeDir)
	if result.ConfigPath != wantConfigPath {
		t.Fatalf("ConfigPath = %q, want Settings root DefaultConfigPath %q", result.ConfigPath, wantConfigPath)
	}
	if len(settings.ensureCalls) != 1 || settings.ensureCalls[0] != wantConfigPath {
		t.Fatalf("EnsureLocalBackendScope calls = %#v, want [%q]", settings.ensureCalls, wantConfigPath)
	}
	if len(settings.loadCalls) != 1 || settings.loadCalls[0] != wantConfigPath {
		t.Fatalf("LoadFileConfig calls = %#v, want [%q]", settings.loadCalls, wantConfigPath)
	}
	if !installer.called {
		t.Fatalf("installer.called = false, want packaged install after Settings commands")
	}
}

// TestRootService_InitializeSkipPathConstructsSettingsLoadCommandThroughRootCollaborator
// proves the published Service seam routes skip-path Settings load commands through
// the injected Settings root collaborator without invoking ensure.
func TestRootService_InitializeSkipPathConstructsSettingsLoadCommandThroughRootCollaborator(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	settings := &routingOperatorSettings{}
	configPath := settings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"customer":"owned"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	installer := &routingPackagedInstaller{}
	service, err := systeminitializationwire.NewService(
		settings,
		routingDefinitionsService(installer),
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var rootService systeminitialization.Service = service
	result, err := rootService.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("SystemConfigOutcome = %q, want skipped", result.SystemConfigOutcome)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("ConfigPath = %q, want %q", result.ConfigPath, configPath)
	}
	if len(settings.loadCalls) != 1 || settings.loadCalls[0] != configPath {
		t.Fatalf("LoadFileConfig calls = %#v, want [%q]", settings.loadCalls, configPath)
	}
	if len(settings.ensureCalls) != 0 {
		t.Fatalf("EnsureLocalBackendScope calls = %#v, want none on skip path", settings.ensureCalls)
	}
}
