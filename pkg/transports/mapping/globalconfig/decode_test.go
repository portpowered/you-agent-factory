package globalconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestLoadFileConfig_DecodesGeneratedContractAndNormalizesDomainValues(t *testing.T) {
	path := writeConfig(t, `{
		"backendScopeID": "local-11111111-1111-4111-8111-111111111111",
		"defaults": {
			"workerModelProvider": " codex ",
			"workerModel": " gpt-5.4 "
		},
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
	wantPreset := operatorsettings.WorkerPreset{
		ID: "research", ModelProvider: "CODEX", Model: "gpt-5.4-mini", ReasoningEffort: "high",
	}
	if len(config.WorkerPresets) != 1 || config.WorkerPresets[0] != wantPreset {
		t.Fatalf("worker presets = %#v, want %#v", config.WorkerPresets, []operatorsettings.WorkerPreset{wantPreset})
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

func TestLoadFileConfig_MissingFileReturnsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	config, err := operatorsettings.LoadFileConfig(platformfilesystem.Local{}, globalconfig.Decode, path)
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if config.Defaults != (operatorsettings.Defaults{}) || len(config.WorkerPresets) != 0 {
		t.Fatalf("config = %#v, want empty", config)
	}
}

func TestLoadFileConfig_InvalidDocumentsNamePathAndCause(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "malformed", json: `{"defaults":`, want: "decode generated global config"},
		{name: "unknown top-level", json: `{"unsupported":true}`, want: `unknown field "unsupported"`},
		{name: "unknown defaults field", json: `{"defaults":{"unsupported":true}}`, want: `unknown field "unsupported"`},
		{name: "trailing JSON", json: `{}` + "\n{}", want: "unexpected trailing JSON"},
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
