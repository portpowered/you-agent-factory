package systeminitializationwire

import (
	"context"
	"os"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

type wireOperatorSettings struct{}

func (wireOperatorSettings) LoadFileConfig(string) (operatorsettings.Config, error) {
	return operatorsettings.Config{}, nil
}

func (wireOperatorSettings) EnsureLocalBackendScope(string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.ResolvedBackendScope{}, nil
}

type wirePackagedInstaller struct{}

func (wirePackagedInstaller) EnsurePackagedFactories(
	context.Context,
	string,
	[]factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	return nil, nil
}

func TestNewServiceConstructsBootstrapService(t *testing.T) {
	t.Parallel()

	service, err := NewService(
		wireOperatorSettings{},
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
		wirePackagedInstaller{},
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() = nil")
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
