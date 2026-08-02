package construct_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestNewServiceFromConfigDocumentRequiresDocumentPorts(t *testing.T) {
	t.Parallel()

	_, err := settingsconstruct.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{})
	if err == nil || !strings.Contains(err.Error(), "operator settings document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

func TestNewServiceFromConfigDocumentConstructsFromPorts(t *testing.T) {
	t.Parallel()

	root, err := settingsconstruct.NewServiceFromConfigDocument(testConfigDocumentService())
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestNewServiceFromConfigDocumentUsesInjectedDocumentOwner(t *testing.T) {
	t.Parallel()

	service := testConfigDocumentService()
	service.DocumentOwner = settingsconstruct.NewDocumentOwner(
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		service.Providers,
	)

	root, err := settingsconstruct.NewServiceFromConfigDocument(service)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestNewConfigDocumentServiceConstructsDocumentOwner(t *testing.T) {
	t.Parallel()

	service := settingsconstruct.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		testProviderCatalog,
		&sync.Mutex{},
	)
	if service.Files == nil || service.Decoder == nil || service.DocumentOwner == nil {
		t.Fatalf("NewConfigDocumentService() = %#v, want populated document ports", service)
	}
}

func TestCompositionDelegatesDocumentAndResolutionOperations(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	root, err := settingsconstruct.NewServiceFromConfigDocument(testConfigDocumentService())
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("loaded provider = %q, want codex", loaded.Document.Defaults.WorkerModelProvider)
	}

	provider := "gemini"
	model := "gemini-pro"
	updated, err := root.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: configPath,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &provider,
			Model:    &model,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() error = %v", err)
	}
	if updated.Document.Defaults.WorkerModelProvider != "GEMINI" || updated.Document.Defaults.WorkerModel != "gemini-pro" {
		t.Fatalf("updated defaults = %#v, want GEMINI/gemini-pro", updated.Document.Defaults)
	}

	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() error = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" || resolved.Selection.WorkerModel != "flag-model" {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}

	_, err = root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: configPath,
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}
}

func TestNewServiceFromHomePortsRequiresFilesystem(t *testing.T) {
	t.Parallel()

	_, err := settingsconstruct.NewServiceFromHomePorts(nil, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}
}

func TestNewServiceFromHomePortsRequiresDecoder(t *testing.T) {
	t.Parallel()

	_, err := settingsconstruct.NewServiceFromHomePorts(platformfilesystem.Local{}, nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestNewServiceFromHomePortsConstructsAcceptedSettingsRoot(t *testing.T) {
	t.Parallel()

	root, err := settingsconstruct.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}

func TestNewServiceFromHomePortsPropagatesResolutionConstructionErrors(t *testing.T) {
	restore := settingsconstruct.SetConstructResolutionServiceForTests(func() (resolution.Service, error) {
		return nil, errors.New("resolution construction failed")
	})
	t.Cleanup(restore)

	_, err := settingsconstruct.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "resolution construction failed") {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want resolution construction failure", err)
	}
}

func TestNewServiceFromConfigDocumentPropagatesResolutionConstructionErrors(t *testing.T) {
	restore := settingsconstruct.SetConstructResolutionServiceForTests(func() (resolution.Service, error) {
		return nil, errors.New("resolution construction failed")
	})
	t.Cleanup(restore)

	_, err := settingsconstruct.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{
		Files:   platformfilesystem.Local{},
		Decoder: globalconfigmapping.Decode,
	})
	if err == nil || !strings.Contains(err.Error(), "resolution construction failed") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want resolution construction failure", err)
	}
}

func TestConstructResolutionServiceConstructsAcceptedResolutionRoot(t *testing.T) {
	t.Parallel()

	service, err := settingsconstruct.ConstructResolutionService()
	if err != nil {
		t.Fatalf("ConstructResolutionService() error = %v", err)
	}
	if service == nil {
		t.Fatal("ConstructResolutionService() = nil, want resolution service")
	}
}

func TestConstructResolutionServicePropagatesProvidersRootErrors(t *testing.T) {
	restore := settingsconstruct.SetConstructProvidersRootForTests(func() (providers.Service, error) {
		return nil, errors.New("providers root failed")
	})
	t.Cleanup(restore)

	_, err := settingsconstruct.ConstructResolutionService()
	if err == nil || !strings.Contains(err.Error(), "construct providers root") {
		t.Fatalf("ConstructResolutionService() error = %v, want providers root failure", err)
	}
}

func TestConstructResolutionServicePropagatesResolutionWireErrors(t *testing.T) {
	restoreProviders := settingsconstruct.SetConstructProvidersRootForTests(func() (providers.Service, error) {
		return &stubProvidersRoot{}, nil
	})
	t.Cleanup(restoreProviders)

	restoreResolution := settingsconstruct.SetConstructResolutionWireForTests(
		func(providers.Service) (resolution.Service, error) {
			return nil, errors.New("resolution wire failed")
		},
	)
	t.Cleanup(restoreResolution)

	_, err := settingsconstruct.ConstructResolutionService()
	if err == nil || !strings.Contains(err.Error(), "construct resolution service") {
		t.Fatalf("ConstructResolutionService() error = %v, want resolution wire failure", err)
	}
}

func testConfigDocumentService() operatorsettings.ConfigDocumentService {
	return settingsconstruct.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		testProviderCatalog,
		&sync.Mutex{},
	)
}

func testProviderCatalog(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		return "", false
	}
}

type stubProvidersRoot struct {
	providers.Service
}

func (stubProvidersRoot) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (stubProvidersRoot) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (stubProvidersRoot) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

var _ providers.Service = (*stubProvidersRoot)(nil)
