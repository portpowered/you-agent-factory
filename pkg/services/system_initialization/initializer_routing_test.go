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
	ensureCalls []string
}

func (settings *routingOperatorSettings) LoadFileConfig(path string) (operatorsettings.Config, error) {
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

func (installer *routingPackagedInstaller) EnsurePackagedFactories(
	context.Context,
	string,
	[]factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	installer.called = true
	return nil, nil
}

type localMigrationFileSystem struct{}

func (localMigrationFileSystem) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }
func (localMigrationFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localMigrationFileSystem) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (localMigrationFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localMigrationFileSystem) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// TestRootService_InitializeRoutesThroughInternalWorkflow proves the published
// root Service.Initialize seam is fulfilled through internal workflow ownership
// rather than a second peer-facing Bootstrap authority interface.
func TestRootService_InitializeRoutesThroughInternalWorkflow(t *testing.T) {
	t.Parallel()

	settings := &routingOperatorSettings{}
	installer := &routingPackagedInstaller{}
	service, err := systeminitializationwire.NewService(
		settings,
		factorydefinitions.PackagedFactoryCatalogOperations{
			List: func(
				context.Context,
				factorydefinitions.ListBuiltInPackagedFactoriesRequest,
			) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
				return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
			},
			Resolve: func(
				context.Context,
				factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
			) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
				return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
			},
		},
		installer,
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
	if len(settings.ensureCalls) != 1 || !installer.called {
		t.Fatalf("settings.ensureCalls = %#v, installer.called = %v", settings.ensureCalls, installer.called)
	}
}
