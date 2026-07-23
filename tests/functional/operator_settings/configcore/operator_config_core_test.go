package configcore

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

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestOperatorConfigCore_PromptedAndPresuppliedUpdatesShareAtomicBehavior(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "operator", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	initial := []byte(`{
  "backendScopeID": "local-11111111-1111-4111-8111-111111111111",
  "defaults": {"workerModelProvider": "CLAUDE", "workerModel": "old-model"},
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
	if prompted.BackendScopeID() != "local-11111111-1111-4111-8111-111111111111" {
		t.Fatalf("backend scope = %q, want preserved identity", prompted.BackendScopeID())
	}

	beforeCanceled := append([]byte(nil), persisted...)
	_, err = service.ConfigureProviderModelPrompted(
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

func operatorConfigService() operatorsettings.ConfigDocumentService {
	return operatorsettings.ConfigDocumentService{
		Files: operatorConfigOS{},
		CreateTemp: func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Providers: func(value string) (string, bool) {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "codex", "openai":
				return "CODEX", true
			case "claude", "anthropic":
				return "CLAUDE", true
			default:
				return "", false
			}
		},
		Decoder:         globalconfigmapping.Decode,
		Encoder:         globalconfigmapping.Encode,
		PersistenceLock: &sync.Mutex{},
	}
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
