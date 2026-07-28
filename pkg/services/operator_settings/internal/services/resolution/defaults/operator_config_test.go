package settingsresolution_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestDefaultRuntimeSettingsMatchProductionArtifactPolicies(t *testing.T) {
	cfg, err := operatorsettings.LoadFileConfig(testFiles, decodeTestConfig, filepath.Join(t.TempDir(), "missing-config.json"))
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	got := cfg.Runtime
	logDefaults := logging.DefaultRuntimeLogConfig()
	wantLogging := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  logDefaults.MaxSize,
		MaxBackups: logDefaults.MaxBackups,
		MaxAgeDays: logDefaults.MaxAge,
		Compress:   logDefaults.Compress,
	}
	if got.Logging != wantLogging {
		t.Fatalf("runtime logging defaults = %#v, want production defaults %#v", got.Logging, wantLogging)
	}

	metricsDefaults := platformmetrics.DefaultRuntimeMetricsConfig()
	wantMetrics := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  metricsDefaults.MaxSize,
		MaxBackups: metricsDefaults.MaxBackups,
		MaxAgeDays: metricsDefaults.MaxAge,
		Compress:   metricsDefaults.Compress,
	}
	if got.Metrics != wantMetrics {
		t.Fatalf("runtime metrics defaults = %#v, want production defaults %#v", got.Metrics, wantMetrics)
	}
}

func decodeTestConfig(data []byte) (operatorsettings.Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var generated *factoryapi.GlobalConfig
	if err := decoder.Decode(&generated); err != nil {
		return operatorsettings.Config{}, err
	}
	if generated == nil {
		return operatorsettings.Config{}, fmt.Errorf("expected a JSON object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return operatorsettings.Config{}, fmt.Errorf("unexpected trailing JSON")
	}
	config := operatorsettings.Config{}
	if generated.BackendScopeID != nil {
		config.BackendScopeID = *generated.BackendScopeID
	}
	if generated.Defaults != nil {
		if generated.Defaults.WorkerModelProvider != nil {
			config.Defaults.WorkerModelProvider = *generated.Defaults.WorkerModelProvider
		}
		if generated.Defaults.WorkerModel != nil {
			config.Defaults.WorkerModel = *generated.Defaults.WorkerModel
		}
	}
	if generated.WorkerPresets != nil {
		for _, preset := range *generated.WorkerPresets {
			mapped := operatorsettings.WorkerPreset{ID: preset.Id, ModelProvider: string(preset.ModelProvider)}
			if preset.Model != nil {
				mapped.Model = *preset.Model
			}
			if preset.ReasoningEffort != nil {
				mapped.ReasoningEffort = string(*preset.ReasoningEffort)
			}
			config.WorkerPresets = append(config.WorkerPresets, mapped)
		}
	}
	if generated.Workers != nil && generated.Workers.Acp != nil && generated.Workers.Acp.Integrations != nil {
		for _, integration := range *generated.Workers.Acp.Integrations {
			config.Workers.ACP.Integrations = append(config.Workers.ACP.Integrations, operatorsettings.ACPIntegration{
				ID: integration.Id, Name: integration.Name, Transport: string(integration.Transport), Command: integration.Command,
			})
		}
	}
	return config.Normalize()
}

func encodeTestConfig(config operatorsettings.Config) ([]byte, error) {
	generated := factoryapi.GlobalConfig{}
	if config.BackendScopeID != "" {
		generated.BackendScopeID = &config.BackendScopeID
	}
	if config.Defaults != (operatorsettings.Defaults{}) {
		generated.Defaults = &factoryapi.GlobalConfigDefaults{}
		if config.Defaults.WorkerModelProvider != "" {
			generated.Defaults.WorkerModelProvider = &config.Defaults.WorkerModelProvider
		}
		if config.Defaults.WorkerModel != "" {
			generated.Defaults.WorkerModel = &config.Defaults.WorkerModel
		}
	}
	if config.WorkerPresets != nil {
		presets := make([]factoryapi.GlobalConfigWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			presets[i] = factoryapi.GlobalConfigWorkerPreset{
				Id:            preset.ID,
				ModelProvider: factoryapi.GlobalConfigWorkerPresetModelProvider(preset.ModelProvider),
			}
			if preset.Model != "" {
				presets[i].Model = &preset.Model
			}
			if preset.ReasoningEffort != "" {
				effort := factoryapi.GlobalConfigWorkerPresetReasoningEffort(preset.ReasoningEffort)
				presets[i].ReasoningEffort = &effort
			}
		}
		generated.WorkerPresets = &presets
	}
	if config.Workers.ACP.Integrations != nil {
		integrations := make([]factoryapi.GlobalConfigACPIntegration, len(config.Workers.ACP.Integrations))
		for index, integration := range config.Workers.ACP.Integrations {
			integrations[index] = factoryapi.GlobalConfigACPIntegration{
				Id: integration.ID, Name: integration.Name, Command: integration.Command,
				Transport: factoryapi.GlobalConfigACPIntegrationTransport(integration.Transport),
			}
		}
		generated.Workers = &factoryapi.GlobalConfigWorkers{Acp: &factoryapi.GlobalConfigACPSettings{Integrations: &integrations}}
	}
	payload, err := json.MarshalIndent(generated, "", "  ")
	return append(payload, '\n'), err
}

func testConfigDocumentService() operatorsettings.ConfigDocumentService {
	return operatorsettings.ConfigDocumentService{Decoder: decodeTestConfig, Encoder: encodeTestConfig}
}

func TestLoadFileDefaults_MissingFileReturnsEmptyDefaults(t *testing.T) {
	defaults, err := operatorsettings.LoadFileDefaults(testFiles, decodeTestConfig, filepath.Join(t.TempDir(), "missing-config.json"))
	if err != nil {
		t.Fatalf("operatorsettings.LoadFileDefaults() error = %v", err)
	}
	if defaults != (operatorsettings.Defaults{}) {
		t.Fatalf("defaults = %#v, want empty", defaults)
	}
}

func TestLoadConfigDocument_AbsentFileProducesMergeableEmptyConfig(t *testing.T) {
	t.Parallel()
	service := testConfigDocumentService()
	service.Files = testFiles
	service.Providers = controlledProviderCatalog
	document, err := service.Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadConfigDocument() error = %v", err)
	}
	provider, model := " codex ", " gpt-custom "
	merged, err := service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &provider, Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if got := merged.FileConfig().Defaults; got != (operatorsettings.Defaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-custom"}) {
		t.Fatalf("defaults = %#v, want trimmed supplied values", got)
	}
	assertDocumentRoundTrip(t, merged)
}

func TestConfigDocumentServiceLoad_RequiresFilesystem(t *testing.T) {
	t.Parallel()
	_, err := (operatorsettings.ConfigDocumentService{}).Load("config.json")
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("Load() error = %v, want required filesystem", err)
	}
}

func TestMergeProviderModelDefaults_PreservesUnrelatedSemanticValues(t *testing.T) {
	t.Parallel()
	service := testConfigDocumentService()
	service.Providers = controlledProviderCatalog
	input := []byte(`{
  "backendScopeID": "local-11111111-1111-4111-8111-111111111111",
  "defaults": {"workerModelProvider": "claude", "workerModel": "old-model"},
  "workerPresets": [{"id":" research ","modelProvider":"openai","model":" preset-model ","reasoningEffort":" HIGH "}]
}`)
	document, err := service.Parse(input)
	if err != nil {
		t.Fatalf("ParseConfigDocument() error = %v", err)
	}
	before := document.FileConfig()
	provider, model := "gemini", "gemini-experimental"
	merged, err := service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &provider, Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	after := merged.FileConfig()
	if merged.BackendScopeID() != document.BackendScopeID() {
		t.Fatalf("backendScopeID = %q, want preserved %q", merged.BackendScopeID(), document.BackendScopeID())
	}
	if !reflect.DeepEqual(after.WorkerPresets, before.WorkerPresets) {
		t.Fatalf("worker presets = %#v, want preserved %#v", after.WorkerPresets, before.WorkerPresets)
	}
	if after.Defaults != (operatorsettings.Defaults{WorkerModelProvider: "GEMINI", WorkerModel: model}) {
		t.Fatalf("defaults = %#v, want supplied provider/model", after.Defaults)
	}
	assertDocumentRoundTrip(t, merged)
	if document.FileConfig().Defaults != before.Defaults {
		t.Fatal("merge mutated the input document")
	}
}

func TestMergeProviderModelDefaults_OmittedFieldsPreserveExistingDefaults(t *testing.T) {
	t.Parallel()
	service := testConfigDocumentService()
	service.Providers = controlledProviderCatalog
	document, err := service.Parse([]byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"existing-model"}}`))
	if err != nil {
		t.Fatalf("ParseConfigDocument() error = %v", err)
	}
	provider := "claude"
	merged, err := service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &provider})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if got := merged.FileConfig().Defaults; got != (operatorsettings.Defaults{WorkerModelProvider: "CLAUDE", WorkerModel: "existing-model"}) {
		t.Fatalf("defaults = %#v, want omitted model preserved", got)
	}
	clearModel := "  "
	cleared, err := service.MergeProviderModelDefaults(merged, operatorsettings.ProviderModelUpdate{Model: &clearModel})
	if err != nil {
		t.Fatalf("clear model: %v", err)
	}
	if got := cleared.FileConfig().Defaults.WorkerModel; got != "" {
		t.Fatalf("model = %q, want explicitly supplied empty value cleared", got)
	}
}

func TestMergeProviderModelDefaults_ValidatesProviderThroughInjectedCatalog(t *testing.T) {
	t.Parallel()
	document, err := testConfigDocumentService().Parse([]byte(`{"defaults":{"workerModelProvider":"CODEX","workerModel":"existing"}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	for _, test := range []struct {
		name     string
		provider string
		want     string
	}{
		{name: "alias", provider: " openai ", want: "CODEX"},
		{name: "canonical case insensitive", provider: "claude", want: "CLAUDE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := testConfigDocumentService()
			service.Providers = controlledProviderCatalog
			merged, mergeErr := service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &test.provider})
			if mergeErr != nil {
				t.Fatalf("MergeProviderModelDefaults() error = %v", mergeErr)
			}
			if got := merged.FileConfig().Defaults.WorkerModelProvider; got != test.want {
				t.Fatalf("provider = %q, want catalog canonical identity %q", got, test.want)
			}
		})
	}
}

func TestMergeProviderModelDefaults_RejectsInvalidRequiredProvider(t *testing.T) {
	t.Parallel()
	document, err := testConfigDocumentService().Parse([]byte(`{"defaults":{"workerModelProvider":"CODEX"}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	for _, test := range []struct {
		name      string
		provider  string
		catalog   operatorsettings.ProviderCatalog
		wantError string
	}{
		{name: "empty", provider: "  ", catalog: controlledProviderCatalog, wantError: "provider is required"},
		{name: "unsupported", provider: "other", catalog: controlledProviderCatalog, wantError: `unsupported worker model provider "other"`},
		{name: "catalog required", provider: "codex", wantError: "provider catalog is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := testConfigDocumentService()
			service.Providers = test.catalog
			_, mergeErr := service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &test.provider})
			if mergeErr == nil || !strings.Contains(mergeErr.Error(), test.wantError) {
				t.Fatalf("MergeProviderModelDefaults() error = %v, want %q", mergeErr, test.wantError)
			}
			if got := document.FileConfig().Defaults.WorkerModelProvider; got != "CODEX" {
				t.Fatalf("input provider = %q, want unchanged CODEX", got)
			}
		})
	}
}

func TestMergeProviderModelDefaults_AcceptsFreeFormModelWithoutCatalogLookup(t *testing.T) {
	t.Parallel()
	document, err := testConfigDocumentService().Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	model := " provider/private-model@next "
	merged, err := testConfigDocumentService().MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if got := merged.FileConfig().Defaults.WorkerModel; got != "provider/private-model@next" {
		t.Fatalf("model = %q, want trimmed free-form value", got)
	}
}

func controlledProviderCatalog(value string) (string, bool) {
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

func TestLoadConfigDocument_InvalidContentFailsBeforeMutation(t *testing.T) {
	t.Parallel()
	service := testConfigDocumentService()
	service.Files = testFiles
	for _, test := range []struct{ name, data string }{
		{name: "malformed", data: `{"defaults":`},
		{name: "trailing", data: `{} {}`},
		{name: "unknown", data: `{"unexpected":true}`},
		{name: "null", data: `null`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile before: %v", err)
			}
			_, err = service.Load(path)
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("LoadConfigDocument() error = %v, want invalid config error naming path", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile after: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("invalid load changed destination: got %q want %q", after, before)
			}
		})
	}
}

func assertDocumentRoundTrip(t *testing.T, document operatorsettings.ConfigDocument) {
	t.Helper()
	service := testConfigDocumentService()
	data, err := service.Marshal(document)
	if err != nil {
		t.Fatalf("MarshalConfigDocument() error = %v", err)
	}
	decodedDocument, err := service.Parse(data)
	if err != nil {
		t.Fatalf("Parse(encoded) error = %v", err)
	}
	decoded := decodedDocument.FileConfig()
	if !reflect.DeepEqual(decoded, document.FileConfig()) {
		t.Fatalf("decoded config = %#v, want %#v", decoded, document.FileConfig())
	}
}

func TestLoadFileDefaults_MalformedFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"defaults":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := operatorsettings.LoadFileDefaults(testFiles, decodeTestConfig, path)
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path %q", err.Error(), path)
	}
}

func TestParseFileDefaults_AcceptsBackendScopeIDAlongsideDefaults(t *testing.T) {
	cfg, err := decodeTestConfig([]byte(`{
		"backendScopeID": "local-11111111-1111-4111-8111-111111111111",
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5-codex"
		}
	}`))
	defaults := cfg.Defaults
	if err != nil {
		t.Fatalf("ParseFileDefaults() error = %v", err)
	}
	if defaults.WorkerModelProvider != "codex" {
		t.Fatalf("provider = %q, want codex", defaults.WorkerModelProvider)
	}
	if defaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", defaults.WorkerModel)
	}
}

func TestLoadFileDefaults_AcceptsBackendScopeIDAlongsideDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"backendScopeID": "local-22222222-2222-4222-8222-222222222222",
		"defaults": {
			"workerModelProvider": "claude",
			"workerModel": "claude-sonnet"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	defaults, err := operatorsettings.LoadFileDefaults(testFiles, decodeTestConfig, path)
	if err != nil {
		t.Fatalf("operatorsettings.LoadFileDefaults() error = %v", err)
	}
	if defaults.WorkerModelProvider != "claude" {
		t.Fatalf("provider = %q, want claude", defaults.WorkerModelProvider)
	}
	if defaults.WorkerModel != "claude-sonnet" {
		t.Fatalf("model = %q, want claude-sonnet", defaults.WorkerModel)
	}
}

func TestParseFileDefaults_AcceptsWorkerModelDefaults(t *testing.T) {
	cfg, err := decodeTestConfig([]byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5-codex"
		}
	}`))
	defaults := cfg.Defaults
	if err != nil {
		t.Fatalf("ParseFileDefaults() error = %v", err)
	}
	if defaults.WorkerModelProvider != "codex" {
		t.Fatalf("provider = %q, want codex", defaults.WorkerModelProvider)
	}
	if defaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", defaults.WorkerModel)
	}
}

func TestParseFileConfig_ValidatesAndCanonicalizesWorkerPresets(t *testing.T) {
	cfg, err := decodeTestConfig([]byte(`{
		"workerPresets": [{
			"id": " research ",
			"modelProvider": "openai",
			"model": " gpt-5.4 ",
			"reasoningEffort": " HIGH "
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseFileConfig() error = %v", err)
	}
	want := operatorsettings.WorkerPreset{ID: "research", ModelProvider: "CODEX", Model: "gpt-5.4", ReasoningEffort: "high"}
	if len(cfg.WorkerPresets) != 1 || cfg.WorkerPresets[0] != want {
		t.Fatalf("worker presets = %#v, want %#v", cfg.WorkerPresets, []operatorsettings.WorkerPreset{want})
	}
}

func TestParseFileConfig_MissingWorkerPresetsIsBackwardCompatible(t *testing.T) {
	cfg, err := decodeTestConfig([]byte(`{"defaults":{"workerModel":"existing-model"}}`))
	if err != nil {
		t.Fatalf("ParseFileConfig() error = %v", err)
	}
	if cfg.Defaults.WorkerModel != "existing-model" || len(cfg.WorkerPresets) != 0 {
		t.Fatalf("config = %#v, want existing defaults and no presets", cfg)
	}
}

func TestParseFileConfig_RejectsInvalidWorkerPresets(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{name: "empty id", json: `{"workerPresets":[{"id":"  ","modelProvider":"codex"}]}`, want: []string{`workerPresets[0].id`, `"  "`, "non-empty"}},
		{name: "duplicate id", json: `{"workerPresets":[{"id":"build","modelProvider":"codex"},{"id":" build ","modelProvider":"claude"}]}`, want: []string{`workerPresets[1].id`, `"build"`, "duplicated"}},
		{name: "missing provider", json: `{"workerPresets":[{"id":"build"}]}`, want: []string{`workerPresets[0]`, `"build"`, "modelProvider"}},
		{name: "symbolic provider", json: `{"workerPresets":[{"id":"build","modelProvider":"DEFAULT"}]}`, want: []string{`"build"`, `"DEFAULT"`, "unsupported modelProvider"}},
		{name: "malformed provider", json: `{"workerPresets":[{"id":"build","modelProvider":"Other_Provider"}]}`, want: []string{`"build"`, `"Other_Provider"`, "unsupported modelProvider"}},
		{name: "unsupported reasoning", json: `{"workerPresets":[{"id":"build","modelProvider":"codex","reasoningEffort":"extreme"}]}`, want: []string{`"build"`, `"extreme"`, "unsupported reasoningEffort"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeTestConfig([]byte(tt.json))
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, fragment := range tt.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}
}

func TestLoadFileDefaults_RejectsMalformedWorkerPresets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"workerPresets":[{"id":"bad","modelProvider":"Unknown_Provider"}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := operatorsettings.LoadFileDefaults(testFiles, decodeTestConfig, path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("operatorsettings.LoadFileDefaults() error = %v, want validation error naming %q", err, path)
	}
}

func TestDefaultConfigPathUsesCanonicalDefaultPathsPolicy(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(string(filepath.Separator), "tmp", "operator-home")

	if got, want := operatorsettings.DefaultConfigPath(homeDir), filepath.Join(homeDir, ".you-agent-factory", "config.json"); got != want {
		t.Fatalf("operatorsettings.DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestResolve_FileEnvFlagPrecedenceIsIndependentPerField(t *testing.T) {
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		Env: operatorsettings.Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		Flag: operatorsettings.Defaults{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("operatorsettings.Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "flag-model" {
		t.Fatalf("model = %q, want flag-model", resolved.WorkerModel)
	}
	if resolved.WorkerModelProviderSource != operatorsettings.SourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.WorkerModelProviderSource)
	}
	if resolved.WorkerModelSource != operatorsettings.SourceFlag {
		t.Fatalf("model source = %q, want flag", resolved.WorkerModelSource)
	}
}

func TestResolve_EnvOverridesFileWhenFlagsUnset(t *testing.T) {
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		Env: operatorsettings.Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("operatorsettings.Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", resolved.WorkerModel)
	}
	if resolved.WorkerModelProviderSource != operatorsettings.SourceEnv {
		t.Fatalf("provider source = %q, want env", resolved.WorkerModelProviderSource)
	}
	if resolved.WorkerModelSource != operatorsettings.SourceEnv {
		t.Fatalf("model source = %q, want env", resolved.WorkerModelSource)
	}
}

func TestResolve_CanonicalizesProviderAliases(t *testing.T) {
	for _, test := range []struct {
		alias     string
		canonical string
	}{
		{alias: "anthropic", canonical: "CLAUDE"},
		{alias: "openai", canonical: "CODEX"},
		{alias: "agent", canonical: "CURSOR"},
		{alias: "cursor-agent", canonical: "CURSOR"},
		{alias: "kiro-cli", canonical: "KIRO"},
		{alias: "antigravity", canonical: "AGY"},
	} {
		test := test
		t.Run(test.alias, func(t *testing.T) {
			resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
				File: operatorsettings.Defaults{WorkerModelProvider: test.alias},
			}, "/tmp/config.json")
			if err != nil {
				t.Fatalf("operatorsettings.Resolve() error = %v", err)
			}
			if resolved.WorkerModelProvider != test.canonical {
				t.Fatalf("provider = %q, want %q", resolved.WorkerModelProvider, test.canonical)
			}
		})
	}
}

func TestResolve_SymbolicDefaultResolvesThroughLowerPrecedenceConcreteProvider(t *testing.T) {
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{WorkerModelProvider: "codex"},
		Flag: operatorsettings.Defaults{WorkerModelProvider: "DEFAULT"},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("operatorsettings.Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModelProviderSource != operatorsettings.SourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.WorkerModelProviderSource)
	}
}

func TestResolve_SymbolicDefaultWithoutConcreteProviderFails(t *testing.T) {
	_, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		Flag: operatorsettings.Defaults{WorkerModelProvider: "DEFAULT"},
	}, "/tmp/config.json")
	if err == nil {
		t.Fatal("expected unresolved DEFAULT error")
	}
	if !strings.Contains(err.Error(), "concrete provider") {
		t.Fatalf("error = %q, want concrete provider guidance", err.Error())
	}
}

func TestResolve_PreservesExplicitConfigPathOverride(t *testing.T) {
	t.Parallel()

	overridePath := filepath.Join(string(filepath.Separator), "tmp", "custom", "operator-config.json")
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "file-model",
		},
	}, overridePath)
	if err != nil {
		t.Fatalf("operatorsettings.Resolve() error = %v", err)
	}
	if resolved.ConfigPath != overridePath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, overridePath)
	}
}

func TestResolve_MalformedProviderFailsWithIdentitySyntax(t *testing.T) {
	_, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{WorkerModelProvider: "Not_A_Provider"},
	}, "/tmp/config.json")
	if err == nil {
		t.Fatal("expected malformed provider error")
	}
	if !strings.Contains(err.Error(), "unsupported worker model provider") {
		t.Fatalf("error = %q, want unsupported provider message", err.Error())
	}
	if !strings.Contains(err.Error(), "canonical lowercase provider identity") {
		t.Fatalf("error = %q, want provider identity syntax", err.Error())
	}
}

func TestResolve_PreservesExtensionProviderFromCLIOverride(t *testing.T) {
	const identity = "customer.provider-v2"
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		Flag: operatorsettings.Defaults{WorkerModelProvider: identity},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("operatorsettings.Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != identity || resolved.WorkerModelProviderSource != operatorsettings.SourceFlag {
		t.Fatalf("resolved provider = %#v, want CLI identity %q", resolved, identity)
	}
}

func TestResolveFromHome_LoadsFileAndEnvironment(t *testing.T) {
	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "claude",
			"workerModel": "file-model"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "env-model")

	resolved, err := operatorsettings.ResolveFromHomeWithEnvironment(testFiles, decodeTestConfig, homeDir, operatorsettings.Defaults{
		WorkerModelProvider: os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider),
		WorkerModel:         os.Getenv(operatorsettings.EnvDefaultWorkerModel),
	}, operatorsettings.FlagOverrides{})
	if err != nil {
		t.Fatalf("ResolveFromHome() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", resolved.WorkerModel)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, configPath)
	}
}

func TestResolveFromHome_FlagsOverrideEnvironmentAndFile(t *testing.T) {
	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "claude",
			"workerModel": "file-model"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "env-model")

	resolved, err := operatorsettings.ResolveFromHomeWithEnvironment(testFiles, decodeTestConfig, homeDir, operatorsettings.Defaults{
		WorkerModelProvider: os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider),
		WorkerModel:         os.Getenv(operatorsettings.EnvDefaultWorkerModel),
	}, operatorsettings.FlagOverrides{
		WorkerModelProvider: "gemini",
		WorkerModel:         "flag-model",
	})
	if err != nil {
		t.Fatalf("ResolveFromHome() error = %v", err)
	}
	if resolved.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "flag-model" {
		t.Fatalf("model = %q, want flag-model", resolved.WorkerModel)
	}
}
