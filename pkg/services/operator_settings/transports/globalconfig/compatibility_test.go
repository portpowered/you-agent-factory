package globalconfig_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

// TestOperatorConfigCore_ModelOverlaysRoundTripAndReportTypedFailures proves
// model overlays round-trip through operator configuration and invalid inputs
// report typed failures.
func TestOperatorConfigCore_ModelOverlaysRoundTripAndReportTypedFailures(t *testing.T) {
	t.Parallel()

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
	assertModelOverlaysAndClone(t, decoded)

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

	assertInvalidModelOverlays(t)

}

// TestOperatorConfigCore_FutureFieldsWarnAndSurviveConfigRewrite proves future settings fields warn and survive a configuration rewrite.
func TestOperatorConfigCore_FutureFieldsWarnAndSurviveConfigRewrite(t *testing.T) {
	t.Parallel()
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

	assertOmittedFutureContainer(t)

}

func assertModelOverlaysAndClone(t *testing.T, decoded operatorsettings.Config) {
	t.Helper()
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
	assertDetachedModelClone(t, decoded)

}

func assertInvalidModelOverlays(t *testing.T) {
	t.Helper()
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

func assertOmittedFutureContainer(t *testing.T) {
	t.Helper()
	omittedContainer, err := globalconfigmapping.PreserveUnknownFields(
		[]byte(`{"runtime":{"logging":{"futureLogging":{"enabled":true}},"futureRuntime":"keep"}}`),
		[]byte(`{"defaults":{"workerModel":"after"}}`),
	)
	if err != nil {
		t.Fatalf("PreserveUnknownFields(omitted runtime) error = %v", err)
	}
	var omittedDocument map[string]any
	if err := json.Unmarshal(omittedContainer, &omittedDocument); err != nil {
		t.Fatalf("decode omitted runtime rewrite: %v", err)
	}
	runtime, ok := omittedDocument["runtime"].(map[string]any)
	if !ok || runtime["futureRuntime"] != "keep" || !reflect.DeepEqual(runtime["logging"], map[string]any{
		"futureLogging": map[string]any{"enabled": true},
	}) {
		t.Fatalf("runtime after omitted-container rewrite = %#v, want preserved future values", omittedDocument["runtime"])
	}
}

func assertDetachedModelClone(t *testing.T, decoded operatorsettings.Config) {
	t.Helper()
	clone := decoded.Clone()
	*clone.Models["llm"].Backend = "mutated"
	clone.Models["custom-model"].Operations[0] = "ASR"
	if *decoded.Models["llm"].Backend != "localai-llamacpp" || decoded.Models["custom-model"].Operations[0] != "OMNI" {
		t.Fatal("Config.Clone shared model overlay state")
	}

}
