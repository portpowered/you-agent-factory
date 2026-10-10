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
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

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

func TestResolveFromHomeUsesCompletedSettingsOwners(t *testing.T) {
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

	settingsRoot := newPreservationWireService(t)
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
