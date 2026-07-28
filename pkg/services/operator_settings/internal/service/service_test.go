package service_test

import (
	"io/fs"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	documentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
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
	root, err := operatorservice.New(documentService, resolutionService)
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

	service, err := operatorservice.New(nil, resolutionService)
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

	service, err := operatorservice.New(documentService, nil)
	if err == nil || service != nil {
		t.Fatalf("New(document, nil) = (%v, %v), want error", service, err)
	}
}

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
