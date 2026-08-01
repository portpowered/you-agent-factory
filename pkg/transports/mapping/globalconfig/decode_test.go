package globalconfig_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestLoadFileConfig_DecodesGeneratedContractAndNormalizesDomainValues(t *testing.T) {
	path := writeConfig(t, `{
		"backendScopeID": "local-11111111-1111-4111-8111-111111111111",
		"defaults": {
			"workerModelProvider": " codex ",
			"workerModel": " gpt-5.4 "
		},
		"runtime": {
			"logging": {
				"directory": " logs/runtime ",
				"maxSizeMB": 11,
				"maxBackups": 12,
				"maxAgeDays": 13,
				"compress": true
			},
			"metrics": {
				"directory": " metrics/runtime ",
				"maxSizeMB": 21,
				"maxBackups": 22,
				"maxAgeDays": 23,
				"compress": true
			}
		},
		"workers": {"acp":{"integrations":[{
			"id":" entry-1 ",
			"name":" cursor-acp ",
			"transport":"stdio",
			"command":" cursor-agent acp "
		}]}},
		"workerPresets": [{
			"id": " research ",
			"modelProvider": "openai",
			"model": " gpt-5.4-mini ",
			"reasoningEffort": "high"
		}]
	}`)

	config, err := operatorsettings.LoadFileConfig(platformfilesystem.Local{}, globalconfig.Decode, path)
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if got, want := config.BackendScopeID, "local-11111111-1111-4111-8111-111111111111"; got != want {
		t.Fatalf("backendScopeID = %q, want %q", got, want)
	}
	if got, want := config.Defaults, (operatorsettings.Defaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5.4",
	}); got != want {
		t.Fatalf("defaults = %#v, want %#v", got, want)
	}
	if got, want := config.Runtime.Logging, (operatorsettings.RuntimeArtifactSettings{
		Directory: "logs/runtime", MaxSizeMB: 11, MaxBackups: 12, MaxAgeDays: 13, Compress: true,
	}); got != want {
		t.Fatalf("runtime logging = %#v, want %#v", got, want)
	}
	if got, want := config.Runtime.Metrics, (operatorsettings.RuntimeArtifactSettings{
		Directory: "metrics/runtime", MaxSizeMB: 21, MaxBackups: 22, MaxAgeDays: 23, Compress: true,
	}); got != want {
		t.Fatalf("runtime metrics = %#v, want %#v", got, want)
	}
	wantPreset := operatorsettings.WorkerPreset{
		ID: "research", ModelProvider: "CODEX", Model: "gpt-5.4-mini", ReasoningEffort: "high",
	}
	if len(config.WorkerPresets) != 1 || config.WorkerPresets[0] != wantPreset {
		t.Fatalf("worker presets = %#v, want %#v", config.WorkerPresets, []operatorsettings.WorkerPreset{wantPreset})
	}
	wantIntegration := operatorsettings.ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	if !reflect.DeepEqual(config.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{wantIntegration}) {
		t.Fatalf("ACP integrations = %#v, want %#v", config.Workers.ACP.Integrations, wantIntegration)
	}
}

func TestEncode_RoundTripsCanonicalIdentityAndSiblingSettings(t *testing.T) {
	want := operatorsettings.Config{
		BackendScopeID: "local-11111111-1111-4111-8111-111111111111",
		Defaults: operatorsettings.Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5.4",
		},
		WorkerPresets: []operatorsettings.WorkerPreset{{
			ID: "research", ModelProvider: "CODEX", Model: "gpt-5.4-mini", ReasoningEffort: "high",
		}},
		Workers: operatorsettings.WorkerSettings{ACP: operatorsettings.ACPSettings{Integrations: []operatorsettings.ACPIntegration{{
			ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
		}}}},
	}

	payload, err := globalconfig.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if got.BackendScopeID != want.BackendScopeID || got.Defaults != want.Defaults {
		t.Fatalf("round trip config = %#v, want identity/defaults %#v", got, want)
	}
	if len(got.WorkerPresets) != 1 || got.WorkerPresets[0] != want.WorkerPresets[0] {
		t.Fatalf("round trip presets = %#v, want %#v", got.WorkerPresets, want.WorkerPresets)
	}
}

func TestEncode_RoundTripsExtensionProviderDefaultsAndPresets(t *testing.T) {
	const identity = "customer.provider-v2"
	want := operatorsettings.Config{
		Defaults: operatorsettings.Defaults{WorkerModelProvider: identity},
		WorkerPresets: []operatorsettings.WorkerPreset{{
			ID: "extension", ModelProvider: identity,
		}},
	}

	payload, err := globalconfig.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if got.Defaults.WorkerModelProvider != identity || len(got.WorkerPresets) != 1 || got.WorkerPresets[0].ModelProvider != identity {
		t.Fatalf("extension provider round trip = %#v, want %q in defaults and presets", got, identity)
	}
}

func TestDecode_EmptyObjectReturnsEmptyConfig(t *testing.T) {
	config, err := globalconfig.Decode([]byte(`{}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if config.BackendScopeID != "" || config.Defaults != (operatorsettings.Defaults{}) || len(config.WorkerPresets) != 0 {
		t.Fatalf("config = %#v, want empty identity, defaults, and presets", config)
	}
	if config.Runtime != defaultRuntimeSettings() {
		t.Fatalf("runtime = %#v, want defaults %#v", config.Runtime, defaultRuntimeSettings())
	}
}

func TestDecode_PartialRuntimeSettingsApplyDefaultsIndependently(t *testing.T) {
	config, err := globalconfig.Decode([]byte(`{
		"runtime": {
			"logging": {"compress": true},
			"metrics": {"maxSizeMB": 7}
		}
	}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	wantLogging := defaultRuntimeArtifactSettings()
	wantLogging.Compress = true
	if config.Runtime.Logging != wantLogging {
		t.Fatalf("runtime logging = %#v, want %#v", config.Runtime.Logging, wantLogging)
	}
	wantMetrics := defaultRuntimeArtifactSettings()
	wantMetrics.MaxSizeMB = 7
	if config.Runtime.Metrics != wantMetrics {
		t.Fatalf("runtime metrics = %#v, want %#v", config.Runtime.Metrics, wantMetrics)
	}
}

func TestDecode_ReturnsDetachedNormalizedRuntimeValues(t *testing.T) {
	payload := []byte(`{
		"runtime": {
			"logging": {"directory":" logs "},
			"metrics": {"compress":true}
		}
	}`)

	first, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	first.Runtime.Logging.Directory = "mutated"
	first.Runtime.Metrics.Compress = false

	second, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("second Decode() error = %v", err)
	}
	if second.Runtime.Logging.Directory != "logs" || !second.Runtime.Metrics.Compress {
		t.Fatalf("second decoded runtime = %#v, want independent normalized values", second.Runtime)
	}
}

func TestLoadFileConfig_PartialDocumentParticipatesInDocumentedPrecedence(t *testing.T) {
	path := writeConfig(t, `{"defaults":{"workerModelProvider":"codex","workerModel":"file-model"}}`)
	fileConfig, err := operatorsettings.LoadFileConfig(platformfilesystem.Local{}, globalconfig.Decode, path)
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	resolved, err := operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: fileConfig.Defaults,
		Env:  operatorsettings.Defaults{WorkerModel: "env-model"},
		Flag: operatorsettings.Defaults{WorkerModelProvider: "claude"},
	}, path)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CLAUDE" || resolved.WorkerModelProviderSource != operatorsettings.SourceFlag {
		t.Fatalf("provider = %q from %q, want CLAUDE from flag", resolved.WorkerModelProvider, resolved.WorkerModelProviderSource)
	}
	if resolved.WorkerModel != "env-model" || resolved.WorkerModelSource != operatorsettings.SourceEnv {
		t.Fatalf("model = %q from %q, want env-model from env", resolved.WorkerModel, resolved.WorkerModelSource)
	}
}

func TestEncode_OmitsAbsentOptionalValues(t *testing.T) {
	payload, err := globalconfig.Encode(operatorsettings.Config{
		Defaults: operatorsettings.Defaults{WorkerModelProvider: "CODEX"},
		WorkerPresets: []operatorsettings.WorkerPreset{{
			ID: "build", ModelProvider: "CODEX",
		}},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	for _, absent := range []string{`"backendScopeID"`, `"workerModel"`, `"model"`, `"reasoningEffort"`} {
		if strings.Contains(string(payload), absent) {
			t.Fatalf("Encode() payload = %s, want %s omitted", payload, absent)
		}
	}
	if _, err := globalconfig.Decode(payload); err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
}

func TestLoadFileConfig_MissingFileReturnsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	config, err := operatorsettings.LoadFileConfig(platformfilesystem.Local{}, globalconfig.Decode, path)
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if config.Defaults != (operatorsettings.Defaults{}) || len(config.WorkerPresets) != 0 {
		t.Fatalf("config = %#v, want empty", config)
	}
	if config.Runtime != defaultRuntimeSettings() {
		t.Fatalf("runtime = %#v, want defaults %#v", config.Runtime, defaultRuntimeSettings())
	}
}

func TestGeneratedLoaderAndConfigDocumentServiceAgreeOnEffectiveConfig(t *testing.T) {
	tests := []struct {
		name     string
		document string
		absent   bool
	}{
		{name: "absent", absent: true},
		{name: "partial", document: `{
			"defaults":{"workerModelProvider":"codex"},
			"runtime":{"metrics":{"compress":true}}
		}`},
		{name: "complete", document: `{
			"backendScopeID":"local-11111111-1111-4111-8111-111111111111",
			"defaults":{"workerModelProvider":"claude","workerModel":"claude-next"},
			"runtime":{
				"logging":{"directory":"logs","maxSizeMB":11,"maxBackups":12,"maxAgeDays":13,"compress":true},
				"metrics":{"directory":"metrics","maxSizeMB":21,"maxBackups":22,"maxAgeDays":23,"compress":false}
			},
			"workerPresets":[{"id":"build","modelProvider":"codex","model":"gpt-next","reasoningEffort":"high"}]
		}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if !test.absent {
				if err := os.WriteFile(path, []byte(test.document), 0o600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			files := platformfilesystem.Local{}
			loaded, err := operatorsettings.LoadFileConfig(files, globalconfig.Decode, path)
			if err != nil {
				t.Fatalf("LoadFileConfig() error = %v", err)
			}
			document, err := settingswire.NewConfigDocumentService(
				files,
				nil,
				globalconfig.Decode,
				nil,
				nil,
				nil,
			).Load(path)
			if err != nil {
				t.Fatalf("ConfigDocumentService.Load() error = %v", err)
			}
			if got := document.FileConfig(); !reflect.DeepEqual(got, loaded) {
				t.Fatalf("document service config = %#v, generated loader config = %#v", got, loaded)
			}
		})
	}
}

func TestLoadFileConfig_InvalidDocumentsNamePathAndCause(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "malformed", json: `{"defaults":`, want: "decode generated global config"},
		{name: "null root", json: `null`, want: "expected a JSON object"},
		{name: "unknown top-level", json: `{"unsupported":true}`, want: `unknown field "unsupported"`},
		{name: "unknown defaults field", json: `{"defaults":{"unsupported":true}}`, want: `unknown field "unsupported"`},
		{name: "unknown runtime field", json: `{"runtime":{"unsupported":true}}`, want: `unknown field "unsupported"`},
		{name: "empty runtime directory", json: `{"runtime":{"logging":{"directory":" "}}}`, want: "runtime.logging.directory must be non-empty"},
		{name: "invalid runtime size", json: `{"runtime":{"metrics":{"maxSizeMB":0}}}`, want: "runtime.metrics.maxSizeMB must be at least 1"},
		{name: "invalid runtime backups", json: `{"runtime":{"logging":{"maxBackups":0}}}`, want: "runtime.logging.maxBackups must be at least 1"},
		{name: "invalid runtime age", json: `{"runtime":{"logging":{"maxAgeDays":-1}}}`, want: "runtime.logging.maxAgeDays must be at least 1"},
		{name: "trailing JSON", json: `{}` + "\n{}", want: "unexpected trailing JSON"},
		{name: "invalid trailing token", json: `{}` + "\nx", want: "invalid character"},
		{name: "missing preset provider", json: `{"workerPresets":[{"id":"build"}]}`, want: "modelProvider"},
		{name: "duplicate preset", json: `{"workerPresets":[{"id":"build","modelProvider":"codex"},{"id":" build ","modelProvider":"claude"}]}`, want: "duplicated"},
		{name: "symbolic preset provider", json: `{"workerPresets":[{"id":"build","modelProvider":"DEFAULT"}]}`, want: `unsupported modelProvider "DEFAULT"`},
		{name: "invalid reasoning effort", json: `{"workerPresets":[{"id":"build","modelProvider":"codex","reasoningEffort":"extreme"}]}`, want: `unsupported reasoningEffort "extreme"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.json)
			_, err := operatorsettings.LoadFileConfig(platformfilesystem.Local{}, globalconfig.Decode, path)
			if err == nil {
				t.Fatal("LoadFileConfig() error = nil, want rejection")
			}
			for _, fragment := range []string{path, tt.want} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func defaultRuntimeArtifactSettings() operatorsettings.RuntimeArtifactSettings {
	return operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
		MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
	}
}

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	defaults := defaultRuntimeArtifactSettings()
	return operatorsettings.RuntimeSettings{Logging: defaults, Metrics: defaults}
}
