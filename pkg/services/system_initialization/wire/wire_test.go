package systeminitializationwire

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	return operatorsettings.Config{}, nil
}

func (settings *recordingOperatorSettings) EnsureLocalBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	settings.ensureCalls++
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return operatorsettings.ResolvedBackendScope{}, err
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		return operatorsettings.ResolvedBackendScope{}, err
	}
	return operatorsettings.ResolvedBackendScope{}, nil
}

type recordingPackaging struct {
	entries      []factorydefinitions.BuiltInPackagedFactoryEntry
	listCalls    int
	resolveCalls int
	installCalls int
}

func (packaging *recordingPackaging) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	packaging.listCalls++
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{
		Entries: append([]factorydefinitions.BuiltInPackagedFactoryEntry(nil), packaging.entries...),
	}, nil
}

func (packaging *recordingPackaging) ResolveBuiltInPackagedFactory(
	_ context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	packaging.resolveCalls++
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
		Definition: factorydefinitions.PackagedDefinition{Name: request.Name},
	}, nil
}

func (packaging *recordingPackaging) InstallPackagedFactory(
	_ context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	packaging.installCalls++
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name: request.Name, FactoryDir: filepath.Join(request.RootDir, request.Name),
		},
		Outcome: factorydefinitions.PackagedFactoryInstallCreated,
		Format:  request.Format,
	}, nil
}

type recordingMigrationFileSystem struct {
	statCalls int
}

func (filesystem *recordingMigrationFileSystem) Stat(string) (fs.FileInfo, error) {
	filesystem.statCalls++
	return nil, fs.ErrNotExist
}
func (recordingMigrationFileSystem) ReadFile(string) ([]byte, error) { return nil, fs.ErrNotExist }
func (recordingMigrationFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, fs.ErrNotExist
}
func (recordingMigrationFileSystem) MkdirAll(string, fs.FileMode) error { return nil }
func (recordingMigrationFileSystem) Rename(string, string) error        { return nil }

func TestNewServiceReturnsPublishedServiceRootAndIsInert(t *testing.T) {
	t.Parallel()

	settings := &recordingOperatorSettings{}
	packaging := &recordingPackaging{}
	migrationFiles := &recordingMigrationFileSystem{}
	inspectCalls := 0
	service, err := NewService(
		settings,
		packaging,
		func(string) (fs.FileInfo, error) {
			inspectCalls++
			return nil, fs.ErrNotExist
		},
		migrationFiles,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root systeminitialization.Service = service
	if root == nil {
		t.Fatal("NewService() did not return the System Initialization root")
	}
	if settings.loadCalls != 0 || settings.ensureCalls != 0 ||
		packaging.listCalls != 0 || packaging.resolveCalls != 0 || packaging.installCalls != 0 ||
		inspectCalls != 0 || migrationFiles.statCalls != 0 {
		t.Fatalf("NewService() invoked collaborators during construction")
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	validSettings := &recordingOperatorSettings{}
	validPackaging := &recordingPackaging{}
	validInspectPath := func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
	validMigrationFiles := &recordingMigrationFileSystem{}
	tests := []struct {
		name       string
		settings   OperatorSettings
		packaging  factorydefinitions.Packaging
		inspect    InspectPath
		migration  LegacyFactoryMigrationFileSystem
		wantReason string
	}{
		{"operator settings", nil, validPackaging, validInspectPath, validMigrationFiles, "Operator Settings service is required"},
		{"packaging", validSettings, nil, validInspectPath, validMigrationFiles, "packaging capability is required"},
		{"inspect path", validSettings, validPackaging, nil, validMigrationFiles, "inspect path edge is required"},
		{"migration files", validSettings, validPackaging, validInspectPath, nil, "legacy Factory migration filesystem is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(test.settings, test.packaging, test.inspect, test.migration)
			if err == nil || service != nil || !strings.Contains(err.Error(), test.wantReason) {
				t.Fatalf("NewService() = (%#v, %v), want missing %s", service, err, test.name)
			}
		})
	}
}

func TestNewServiceUsesFocusedPackagingAfterConstruction(t *testing.T) {
	t.Parallel()

	settings := &recordingOperatorSettings{}
	packaging := &recordingPackaging{entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
		Name: "@you/goal",
	}}}
	service, err := NewService(
		settings,
		packaging,
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigCreated ||
		len(result.PackagedFactories) != 1 ||
		result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("Initialize() result = %#v", result)
	}
	if settings.ensureCalls != 1 || settings.loadCalls != 1 ||
		packaging.listCalls != 1 || packaging.resolveCalls != 1 || packaging.installCalls != 1 {
		t.Fatalf("Initialize() collaborator calls = settings(%d/%d) packaging(%d/%d/%d)", settings.ensureCalls, settings.loadCalls, packaging.listCalls, packaging.resolveCalls, packaging.installCalls)
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
