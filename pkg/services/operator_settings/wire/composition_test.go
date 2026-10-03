package wire_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

func TestNewServiceFromConfigDocumentRequiresDocumentPorts(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromConfigDocument(
		operatorsettings.ConfigDocumentService{},
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		logging.NoopLogger{},
	)
	if err == nil || !strings.Contains(err.Error(), "operator settings document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

func TestNewServiceFromConfigDocumentConstructsFromPorts(t *testing.T) {
	t.Parallel()

	root, err := settingswire.NewServiceFromConfigDocument(
		testConfigDocumentService(),
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		logging.NoopLogger{},
	)
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
	service.DocumentOwner = settingswire.NewDocumentOwner(
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		service.Providers,
		nil,
	)

	root, err := settingswire.NewServiceFromConfigDocument(
		service,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestNewConfigDocumentServiceUsesCompletedDocumentForRoundTrip(t *testing.T) {
	t.Parallel()

	owner := settingswire.NewDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		testProviderCatalog,
		nil,
		nil,
	)
	service := settingswire.NewConfigDocumentService(owner, globalconfigmapping.Decode, globalconfigmapping.Encode, &sync.Mutex{})
	path := filepath.Join(t.TempDir(), "config.json")
	provider, model := "codex", "completed-model"
	updated, err := service.ConfigureProviderModel(context.Background(), path, operatorsettings.ProviderModelUpdate{
		Provider: &provider, Model: &model,
	})
	if err != nil {
		t.Fatalf("ConfigureProviderModel() = %v", err)
	}
	want := operatorsettings.Defaults{WorkerModelProvider: "CODEX", WorkerModel: "completed-model"}
	if updated.FileConfig().Defaults != want {
		t.Fatalf("configured defaults = %#v, want %#v", updated.FileConfig().Defaults, want)
	}
	reloaded, err := service.Load(path)
	if err != nil || reloaded.FileConfig().Defaults != want {
		t.Fatalf("Load() = %#v, %v, want persisted defaults %#v", reloaded.FileConfig(), err, want)
	}
}

func TestWireCompositionDelegatesDocumentAndResolutionOperations(t *testing.T) {
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

	root := newPreservationWireService(t)

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

func TestNewServiceFromConfigDocumentRejectsMissingIDGenerator(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromConfigDocument(
		testConfigDocumentService(),
		internaltestproviders.StandardCatalog(),
		nil,
		logging.NoopLogger{},
	)
	if err == nil || err.Error() != "operator settings ID generator is required" {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want missing ID generator", err)
	}
}

func testConfigDocumentService() operatorsettings.ConfigDocumentService {
	files := platformfilesystem.Local{}
	create := func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
	// The remaining legacy root-constructor classifications still require their
	// old port view until that construction entry is retired in story 003.
	return operatorsettings.ConfigDocumentService{
		Files: files, CreateTemp: create, Providers: testProviderCatalog,
		Decoder: globalconfigmapping.Decode, Encoder: globalconfigmapping.Encode,
		PersistenceLock: &sync.Mutex{},
		DocumentOwner: settingswire.NewDocumentService(files, create, globalconfigmapping.Decode,
			globalconfigmapping.Encode, testProviderCatalog, nil, nil),
	}
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

// newWireCompositionRoot prepares the public Settings construction contract.
func newWireCompositionRoot(t *testing.T) (operatorsettings.Service, string) {
	t.Helper()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "openai",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	root := newPreservationWireService(t)
	return root, configPath
}

// TestWireCompositionServesDocumentAndResolutionOperations characterizes the
// completed-owner construction contract; it does not prove transport activation.
func TestWireCompositionServesDocumentAndResolutionOperations(t *testing.T) {
	t.Parallel()

	root, configPath := newWireCompositionRoot(t)

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "openai" {
		t.Fatalf("loaded provider = %q, want openai from file", loaded.Document.Defaults.WorkerModelProvider)
	}

	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: loaded.Document.Defaults,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "claude",
			WorkerModel:         "claude-sonnet",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() error = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "CLAUDE" || resolved.Selection.WorkerModel != "claude-sonnet" {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
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
}

// TestWireCompositionResolvesDefaultACPAgentProfileWhenAbsent covers
func TestWireCompositionResolvesDefaultACPAgentProfileWhenAbsent(t *testing.T) {
	t.Parallel()

	root, configPath := newWireCompositionRoot(t)

	defaultProfile, err := root.ResolveACPAgentProfile(configPath)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	wantDefaultProfile := operatorsettings.DefaultACPAgentProfile()
	if defaultProfile.DefaultTarget != wantDefaultProfile.DefaultTarget ||
		!reflect.DeepEqual(defaultProfile.AllowedTargets, wantDefaultProfile.AllowedTargets) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want safe Factory Builder default %#v", defaultProfile, wantDefaultProfile)
	}
}

// TestWireCompositionUpdatesACPAgentProfileAndPreservesSiblingSettings covers
func TestWireCompositionUpdatesACPAgentProfileAndPreservesSiblingSettings(t *testing.T) {
	t.Parallel()

	root, configPath := newWireCompositionRoot(t)

	authoredProfile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/research",
		AllowedTargets: []string{"factory:@you/research", "factory:@you/factory-builder"},
	}
	updatedProfile, err := root.UpdateACPAgentProfile(t.Context(), configPath, authoredProfile)
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}
	if updatedProfile.DefaultTarget != authoredProfile.DefaultTarget {
		t.Fatalf("UpdateACPAgentProfile() default = %q, want %q", updatedProfile.DefaultTarget, authoredProfile.DefaultTarget)
	}

	resolvedProfile, err := root.ResolveACPAgentProfile(configPath)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after update error = %v", err)
	}
	if resolvedProfile.DefaultTarget != authoredProfile.DefaultTarget ||
		len(resolvedProfile.AllowedTargets) != len(authoredProfile.AllowedTargets) {
		t.Fatalf("ResolveACPAgentProfile() after update = %#v, want %#v", resolvedProfile, authoredProfile)
	}

	provider := "gemini"
	model := "gemini-pro"
	if updatedAgain, err := root.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: configPath,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &provider,
			Model:    &model,
		},
	}); err != nil {
		t.Fatalf("ApplyDocumentUpdate() after profile update error = %v", err)
	} else if updatedAgain.Document.Workers.ACP.AgentProfile == nil ||
		updatedAgain.Document.Workers.ACP.AgentProfile.DefaultTarget != authoredProfile.DefaultTarget {
		t.Fatalf(
			"ApplyDocumentUpdate() after profile update = %#v, want authored profile preserved",
			updatedAgain.Document.Workers.ACP.AgentProfile,
		)
	}
}

// TestWireCompositionUpdateACPAgentProfileRejectsBlankCandidate covers
func TestWireCompositionUpdateACPAgentProfileRejectsBlankCandidate(t *testing.T) {
	t.Parallel()

	root, configPath := newWireCompositionRoot(t)

	if _, err := root.UpdateACPAgentProfile(t.Context(), configPath, operatorsettings.ACPAgentProfile{}); err == nil {
		t.Fatal("UpdateACPAgentProfile() with blank candidate error = nil, want validation failure")
	}
}

func TestWireCompositionFromHomePortsConstructsSettingsRoot(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	root, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}

func TestWireCompositionFromHomePortsRejectsMissingPorts(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" }, logging.NoopLogger{})
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}

	_, err = settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" }, logging.NoopLogger{})
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestWireCompositionRegisterDefaultsResolutionFromHomeRestoresAdapterOwnership(t *testing.T) {
	t.Parallel()

	settingswire.RegisterDefaultsResolutionFromHome()
}

func TestResolveFromHomeRejectsMissingFilesystemPorts(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	_, err := settingswire.NewServiceFromHomePorts(
		nil,
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err == nil || !strings.Contains(err.Error(), "operator settings filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want home-port construction failure", err)
	}
}

func TestResolveFromHomeUsesSettingsAdapterOwnershipPath(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "openai",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	providersRoot := internaltestproviders.StandardCatalog()
	settingsRoot, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	resolved, err := settingsRoot.ResolveFromHomeWithEnvironment(
		homeDir,
		operatorsettings.Defaults{},
		operatorsettings.FlagOverrides{},
	)
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX from adapter ownership path", resolved.WorkerModelProvider)
	}
}

func TestWireCompositionFromConfigDocumentConstructsFromDocumentPorts(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	root, err := settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{
		Files:     platformfilesystem.Local{},
		Decoder:   globalconfigmapping.Decode,
		Encoder:   globalconfigmapping.Encode,
		Providers: wireCompositionProviderCatalog,
		CreateTemp: func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
	}, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" }, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestWireCompositionFromConfigDocumentRejectsMissingDocumentPorts(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()
	_, err := settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{}, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" }, logging.NoopLogger{})
	if err == nil || !strings.Contains(err.Error(), "operator settings document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

func wireCompositionProviderCatalog(value string) (string, bool) {
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
