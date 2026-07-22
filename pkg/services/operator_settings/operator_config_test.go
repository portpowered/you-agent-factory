package operatorsettings

import (
	"os"
	"path/filepath"
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
