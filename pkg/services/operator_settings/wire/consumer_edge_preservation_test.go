package wire_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
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

// TestNewServicePreservesDocumentPersistAndEffectiveResolution proves document
// load/update/atomic persist and effective resolution keep the same Settings-
// observable outcomes after the Settings→Providers consumer cut when the wire
// composes resolution against an injected providers.Service root.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestNewServicePreservesDocumentPersistAndEffectiveResolution(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) = %v", err)
	}

	root := newPreservationWireService(t)

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" ||
		loaded.Document.Defaults.WorkerModel != "gpt-5" {
		t.Fatalf("LoadDocument() defaults = %#v, want codex/gpt-5", loaded.Document.Defaults)
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
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if !updated.Persisted {
		t.Fatal("ApplyDocumentUpdate() Persisted = false, want true after atomic persist")
	}
	if updated.Document.Defaults.WorkerModelProvider != "GEMINI" ||
		updated.Document.Defaults.WorkerModel != "gemini-pro" {
		t.Fatalf("ApplyDocumentUpdate() defaults = %#v, want GEMINI/gemini-pro", updated.Document.Defaults)
	}

	reloaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() after persist = %v", err)
	}
	if reloaded.Document.Defaults != updated.Document.Defaults {
		t.Fatalf("reloaded defaults = %#v, want %#v", reloaded.Document.Defaults, updated.Document.Defaults)
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
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
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

func newPreservationWireService(t *testing.T) operatorsettings.Service {
	t.Helper()

	files := platformfilesystem.Local{}
	createTemp := func(dir, pattern string) (operatorsettings.TemporaryFile, error) { return os.CreateTemp(dir, pattern) }
	document := settingswire.NewDocumentService(files, createTemp, globalconfigmapping.Decode,
		globalconfigmapping.Encode, preservationProviderCatalog, nil, nil)
	resolution, err := settingswire.NewResolutionService(internaltestproviders.StandardCatalog())
	if err != nil {
		t.Fatalf("NewResolutionService() = %v", err)
	}
	service, err := settingswire.NewService(document, resolution, files, createTemp,
		globalconfigmapping.Decode, globalconfigmapping.Encode, testIDGenerator(), logging.NoopLogger{}, nil)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	return service
}

func preservationProviderCatalog(value string) (string, bool) {
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

func TestOperatorConfigCore_PromptedAndPresuppliedUpdatesShareAtomicBehavior(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "operator", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	initial := []byte(`{
  "backendScopeID": "local-11111111-1111-4111-8111-111111111111",
  "defaults": {"workerModelProvider": "CLAUDE", "workerModel": "old-model"},
  "runtime": {
    "logging": {"directory":"custom/logs","maxSizeMB":11,"maxBackups":12,"maxAgeDays":13,"compress":true},
    "metrics": {"directory":"custom/metrics","maxSizeMB":21,"maxBackups":22,"maxAgeDays":23,"compress":false}
  },
  "workerPresets": [{"id":"research","modelProvider":"CODEX","model":"gpt-5"}]
}`)
	if err := os.WriteFile(configPath, initial, 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}
	service := operatorConfigService()
	provider, model := " openai ", " provider/private-model@next "
	configured, err := service.ConfigureProviderModel(context.Background(), configPath, operatorsettings.ProviderModelUpdate{
		Provider: &provider,
		Model:    &model,
	})
	if err != nil {
		t.Fatalf("configure pre-supplied defaults: %v", err)
	}
	assertOperatorConfig(t, configured, "CODEX", "provider/private-model@next")

	promptCalled := false
	prompted, err := service.ConfigureProviderModelPrompted(
		context.Background(),
		configPath,
		func(_ context.Context, defaults operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
			promptCalled = true
			if defaults.WorkerModelProvider != "CODEX" || defaults.WorkerModel != "provider/private-model@next" {
				t.Fatalf("prompt defaults = %#v, want committed pre-supplied defaults", defaults)
			}
			nextProvider, nextModel := "anthropic", "claude-future"
			return operatorsettings.ProviderModelUpdate{Provider: &nextProvider, Model: &nextModel}, nil
		},
	)
	if err != nil {
		t.Fatalf("configure prompted defaults: %v", err)
	}
	if !promptCalled {
		t.Fatal("prompt was not called")
	}
	assertOperatorConfig(t, prompted, "CLAUDE", "claude-future")

	persisted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	decoded, err := globalconfigmapping.Decode(persisted)
	if err != nil {
		t.Fatalf("parse persisted config: %v", err)
	}
	if decoded.Defaults != (operatorsettings.Defaults{WorkerModelProvider: "CLAUDE", WorkerModel: "claude-future"}) {
		t.Fatalf("persisted defaults = %#v", decoded.Defaults)
	}
	if len(decoded.WorkerPresets) != 1 || decoded.WorkerPresets[0].ID != "research" {
		t.Fatalf("persisted worker presets = %#v, want preserved research preset", decoded.WorkerPresets)
	}
	assertRuntimePreserved(t, decoded.Runtime)
	if prompted.BackendScopeID() != "local-11111111-1111-4111-8111-111111111111" {
		t.Fatalf("backend scope = %q, want preserved identity", prompted.BackendScopeID())
	}

	assertCanceledAndRejectedConfigUpdates(t, service, configPath, persisted)

}

func assertRuntimePreserved(t *testing.T, got operatorsettings.RuntimeSettings) {
	t.Helper()
	wantRuntime := operatorsettings.RuntimeSettings{
		Logging: operatorsettings.RuntimeArtifactSettings{
			Directory: "custom/logs", MaxSizeMB: 11, MaxBackups: 12, MaxAgeDays: 13, Compress: true,
		},
		Metrics: operatorsettings.RuntimeArtifactSettings{
			Directory: "custom/metrics", MaxSizeMB: 21, MaxBackups: 22, MaxAgeDays: 23,
		},
	}
	if got != wantRuntime {
		t.Fatalf("persisted runtime = %#v, want preserved %#v", got, wantRuntime)
	}
}

func operatorConfigService() operatorsettings.ConfigDocumentService {
	return settingswire.NewConfigDocumentService(
		operatorConfigOS{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		func(value string) (string, bool) {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "codex", "openai":
				return "CODEX", true
			case "claude", "anthropic":
				return "CLAUDE", true
			default:
				return "", false
			}
		},
		&sync.Mutex{},
	)
}

func assertOperatorConfig(t *testing.T, document operatorsettings.ConfigDocument, provider, model string) {
	t.Helper()
	if got := document.FileConfig().Defaults; got != (operatorsettings.Defaults{WorkerModelProvider: provider, WorkerModel: model}) {
		t.Fatalf("configured defaults = %#v, want provider %q model %q", got, provider, model)
	}
}

type operatorConfigOS struct{}

func (operatorConfigOS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (operatorConfigOS) MkdirAll(path string, permissions fs.FileMode) error {
	return os.MkdirAll(path, permissions)
}

func (operatorConfigOS) Remove(path string) error { return os.Remove(path) }

func (operatorConfigOS) Chmod(path string, permissions fs.FileMode) error {
	return os.Chmod(path, permissions)
}

func (operatorConfigOS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func assertCanceledAndRejectedConfigUpdates(t *testing.T, service operatorsettings.ConfigDocumentService, configPath string, persisted []byte) {
	t.Helper()
	beforeCanceled := append([]byte(nil), persisted...)
	_, err := service.ConfigureProviderModelPrompted(
		context.Background(),
		configPath,
		func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
			return operatorsettings.ProviderModelUpdate{}, io.EOF
		},
	)
	if !errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) {
		t.Fatalf("prompt EOF error = %v, want cancellation", err)
	}
	afterCanceled, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after canceled prompt: %v", err)
	}
	if !reflect.DeepEqual(afterCanceled, beforeCanceled) {
		t.Fatal("canceled prompt changed the committed config")
	}

	unsupported := "unknown-provider"
	_, err = service.ConfigureProviderModel(context.Background(), configPath, operatorsettings.ProviderModelUpdate{Provider: &unsupported})
	if err == nil || !strings.Contains(err.Error(), "unsupported worker model provider") {
		t.Fatalf("unsupported provider error = %v", err)
	}
	afterRejected, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after rejected provider: %v", err)
	}
	if !reflect.DeepEqual(afterRejected, beforeCanceled) {
		t.Fatal("rejected provider changed the committed config")
	}
}
