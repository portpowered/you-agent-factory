package operatorsettings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFileDefaults_MissingFileReturnsEmptyDefaults(t *testing.T) {
	defaults, err := LoadFileDefaults(testFiles, filepath.Join(t.TempDir(), "missing-config.json"))
	if err != nil {
		t.Fatalf("LoadFileDefaults() error = %v", err)
	}
	if defaults != (Defaults{}) {
		t.Fatalf("defaults = %#v, want empty", defaults)
	}
}

func TestLoadConfigDocument_AbsentFileProducesMergeableEmptyConfig(t *testing.T) {
	t.Parallel()
	service := ConfigDocumentService{Files: testFiles, Providers: controlledProviderCatalog}
	document, err := service.Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadConfigDocument() error = %v", err)
	}
	provider, model := " codex ", " gpt-custom "
	merged, err := service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &provider, Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if got := merged.FileConfig().Defaults; got != (Defaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-custom"}) {
		t.Fatalf("defaults = %#v, want trimmed supplied values", got)
	}
	assertDocumentRoundTrip(t, merged)
}

func TestConfigDocumentServiceLoad_RequiresFilesystem(t *testing.T) {
	t.Parallel()
	_, err := (ConfigDocumentService{}).Load("config.json")
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("Load() error = %v, want required filesystem", err)
	}
}

func TestMergeProviderModelDefaults_PreservesUnrelatedSemanticValues(t *testing.T) {
	t.Parallel()
	service := ConfigDocumentService{Providers: controlledProviderCatalog}
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
	merged, err := service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &provider, Model: &model})
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
	if after.Defaults != (Defaults{WorkerModelProvider: "GEMINI", WorkerModel: model}) {
		t.Fatalf("defaults = %#v, want supplied provider/model", after.Defaults)
	}
	assertDocumentRoundTrip(t, merged)
	if document.FileConfig().Defaults != before.Defaults {
		t.Fatal("merge mutated the input document")
	}
}

func TestMergeProviderModelDefaults_OmittedFieldsPreserveExistingDefaults(t *testing.T) {
	t.Parallel()
	service := ConfigDocumentService{Providers: controlledProviderCatalog}
	document, err := service.Parse([]byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"existing-model"}}`))
	if err != nil {
		t.Fatalf("ParseConfigDocument() error = %v", err)
	}
	provider := "claude"
	merged, err := service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &provider})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if got := merged.FileConfig().Defaults; got != (Defaults{WorkerModelProvider: "CLAUDE", WorkerModel: "existing-model"}) {
		t.Fatalf("defaults = %#v, want omitted model preserved", got)
	}
	clearModel := "  "
	cleared, err := service.MergeProviderModelDefaults(merged, ProviderModelUpdate{Model: &clearModel})
	if err != nil {
		t.Fatalf("clear model: %v", err)
	}
	if got := cleared.FileConfig().Defaults.WorkerModel; got != "" {
		t.Fatalf("model = %q, want explicitly supplied empty value cleared", got)
	}
}

func TestMergeProviderModelDefaults_ValidatesProviderThroughInjectedCatalog(t *testing.T) {
	t.Parallel()
	document, err := (ConfigDocumentService{}).Parse([]byte(`{"defaults":{"workerModelProvider":"CODEX","workerModel":"existing"}}`))
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
			service := ConfigDocumentService{Providers: controlledProviderCatalog}
			merged, mergeErr := service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &test.provider})
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
	document, err := (ConfigDocumentService{}).Parse([]byte(`{"defaults":{"workerModelProvider":"CODEX"}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	for _, test := range []struct {
		name      string
		provider  string
		catalog   ProviderCatalog
		wantError string
	}{
		{name: "empty", provider: "  ", catalog: controlledProviderCatalog, wantError: "provider is required"},
		{name: "unsupported", provider: "other", catalog: controlledProviderCatalog, wantError: `unsupported worker model provider "other"`},
		{name: "catalog required", provider: "codex", wantError: "provider catalog is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := ConfigDocumentService{Providers: test.catalog}
			_, mergeErr := service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &test.provider})
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
	document, err := (ConfigDocumentService{}).Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	model := " provider/private-model@next "
	merged, err := (ConfigDocumentService{}).MergeProviderModelDefaults(document, ProviderModelUpdate{Model: &model})
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
	service := ConfigDocumentService{Files: testFiles}
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

func assertDocumentRoundTrip(t *testing.T, document ConfigDocument) {
	t.Helper()
	data, err := (ConfigDocumentService{}).Marshal(document)
	if err != nil {
		t.Fatalf("MarshalConfigDocument() error = %v", err)
	}
	decoded, err := ParseFileConfig(data)
	if err != nil {
		t.Fatalf("ParseFileConfig(encoded) error = %v", err)
	}
	if !reflect.DeepEqual(decoded, document.FileConfig()) {
		t.Fatalf("decoded config = %#v, want %#v", decoded, document.FileConfig())
	}
}

func TestLoadFileDefaults_MalformedFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"defaults":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadFileDefaults(testFiles, path)
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path %q", err.Error(), path)
	}
}

func TestParseFileDefaults_AcceptsBackendScopeIDAlongsideDefaults(t *testing.T) {
	defaults, err := ParseFileDefaults([]byte(`{
		"backendScopeID": "local-11111111-1111-4111-8111-111111111111",
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5-codex"
		}
	}`))
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

	defaults, err := LoadFileDefaults(testFiles, path)
	if err != nil {
		t.Fatalf("LoadFileDefaults() error = %v", err)
	}
	if defaults.WorkerModelProvider != "claude" {
		t.Fatalf("provider = %q, want claude", defaults.WorkerModelProvider)
	}
	if defaults.WorkerModel != "claude-sonnet" {
		t.Fatalf("model = %q, want claude-sonnet", defaults.WorkerModel)
	}
}

func TestParseFileDefaults_AcceptsWorkerModelDefaults(t *testing.T) {
	defaults, err := ParseFileDefaults([]byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5-codex"
		}
	}`))
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
	cfg, err := ParseFileConfig([]byte(`{
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
	want := WorkerPreset{ID: "research", ModelProvider: "CODEX", Model: "gpt-5.4", ReasoningEffort: "high"}
	if len(cfg.WorkerPresets) != 1 || cfg.WorkerPresets[0] != want {
		t.Fatalf("worker presets = %#v, want %#v", cfg.WorkerPresets, []WorkerPreset{want})
	}
}

func TestParseFileConfig_MissingWorkerPresetsIsBackwardCompatible(t *testing.T) {
	cfg, err := ParseFileConfig([]byte(`{"defaults":{"workerModel":"existing-model"}}`))
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
		{name: "unsupported provider", json: `{"workerPresets":[{"id":"build","modelProvider":"other"}]}`, want: []string{`"build"`, `"other"`, "unsupported modelProvider"}},
		{name: "unsupported reasoning", json: `{"workerPresets":[{"id":"build","modelProvider":"codex","reasoningEffort":"extreme"}]}`, want: []string{`"build"`, `"extreme"`, "unsupported reasoningEffort"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFileConfig([]byte(tt.json))
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
	if err := os.WriteFile(path, []byte(`{"workerPresets":[{"id":"bad","modelProvider":"unknown"}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadFileDefaults(testFiles, path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadFileDefaults() error = %v, want validation error naming %q", err, path)
	}
}

func TestDefaultConfigPathUsesCanonicalDefaultPathsPolicy(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(string(filepath.Separator), "tmp", "operator-home")

	if got, want := DefaultConfigPath(homeDir), filepath.Join(homeDir, ".you-agent-factory", "config.json"); got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestResolve_FileEnvFlagPrecedenceIsIndependentPerField(t *testing.T) {
	resolved, err := Resolve(ResolveInput{
		File: Defaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		Env: Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		Flag: Defaults{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "flag-model" {
		t.Fatalf("model = %q, want flag-model", resolved.WorkerModel)
	}
	if resolved.WorkerModelProviderSource != SourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.WorkerModelProviderSource)
	}
	if resolved.WorkerModelSource != SourceFlag {
		t.Fatalf("model source = %q, want flag", resolved.WorkerModelSource)
	}
}

func TestResolve_EnvOverridesFileWhenFlagsUnset(t *testing.T) {
	resolved, err := Resolve(ResolveInput{
		File: Defaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		Env: Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", resolved.WorkerModel)
	}
	if resolved.WorkerModelProviderSource != SourceEnv {
		t.Fatalf("provider source = %q, want env", resolved.WorkerModelProviderSource)
	}
	if resolved.WorkerModelSource != SourceEnv {
		t.Fatalf("model source = %q, want env", resolved.WorkerModelSource)
	}
}

func TestResolve_CanonicalizesProviderAliases(t *testing.T) {
	resolved, err := Resolve(ResolveInput{
		File: Defaults{WorkerModelProvider: "kiro-cli"},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "KIRO" {
		t.Fatalf("provider = %q, want KIRO", resolved.WorkerModelProvider)
	}
}

func TestResolve_SymbolicDefaultResolvesThroughLowerPrecedenceConcreteProvider(t *testing.T) {
	resolved, err := Resolve(ResolveInput{
		File: Defaults{WorkerModelProvider: "codex"},
		Flag: Defaults{WorkerModelProvider: "DEFAULT"},
	}, "/tmp/config.json")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModelProviderSource != SourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.WorkerModelProviderSource)
	}
}

func TestResolve_SymbolicDefaultWithoutConcreteProviderFails(t *testing.T) {
	_, err := Resolve(ResolveInput{
		Flag: Defaults{WorkerModelProvider: "DEFAULT"},
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
	resolved, err := Resolve(ResolveInput{
		File: Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "file-model",
		},
	}, overridePath)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ConfigPath != overridePath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, overridePath)
	}
}

func TestResolve_UnsupportedProviderFailsWithAcceptedProviders(t *testing.T) {
	_, err := Resolve(ResolveInput{
		File: Defaults{WorkerModelProvider: "not-a-provider"},
	}, "/tmp/config.json")
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "unsupported worker model provider") {
		t.Fatalf("error = %q, want unsupported provider message", err.Error())
	}
	if !strings.Contains(err.Error(), "accepted canonical providers") {
		t.Fatalf("error = %q, want accepted provider summary", err.Error())
	}
}

func TestResolveFromHome_LoadsFileAndEnvironment(t *testing.T) {
	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
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
	t.Setenv(EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(EnvDefaultWorkerModel, "env-model")

	resolved, err := ResolveFromHomeWithEnvironment(testFiles, homeDir, Defaults{
		WorkerModelProvider: os.Getenv(EnvDefaultWorkerModelProvider),
		WorkerModel:         os.Getenv(EnvDefaultWorkerModel),
	}, FlagOverrides{})
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
	configPath := DefaultConfigPath(homeDir)
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
	t.Setenv(EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(EnvDefaultWorkerModel, "env-model")

	resolved, err := ResolveFromHomeWithEnvironment(testFiles, homeDir, Defaults{
		WorkerModelProvider: os.Getenv(EnvDefaultWorkerModelProvider),
		WorkerModel:         os.Getenv(EnvDefaultWorkerModel),
	}, FlagOverrides{
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
