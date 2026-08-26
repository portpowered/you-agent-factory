package settingsresolution_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func decodeTestConfig(data []byte) (operatorsettings.Config, error) {
	config, diagnostics, err := globalconfigmapping.DecodeWithDiagnostics(data)
	if err != nil {
		return operatorsettings.Config{}, err
	}
	if paths := diagnostics.Paths(); len(paths) > 0 {
		return operatorsettings.Config{}, fmt.Errorf("json: unknown field %q", paths[0])
	}
	return config, nil
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
	return operatorsettings.ConfigDocumentService{
		Files:           testFiles,
		CreateTemp:      testCreateTemp,
		Providers:       controlledProviderCatalog,
		Decoder:         decodeTestConfig,
		Encoder:         encodeTestConfig,
		PersistenceLock: &sync.Mutex{},
		DocumentOwner: settingswire.NewDocumentOwner(
			testFiles,
			testCreateTemp,
			decodeTestConfig,
			encodeTestConfig,
			controlledProviderCatalog,
		),
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

func TestDefaultConfigPathUsesCanonicalDefaultPathsPolicy(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(string(filepath.Separator), "tmp", "operator-home")

	if got, want := operatorsettings.DefaultConfigPath(homeDir), filepath.Join(homeDir, ".you-agent-factory", "config.json"); got != want {
		t.Fatalf("operatorsettings.DefaultConfigPath() = %q, want %q", got, want)
	}
}
