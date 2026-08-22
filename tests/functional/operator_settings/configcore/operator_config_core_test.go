package configcore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestOperatorConfigCore_ModelOverlaysRoundTripAndReportTypedFailures proves
// model overlays round-trip through operator configuration and invalid inputs
// report typed failures.
func TestOperatorConfigCore_ModelOverlaysRoundTripAndReportTypedFailures(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	help := support.FakeInputs(t.Context(), []string{"you", "session", "show", "--help"})
	if err := process.Execute(help.Input); err != nil {
		t.Fatalf("Process.Execute(session show help) error = %v", err)
	}

	initial := []byte(`{
  "backendScopeID": " scope-functional-operator ",
  "defaults": {"workerModelProvider": " CODEX ", "workerModel": " llm "},
  "models": {
    "llm": {"backend": " localai-llamacpp "},
    "custom-model": {
      "source": " hf://example/custom.gguf ",
      "backend": " localai-llamacpp ",
      "loadPolicy": " on_demand ",
      "operations": [" omni "]
    }
  },
  "runtime": {"logging": {"directory": "operator/logs", "maxSizeMB": 11}, "metrics": {"compress": true}},
  "workerPresets": [{"id": "research", "modelProvider": "CODEX", "model": "gpt-5"}]
}`)
	decoded, err := globalconfigmapping.Decode(initial)
	if err != nil {
		t.Fatalf("Decode(model overlays) error = %v", err)
	}
	if decoded.BackendScopeID != "scope-functional-operator" || decoded.Defaults.WorkerModel != "llm" {
		t.Fatalf("decoded identity/defaults = %#v", decoded)
	}
	llm, ok := decoded.Models["llm"]
	if !ok || llm.Backend == nil || *llm.Backend != "localai-llamacpp" || llm.Source != nil {
		t.Fatalf("decoded built-in overlay = %#v", llm)
	}
	custom, ok := decoded.Models["custom-model"]
	if !ok || custom.Source == nil || *custom.Source != "hf://example/custom.gguf" || custom.LoadPolicy == nil ||
		*custom.LoadPolicy != operatorsettings.ModelLoadPolicyOnDemand || len(custom.Operations) != 1 || custom.Operations[0] != "OMNI" {
		t.Fatalf("decoded complete model = %#v", custom)
	}
	clone := decoded.Clone()
	*clone.Models["llm"].Backend = "mutated"
	clone.Models["custom-model"].Operations[0] = "ASR"
	if *decoded.Models["llm"].Backend != "localai-llamacpp" || decoded.Models["custom-model"].Operations[0] != "OMNI" {
		t.Fatal("Config.Clone shared model overlay state")
	}

	encoded, err := globalconfigmapping.Encode(decoded)
	if err != nil {
		t.Fatalf("Encode(model overlays) error = %v", err)
	}
	roundTrip, err := globalconfigmapping.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode(encoded model overlays) error = %v", err)
	}
	if _, ok := roundTrip.Models["llm"]; !ok || roundTrip.Models["custom-model"].Operations[0] != "OMNI" ||
		roundTrip.WorkerPresets[0].ID != "research" || roundTrip.Runtime.Logging.Directory != "operator/logs" {
		t.Fatalf("round-trip config = %#v", roundTrip)
	}

	cases := []struct {
		name      string
		models    string
		wantModel string
		wantField string
	}{
		{name: "invalid name", models: `"bad name":{"backend":"backend"}`, wantModel: "bad name", wantField: "name"},
		{name: "empty source", models: `"llm":{"source":""}`, wantModel: "llm", wantField: "source"},
		{name: "unsupported policy", models: `"llm":{"loadPolicy":"ALWAYS"}`, wantModel: "llm", wantField: "loadPolicy"},
		{name: "malformed operation", models: `"llm":{"operations":["UNKNOWN"]}`, wantModel: "llm", wantField: "operations"},
		{name: "incomplete new model", models: `"custom":{"source":"hf://custom"}`, wantModel: "custom", wantField: "backend"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(`{"models":{` + test.models + `}}`)
			_, err := globalconfigmapping.Decode(payload)
			var failure operatorsettings.ConfigurationFailure
			if err == nil || !errors.Is(err, operatorsettings.ErrConfigurationInvalid) || !errors.As(err, &failure) {
				t.Fatalf("Decode(%s) error = %v, failure = %#v", test.name, err, failure)
			}
			if failure.ModelName != test.wantModel || failure.Field != test.wantField {
				t.Fatalf("configuration failure = %#v, want model=%q field=%q", failure, test.wantModel, test.wantField)
			}
		})
	}
}

func TestOperatorConfigCore_FutureFieldsWarnAndSurviveConfigRewrite(t *testing.T) {
	initial := []byte(`{
  "backendScopeID": "scope-functional-compat",
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "before",
    "futureDefault": {"enabled": true}
  },
  "models": {
    "llm": {"source": "hf://example/model", "futureModel": "preserve"}
  },
  "futureRoot": {"version": 2, "secret": "not-a-diagnostic"}
}`)

	config, diagnostics, err := globalconfigmapping.DecodeWithDiagnostics(initial)
	if err != nil {
		t.Fatalf("DecodeWithDiagnostics() error = %v", err)
	}
	if config.BackendScopeID != "scope-functional-compat" || config.Defaults.WorkerModel != "before" {
		t.Fatalf("known configuration = %#v, want identity and defaults preserved", config)
	}
	wantPaths := []string{
		"$.defaults.futureDefault",
		"$.futureRoot",
		"$.models.llm.futureModel",
	}
	paths := diagnostics.Paths()
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("ignored JSON paths = %#v, want %#v", paths, wantPaths)
	}
	if strings.Contains(strings.Join(paths, "\n"), "secret") {
		t.Fatalf("ignored JSON paths leaked a future value: %#v", paths)
	}

	canonical, err := globalconfigmapping.Encode(config)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	preserved, err := globalconfigmapping.PreserveUnknownFields(initial, canonical)
	if err != nil {
		t.Fatalf("PreserveUnknownFields() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(preserved, &document); err != nil {
		t.Fatalf("decode rewritten configuration: %v", err)
	}
	if !reflect.DeepEqual(document["futureRoot"], map[string]any{"version": float64(2), "secret": "not-a-diagnostic"}) {
		t.Fatalf("futureRoot after rewrite = %#v, want preserved future object", document["futureRoot"])
	}
	defaults, ok := document["defaults"].(map[string]any)
	if !ok || !reflect.DeepEqual(defaults["futureDefault"], map[string]any{"enabled": true}) {
		t.Fatalf("defaults after rewrite = %#v, want preserved future child", document["defaults"])
	}
	models, ok := document["models"].(map[string]any)
	if !ok || !reflect.DeepEqual(models["llm"].(map[string]any)["futureModel"], "preserve") {
		t.Fatalf("models after rewrite = %#v, want preserved future child", document["models"])
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
