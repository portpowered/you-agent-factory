package root_composition_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

// TestWireCompositionServesDocumentAndResolutionOperations exercises published
// Settings wire surfaces through the functional lane so transitional composition
// hooks (construct/testlink/testproviders) retain coverage after servicewire/
// retargeting.
func TestWireCompositionServesDocumentAndResolutionOperations(t *testing.T) {
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

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromConfigDocument(
		settingswire.NewConfigDocumentService(
			platformfilesystem.Local{},
			func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
				return os.CreateTemp(dir, pattern)
			},
			globalconfigmapping.Decode,
			globalconfigmapping.Encode,
			wireCompositionProviderCatalog,
			&sync.Mutex{},
		),
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}

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

	defaultProfile, err := root.ResolveACPAgentProfile(configPath)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	wantDefaultProfile := operatorsettings.DefaultACPAgentProfile()
	if defaultProfile.DefaultTarget != wantDefaultProfile.DefaultTarget ||
		!reflect.DeepEqual(defaultProfile.AllowedTargets, wantDefaultProfile.AllowedTargets) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want safe Factory Builder default %#v", defaultProfile, wantDefaultProfile)
	}

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

	if _, err := root.UpdateACPAgentProfile(t.Context(), configPath, operatorsettings.ACPAgentProfile{}); err == nil {
		t.Fatal("UpdateACPAgentProfile() with blank candidate error = nil, want validation failure")
	}
}

func TestWireCompositionFromHomePortsConstructsSettingsRoot(t *testing.T) {
	t.Parallel()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
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

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	_, err = settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" })
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}

	_, err = settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" })
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

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	_, err = settingswire.NewServiceFromHomePorts(
		nil,
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
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

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	settingsRoot, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
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

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{
		Files:     platformfilesystem.Local{},
		Decoder:   globalconfigmapping.Decode,
		Encoder:   globalconfigmapping.Encode,
		Providers: wireCompositionProviderCatalog,
		CreateTemp: func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
	}, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" })
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestWireCompositionFromConfigDocumentRejectsMissingDocumentPorts(t *testing.T) {
	t.Parallel()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	_, err = settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{}, providersRoot, func() string { return "00000000-0000-4000-8000-000000000001" })
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
