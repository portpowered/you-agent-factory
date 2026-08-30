package globalconfig_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
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

func TestDecode_AbsentACPAgentProfileDecodesWithoutMigration(t *testing.T) {
	config, err := globalconfig.Decode([]byte(`{
		"defaults": {"workerModelProvider": "codex"},
		"workers": {"acp": {"integrations": [{"id":"entry-1","name":"cursor-acp","transport":"stdio","command":"cursor-agent acp"}]}}
	}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if config.Workers.ACP.AgentProfile != nil {
		t.Fatalf("AgentProfile = %#v, want nil for a document with no authored profile", config.Workers.ACP.AgentProfile)
	}
	if len(config.Workers.ACP.Integrations) != 1 {
		t.Fatalf("Integrations = %#v, want one entry preserved alongside an absent profile", config.Workers.ACP.Integrations)
	}
}

func TestEncode_RoundTripsACPAgentProfilePreservingOrderAndNamespace(t *testing.T) {
	want := operatorsettings.Config{
		Workers: operatorsettings.WorkerSettings{ACP: operatorsettings.ACPSettings{
			AgentProfile: &operatorsettings.ACPAgentProfile{
				DefaultTarget: "factory:@you/review",
				AllowedTargets: []string{
					"factory:@you/review",
					"factory:@you/factory-builder",
					"factory:local/software-auto",
				},
			},
		}},
	}

	payload, err := globalconfig.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if strings.Contains(string(payload), `"version"`) || strings.Contains(string(payload), `"digest"`) {
		t.Fatalf("Encode() payload = %s, want no version or digest field", payload)
	}
	got, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if got.Workers.ACP.AgentProfile == nil {
		t.Fatal("round trip AgentProfile = nil, want authored profile preserved")
	}
	if !reflect.DeepEqual(*got.Workers.ACP.AgentProfile, *want.Workers.ACP.AgentProfile) {
		t.Fatalf("round trip AgentProfile = %#v, want %#v", *got.Workers.ACP.AgentProfile, *want.Workers.ACP.AgentProfile)
	}
}

func TestEncode_RoundTripsUnrestrictedACPAgentProfileByOmittingAllowedTargets(t *testing.T) {
	// An unrestricted profile must encode without an allowedTargets key at
	// all. Emitting an empty array instead would produce a document that
	// Decode rejects, so the profile would not survive its own round trip.
	want := operatorsettings.Config{
		Workers: operatorsettings.WorkerSettings{ACP: operatorsettings.ACPSettings{
			AgentProfile: &operatorsettings.ACPAgentProfile{
				DefaultTarget: "factory:@you/goal",
			},
		}},
	}

	payload, err := globalconfig.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if strings.Contains(string(payload), `"allowedTargets"`) {
		t.Fatalf("Encode() payload = %s, want no allowedTargets key for an unrestricted profile", payload)
	}

	got, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if got.Workers.ACP.AgentProfile == nil {
		t.Fatal("round trip AgentProfile = nil, want authored profile preserved")
	}
	if !got.Workers.ACP.AgentProfile.IsUnrestricted() {
		t.Fatalf("round trip AgentProfile = %#v, want unrestricted", *got.Workers.ACP.AgentProfile)
	}
	if got.Workers.ACP.AgentProfile.DefaultTarget != "factory:@you/goal" {
		t.Fatalf("round trip DefaultTarget = %q, want %q",
			got.Workers.ACP.AgentProfile.DefaultTarget, "factory:@you/goal")
	}
}

func TestEncode_OmitsAgentProfileWhenAbsent(t *testing.T) {
	payload, err := globalconfig.Encode(operatorsettings.Config{
		Defaults: operatorsettings.Defaults{WorkerModelProvider: "CODEX"},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if strings.Contains(string(payload), `"agentProfile"`) {
		t.Fatalf("Encode() payload = %s, want agentProfile omitted when absent", payload)
	}
	got, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if got.Workers.ACP.AgentProfile != nil {
		t.Fatalf("AgentProfile = %#v, want nil after an absent-profile round trip", got.Workers.ACP.AgentProfile)
	}
}

func TestDecode_RejectsMalformedStoredACPAgentProfile(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			// An authored empty array must stay a rejection rather than
			// silently widening into "every installed Factory is selectable",
			// which is how an omitted allowedTargets is read.
			name: "empty allowlist",
			json: `{"workers":{"acp":{"agentProfile":{"defaultTarget":"factory:@you/factory-builder","allowedTargets":[]}}}}`,
			want: "allowedTargets must not be empty; omit it to leave the profile unrestricted",
		},
		{
			name: "default outside factory namespace",
			json: `{"workers":{"acp":{"agentProfile":{"defaultTarget":"worker:@you/factory-builder","allowedTargets":["worker:@you/factory-builder"]}}}}`,
			want: "must use the factory: namespace",
		},
		{
			name: "default absent from allowlist",
			json: `{"workers":{"acp":{"agentProfile":{"defaultTarget":"factory:@you/review","allowedTargets":["factory:@you/factory-builder"]}}}}`,
			want: "must be present in allowedTargets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := globalconfig.Decode([]byte(tt.json))
			if err == nil {
				t.Fatal("Decode() error = nil, want rejection")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Decode() error = %q, want fragment %q", err, tt.want)
			}
		})
	}
}

func TestEncode_EmitsCanonicalJSONBytes(t *testing.T) {
	payload, err := globalconfig.Encode(operatorsettings.Config{
		BackendScopeID: "local-11111111-1111-4111-8111-111111111111",
		Defaults: operatorsettings.Defaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5.4",
		},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	want := `{
  "backendScopeID": "local-11111111-1111-4111-8111-111111111111",
  "defaults": {
    "workerModel": "gpt-5.4",
    "workerModelProvider": "codex"
  },
  "priceTable": {
    "currency": "USD",
    "models": []
  },
  "runtime": {
    "logging": {
      "compress": false,
      "maxAgeDays": 30,
      "maxBackups": 20,
      "maxSizeMB": 100
    },
    "metrics": {
      "compress": false,
      "maxAgeDays": 30,
      "maxBackups": 20,
      "maxSizeMB": 100
    }
  },
  "workerPresets": []
}
`
	if string(payload) != want {
		t.Fatalf("Encode() bytes = %q, want %q", payload, want)
	}
}

func TestPriceTableDecodeEncodePreservesOptionalZeroAndExactRates(t *testing.T) {
	payload := []byte(`{
		"priceTable": {
			"currency": "USD",
			"models": [{
				"provider": " openai ",
				"model": " gpt-5 ",
				"inputPerMillionTokens": "1.2500",
				"outputPerMillionTokens": "10",
				"cachedInputPerMillionTokens": "0"
			}]
		}
	}`)
	config, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	model := config.PriceTable.Models[0]
	if config.PriceTable.Currency != "USD" || model.Provider != "CODEX" || model.Model != "gpt-5" || model.InputPerMillionTokens != "1.2500" {
		t.Fatalf("decoded price table = %#v, want normalized exact values", config.PriceTable)
	}
	if model.CachedInputPerMillionTokens == nil || *model.CachedInputPerMillionTokens != "0" {
		t.Fatalf("decoded cached rate = %#v, want explicit zero", model.CachedInputPerMillionTokens)
	}
	encoded, err := globalconfig.Encode(config)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"cachedInputPerMillionTokens": "0"`) || !strings.Contains(string(encoded), `"inputPerMillionTokens": "1.2500"`) {
		t.Fatalf("encoded price table = %s, want exact optional/base rate strings", encoded)
	}
	if strings.Contains(string(encoded), "reasoningOutputPerMillionTokens") {
		t.Fatal("encoded price table materialized an omitted reasoning rate")
	}
}

func TestPriceTableDecodeRejectsInvalidContractValues(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "duplicate provider model", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"openai","model":"gpt-5","inputPerMillionTokens":"1","outputPerMillionTokens":"2"},{"provider":"CODEX","model":"gpt-5","inputPerMillionTokens":"3","outputPerMillionTokens":"4"}]}}`, want: "duplicates"},
		{name: "unsupported currency", payload: `{"priceTable":{"currency":"EUR","models":[]}}`, want: "unsupported"},
		{name: "missing currency", payload: `{"priceTable":{"models":[]}}`, want: "currency is required"},
		{name: "negative rate", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"CODEX","model":"gpt-5","inputPerMillionTokens":"-1","outputPerMillionTokens":"2"}]}}`, want: "non-negative decimal"},
		{name: "negative cached rate", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"CODEX","model":"gpt-5","inputPerMillionTokens":"1","outputPerMillionTokens":"2","cachedInputPerMillionTokens":"-0.1"}]}}`, want: "non-negative decimal"},
		{name: "malformed cached rate", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"CODEX","model":"gpt-5","inputPerMillionTokens":"1","outputPerMillionTokens":"2","cachedInputPerMillionTokens":"1e-3"}]}}`, want: "non-negative decimal"},
		{name: "missing base rate", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"CODEX","model":"gpt-5","outputPerMillionTokens":"2"}]}}`, want: "inputPerMillionTokens"},
		{name: "missing model identity", payload: `{"priceTable":{"currency":"USD","models":[{"provider":"CODEX","inputPerMillionTokens":"1","outputPerMillionTokens":"2"}]}}`, want: "model must be non-empty"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := globalconfig.Decode([]byte(testCase.payload))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Decode() = %v, want error containing %q", err, testCase.want)
			}
		})
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
	if config.PriceTable.Currency != operatorsettings.PriceTableCurrencyUSD || config.PriceTable.Models == nil || len(config.PriceTable.Models) != 0 {
		t.Fatalf("price table = %#v, want default-empty USD table", config.PriceTable)
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

func TestDecodeWithDiagnostics_IgnoresUnknownFieldsAndReportsSortedPaths(t *testing.T) {
	config, diagnostics, err := globalconfig.DecodeWithDiagnostics([]byte(`{
		"zFuture": "secret-root-value",
		"defaults": {
			"workerModelProvider": " codex ",
			"workerModel": " gpt-5 ",
			"futureDefault": "secret-default-value"
		},
		"runtime": {
			"futureRuntime": "secret-runtime-value",
			"metrics": {"compress": true, "futureMetric": "secret-metric-value"}
		},
		"models": {
			"llm": {"source": " hf://custom/gemma ", "futureModel": "secret-model-value"}
		},
		"workers": {"acp": {"futureAcp": "secret-acp-value"}},
		"workerPresets": [{
			"id": "build",
			"modelProvider": "codex",
			"futurePreset": "secret-preset-value"
		}]
	}`))
	if err != nil {
		t.Fatalf("DecodeWithDiagnostics() error = %v", err)
	}
	if config.Defaults.WorkerModelProvider != "codex" || config.Defaults.WorkerModel != "gpt-5" {
		t.Fatalf("known defaults = %#v, want normalized codex/gpt-5", config.Defaults)
	}
	if source := config.Models["llm"].Source; source == nil || *source != "hf://custom/gemma" {
		t.Fatalf("known model source = %#v, want normalized source", config.Models["llm"].Source)
	}
	wantPaths := []string{
		"$.defaults.futureDefault",
		"$.models.llm.futureModel",
		"$.runtime.futureRuntime",
		"$.runtime.metrics.futureMetric",
		"$.workerPresets[0].futurePreset",
		"$.workers.acp.futureAcp",
		"$.zFuture",
	}
	if got := diagnostics.Paths(); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("ignored JSON paths = %#v, want %#v", got, wantPaths)
	}
	if strings.Contains(strings.Join(diagnostics.Paths(), "\n"), "secret") {
		t.Fatalf("ignored JSON paths leaked a field value: %#v", diagnostics.Paths())
	}
}

func TestDecodeWithDiagnostics_PreservesCaseInsensitiveKnownFields(t *testing.T) {
	config, diagnostics, err := globalconfig.DecodeWithDiagnostics([]byte(`{
		"backendScopeId": "scope-existing",
		"Defaults": {
			"WorkerModelProvider": "codex",
			"futureDefault": true
		}
	}`))
	if err != nil {
		t.Fatalf("DecodeWithDiagnostics() error = %v", err)
	}
	if config.BackendScopeID != "scope-existing" {
		t.Fatalf("backend scope ID = %q, want scope-existing", config.BackendScopeID)
	}
	if config.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("worker model provider = %q, want codex", config.Defaults.WorkerModelProvider)
	}
	if got, want := diagnostics.Paths(), []string{"$.Defaults.futureDefault"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ignored JSON paths = %#v, want %#v", got, want)
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

func TestDecodeAndEncode_RoundTripsPartialAndCompleteModelEntries(t *testing.T) {
	config, err := globalconfig.Decode([]byte(`{
		"defaults":{"workerModelProvider":"codex","workerModel":"gpt-5.4"},
		"models":{
			" llm ":{"source":" hf://custom/gemma "},
			"studio-whisper":{
				"source":" hf://ggerganov/whisper.cpp/ggml-base.en.bin ",
				"backend":" localai-whisper ",
				"loadPolicy":"on_demand",
				"operations":[" asr "]
			}
		}
	}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	assertPartialModelConfig(t, config)
	assertCompleteModelConfig(t, config)

	payload, err := globalconfig.Encode(config)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	roundTrip, err := globalconfig.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(Encode()) error = %v", err)
	}
	if !reflect.DeepEqual(roundTrip, config) {
		t.Fatalf("round-trip config = %#v, want %#v", roundTrip, config)
	}
	assertModelConfigCloneIsDetached(t, config)
}

func assertPartialModelConfig(t *testing.T, config operatorsettings.Config) {
	t.Helper()
	partial := config.Models["llm"]
	if partial.Source == nil || *partial.Source != "hf://custom/gemma" || partial.Backend != nil || partial.LoadPolicy != nil || partial.Operations != nil {
		t.Fatalf("partial model = %#v, want one detached source override", partial)
	}
}

func assertCompleteModelConfig(t *testing.T, config operatorsettings.Config) {
	t.Helper()
	complete := config.Models["studio-whisper"]
	if complete.Source == nil || *complete.Source != "hf://ggerganov/whisper.cpp/ggml-base.en.bin" || complete.Backend == nil || *complete.Backend != "localai-whisper" {
		t.Fatalf("complete model = %#v, want normalized source/backend", complete)
	}
	if complete.LoadPolicy == nil || *complete.LoadPolicy != operatorsettings.ModelLoadPolicyOnDemand || len(complete.Operations) != 1 || complete.Operations[0] != operatorsettings.ModelOperationASR {
		t.Fatalf("complete model policy/operations = %#v, want ON_DEMAND and ASR", complete)
	}
}

func assertModelConfigCloneIsDetached(t *testing.T, config operatorsettings.Config) {
	t.Helper()
	clone := config.Clone()
	clonedEntry := clone.Models["studio-whisper"]
	*clonedEntry.Source = "changed"
	clonedEntry.Operations[0] = operatorsettings.ModelOperationTTS
	clone.Models["studio-whisper"] = clonedEntry
	if *config.Models["studio-whisper"].Source != "hf://ggerganov/whisper.cpp/ggml-base.en.bin" || config.Models["studio-whisper"].Operations[0] != operatorsettings.ModelOperationASR {
		t.Fatalf("config model entry changed through clone = %#v", config.Models["studio-whisper"])
	}
}

func TestDecode_RejectsInvalidModelConfigurationWithTypedFailure(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		field   string
	}{
		{name: "empty explicit source", payload: `{"models":{"llm":{"source":" "}}}`, field: "models[\"llm\"].source"},
		{name: "unsupported policy", payload: `{"models":{"new-model":{"source":"hf://source","backend":"backend","loadPolicy":"EAGER","operations":["OMNI"]}}}`, field: "models[\"new-model\"].loadPolicy"},
		{name: "unsupported operation", payload: `{"models":{"new-model":{"source":"hf://source","backend":"backend","loadPolicy":"ON_DEMAND","operations":["UNKNOWN"]}}}`, field: "models[\"new-model\"].operations"},
		{name: "incomplete new model", payload: `{"models":{"new-model":{"source":"hf://source"}}}`, field: "models[\"new-model\"].backend"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := globalconfig.Decode([]byte(test.payload))
			if err == nil {
				t.Fatal("Decode() error = nil, want rejection")
			}
			if !errors.Is(err, operatorsettings.ErrConfigurationInvalid) {
				t.Fatalf("Decode() error = %v, want ErrConfigurationInvalid", err)
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Decode() error = %q, want field path %q", err, test.field)
			}
		})
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

func TestConfigDocumentUpdatePreservesUnknownGlobalConfigFields(t *testing.T) {
	path := writeConfig(t, `{
		"defaults": {
			"workerModel": "before",
			"futureNested": {"secret": "retain-me"}
		},
		"futureTopLevel": {"enabled": true, "secret": "retain-me-too"}
	}`)
	service := settingswire.NewConfigDocumentServiceWithPreserver(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfig.Decode,
		globalconfig.Encode,
		func(string) (string, bool) { return "", false },
		&sync.Mutex{},
		globalconfig.PreserveUnknownFields,
		globalconfig.DecodeWithDiagnostics,
	)
	model := "after"
	if _, err := service.ConfigureProviderModel(context.Background(), path, operatorsettings.ProviderModelUpdate{Model: &model}); err != nil {
		t.Fatalf("ConfigureProviderModel() error = %v", err)
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(persisted, &document); err != nil {
		t.Fatalf("decode persisted document: %v", err)
	}
	if got := document["futureTopLevel"]; !reflect.DeepEqual(got, map[string]any{"enabled": true, "secret": "retain-me-too"}) {
		t.Fatalf("futureTopLevel = %#v, want original value", got)
	}
	defaults, ok := document["defaults"].(map[string]any)
	if !ok {
		t.Fatalf("defaults = %#v, want object", document["defaults"])
	}
	if got, want := defaults["workerModel"], "after"; got != want {
		t.Fatalf("defaults.workerModel = %#v, want %q", got, want)
	}
	if got := defaults["futureNested"]; !reflect.DeepEqual(got, map[string]any{"secret": "retain-me"}) {
		t.Fatalf("defaults.futureNested = %#v, want original value", got)
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
		{name: "empty runtime directory", json: `{"runtime":{"logging":{"directory":" "}}}`, want: "runtime.logging.directory must be non-empty"},
		{name: "invalid runtime size", json: `{"runtime":{"metrics":{"maxSizeMB":0}}}`, want: "runtime.metrics.maxSizeMB must be at least 1"},
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

func TestDecode_PreservesCanonicalErrorWrappingAndRejection(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "malformed", payload: `{"defaults":`, want: "decode generated global config: unexpected EOF"},
		{name: "null root", payload: `null`, want: "decode generated global config: expected a JSON object"},
		{name: "trailing JSON", payload: "{}\n{}", want: "decode generated global config: unexpected trailing JSON"},
		{name: "invalid runtime", payload: `{"runtime":{"metrics":{"maxSizeMB":0}}}`, want: "runtime.metrics.maxSizeMB must be at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := globalconfig.Decode([]byte(tt.payload))
			if err == nil {
				t.Fatal("Decode() error = nil, want rejection")
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("Decode() error = %q, want %q", got, tt.want)
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
