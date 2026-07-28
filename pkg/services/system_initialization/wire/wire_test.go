package systeminitializationwire

import (
	"context"
	"io/fs"
	"os"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

type recordingOperatorSettings struct {
	loadCalls   int
	ensureCalls int
}

func (settings *recordingOperatorSettings) LoadFileConfig(string) (operatorsettings.Config, error) {
	settings.loadCalls++
	panic("operator settings load during inert construction")
}

func (settings *recordingOperatorSettings) EnsureLocalBackendScope(string) (operatorsettings.ResolvedBackendScope, error) {
	settings.ensureCalls++
	panic("operator settings ensure during inert construction")
}

type recordingPackagedInstaller struct {
	calls int
}

func (installer *recordingPackagedInstaller) EnsurePackagedFactories(
	context.Context,
	string,
	[]factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	installer.calls++
	panic("packaged install during inert construction")
}

type recordingMigrationFileSystem struct {
	statCalls int
}

func (filesystem *recordingMigrationFileSystem) Stat(string) (fs.FileInfo, error) {
	filesystem.statCalls++
	panic("migration filesystem stat during inert construction")
}

func (filesystem *recordingMigrationFileSystem) ReadFile(string) ([]byte, error) {
	panic("migration filesystem read during inert construction")
}

func (filesystem *recordingMigrationFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	panic("migration filesystem readdir during inert construction")
}

func (filesystem *recordingMigrationFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("migration filesystem mkdir during inert construction")
}

func (filesystem *recordingMigrationFileSystem) Rename(string, string) error {
	panic("migration filesystem rename during inert construction")
}

func validPackagedCatalog() factorydefinitions.PackagedFactoryCatalogOperations {
	return factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			panic("packaged catalog list during inert construction")
		},
		Resolve: func(
			context.Context,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			panic("packaged catalog resolve during inert construction")
		},
	}
}

func TestNewServiceReturnsPublishedServiceRoot(t *testing.T) {
	t.Parallel()

	service, err := NewService(
		&recordingOperatorSettings{},
		validPackagedCatalog(),
		&recordingPackagedInstaller{},
		func(string) (fs.FileInfo, error) {
			panic("inspect path during inert construction")
		},
		&recordingMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() = nil")
	}

	var peer systeminitialization.Service = service
	if peer == nil {
		t.Fatal("constructed value is not assignable to systeminitialization.Service")
	}
}

func TestNewServiceConstructionIsInert(t *testing.T) {
	t.Parallel()

	settings := &recordingOperatorSettings{}
	installer := &recordingPackagedInstaller{}
	migrationFiles := &recordingMigrationFileSystem{}
	listCalls := 0
	resolveCalls := 0
	inspectCalls := 0
	catalog := factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			listCalls++
			panic("packaged catalog list during inert construction")
		},
		Resolve: func(
			context.Context,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			resolveCalls++
			panic("packaged catalog resolve during inert construction")
		},
	}

	service, err := NewService(
		settings,
		catalog,
		installer,
		func(string) (fs.FileInfo, error) {
			inspectCalls++
			panic("inspect path during inert construction")
		},
		migrationFiles,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() = nil")
	}
	if settings.loadCalls != 0 || settings.ensureCalls != 0 ||
		installer.calls != 0 || listCalls != 0 || resolveCalls != 0 ||
		inspectCalls != 0 || migrationFiles.statCalls != 0 {
		t.Fatalf(
			"NewService() invoked collaborators: settings.load=%d settings.ensure=%d installer=%d list=%d resolve=%d inspect=%d migration.stat=%d, want inert construction",
			settings.loadCalls,
			settings.ensureCalls,
			installer.calls,
			listCalls,
			resolveCalls,
			inspectCalls,
			migrationFiles.statCalls,
		)
	}
}

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
