package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIOutputValidationAndProjectionBranches(t *testing.T) {
	t.Parallel()

	required := true
	detail := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "OMNI",
		Inputs: []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}, {
			Name: "text", Modality: modelinference.ModalityText, Required: &required,
		}},
		Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}, {
			Name: "usage", Modality: modelinference.ModalityJSON,
		}},
	}}}}

	validationCases := []struct {
		name string
		cfg  InvokeConfig
		want string
	}{
		{name: "mapping with output path", cfg: InvokeConfig{OutputMappings: []string{"text=a"}, OutputPath: "speech.wav"}, want: "cannot be combined"},
		{name: "mapping missing output", cfg: InvokeConfig{OutputMappings: []string{"text=a"}}, want: "cover every output"},
		{name: "mapping unknown slot", cfg: InvokeConfig{OutputMappings: []string{"other=a", "usage=b"}}, want: "unknown slot"},
		{name: "multiple outputs without mode", cfg: InvokeConfig{}, want: "multiple model outputs"},
		{name: "unknown operation without mode", cfg: InvokeConfig{}, want: "--output is required"},
	}
	for _, testCase := range validationCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			catalog := detail
			operation := "OMNI"
			if testCase.name == "unknown operation without mode" {
				operation = "UNKNOWN"
			}
			if err := validateCLIOutputShape(testCase.cfg, catalog, operation); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateCLIOutputShape() = %v, want error containing %q", err, testCase.want)
			}
		})
	}

	if err := validateCLIOutputShape(InvokeConfig{JSON: true}, detail, "OMNI"); err != nil {
		t.Fatalf("JSON output shape = %v, want success", err)
	}
	audioDetail := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "AUDIO", Outputs: []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}},
	}}}}
	if err := validateCLIOutputShape(InvokeConfig{OutputPath: "speech.wav"}, audioDetail, "AUDIO"); err != nil {
		t.Fatalf("audio output shape = %v, want success", err)
	}
	inlineDetail := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "TEXT", Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
	if err := validateCLIOutputShape(InvokeConfig{}, inlineDetail, "TEXT"); err != nil {
		t.Fatalf("inline output shape = %v, want success", err)
	}

	mappingCases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "missing equals", values: []string{"text"}, want: "expected slot=path"},
		{name: "empty slot", values: []string{"=text.out"}, want: "slot and path are required"},
		{name: "dash path", values: []string{"text=-"}, want: "path '-'"},
		{name: "duplicate slot", values: []string{"text=a", "text=b"}, want: "duplicate"},
		{name: "duplicate path", values: []string{"text=a", "usage=a"}, want: "same path"},
	}
	for _, testCase := range mappingCases {
		testCase := testCase
		t.Run("mapping/"+testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseGenericCLIOutputMappings(testCase.values); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parseGenericCLIOutputMappings() = %v, want error containing %q", err, testCase.want)
			}
		})
	}
	if err := validateGenericCLIOutputMappings([]string{"text=a", "usage=b"}, detail.Operations[0], true); err != nil {
		t.Fatalf("valid output mappings = %v, want success", err)
	}
	if err := validateGenericCLIOutputMappings([]string{"text=a"}, detail.Operations[0], false); err == nil || !strings.Contains(err.Error(), "unknown operation") {
		t.Fatalf("unknown operation mappings = %v, want unknown-operation error", err)
	}

	var output bytes.Buffer
	if err := writeGenericCLIOutput(&output, modelinference.InvokeModelResult{}); err == nil {
		t.Fatal("empty generic output = nil, want shape error")
	}
	if err := writeGenericCLIOutput(&output, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: ""}}}); err == nil {
		t.Fatal("empty inline content = nil, want content error")
	}
	if err := writeGenericCLIOutput(&output, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "a"}, {Content: "b"}}}); err == nil {
		t.Fatal("multiple generic outputs = nil, want shape error")
	}
	if err := writeGenericCLIOutput(&output, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}); err != nil || output.String() != "answer" {
		t.Fatalf("successful generic output = (%v, %q), want answer", err, output.String())
	}

	if genericCLIJSONResult(InvokeConfig{}, inlineDetail, "TEXT", modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}) {
		t.Fatal("non-JSON result classified as JSON")
	}
	if !genericCLIJSONResult(InvokeConfig{JSON: true}, detail, "OMNI", modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{}, {}}}) {
		t.Fatal("multiple JSON outputs not classified as JSON")
	}
	if !genericCLIInlineOutput(InvokeConfig{}, inlineDetail, "TEXT") || genericCLIInlineOutput(InvokeConfig{OutputPath: "speech.wav"}, inlineDetail, "TEXT") {
		t.Fatal("inline output classification did not honor modality or output path")
	}
	for _, modality := range []modelinference.Modality{modelinference.ModalityText, modelinference.ModalityJSON} {
		if !genericCLIInlineModality(modality) {
			t.Fatalf("modality %q = false, want inline", modality)
		}
	}
	if genericCLIInlineModality(modelinference.ModalityAudio) {
		t.Fatal("audio modality classified as inline")
	}
}

func TestCLIOutputContentAndPresentationProjectionBranches(t *testing.T) {
	t.Parallel()

	required := true
	optional := false
	inputs := []modelinference.OperationSlot{
		{Name: "audio", Modality: modelinference.ModalityAudio},
		{Name: "optional", Modality: modelinference.ModalityText, Required: &optional},
		{Name: "text", Modality: modelinference.ModalityText, Required: &required, MediaTypes: []string{"text/custom"}},
	}
	if input := joinedCLITextInput(inputs); input == nil || input.Name != "text" {
		t.Fatalf("joinedCLITextInput() = %#v, want required text input", input)
	}
	if input := joinedCLITextInput([]modelinference.OperationSlot{{Name: "optional", Modality: modelinference.ModalityText, Required: &optional}}); input == nil || input.Name != "optional" {
		t.Fatalf("optional joinedCLITextInput() = %#v, want optional text input", input)
	}
	if input := joinedCLITextInput([]modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}}); input != nil {
		t.Fatalf("non-text joinedCLITextInput() = %#v, want nil", input)
	}

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("output-coverage:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	catalog := modelinference.Detail{Summary: modelinference.Summary{
		ProviderLocality: modelinference.LocalityCloud,
	},
		Capabilities: []modelinference.Capability{{
			Worker: "text-worker", ProviderLocality: modelinference.LocalityCloud,
			Operations: []modelinference.Operation{{Name: "OMNI", Inputs: inputs}},
		}},
	}
	request := joinedCLIInvocationRequest(scope, "model", "OMNI", "hello", catalog)
	if request.Inputs[0].Name != "text" || request.Inputs[0].ContentType != "text/custom" || request.Inputs[0].Content != "hello" {
		t.Fatalf("joined request = %#v, want selected text slot", request)
	}
	if worker, locality := catalogPresentationForOperation(catalog, "OMNI"); worker != "text-worker" || locality != string(modelinference.LocalityCloud) {
		t.Fatalf("catalog presentation = (%q, %q), want text-worker/REMOTE", worker, locality)
	}
	if worker, locality := catalogPresentationForOperation(catalog, "MISSING"); worker != "" || locality != string(modelinference.LocalityCloud) {
		t.Fatalf("catalog fallback presentation = (%q, %q), want empty/REMOTE", worker, locality)
	}
	bindings := resolvedPresentationBindings(catalog, "OMNI", "hello")
	if len(bindings) != 1 || bindings[0].Slot != "audio" || bindings[0].Source != "INPUT" || bindings[0].Content[0].Text != "hello" {
		t.Fatalf("presentation bindings = %#v, want first named input binding", bindings)
	}
	if bindings := resolvedPresentationBindings(catalog, "MISSING", "hello"); len(bindings) != 0 {
		t.Fatalf("missing presentation bindings = %#v, want empty", bindings)
	}

	ref, err := (modelinference.InferenceArtifactRef{}).Parse("artifact:usage")
	if err != nil {
		t.Fatalf("parse artifact ref: %v", err)
	}
	parts := inferenceContentToWorkParts([]modelinference.InferenceContent{
		{ContentType: "audio/wav", Content: "speech.wav"},
		{ContentType: "image/png", Content: "https://image"},
		{ContentType: "application/json", Content: `{"ok":true}`},
		{Content: "plain text"},
		{ContentType: "text/custom", Content: "custom"},
	})
	if len(parts) != 5 || parts[0].Type != work.WorkContentPartTypeAudio || parts[1].Type != work.WorkContentPartTypeImage || parts[2].Type != work.WorkContentPartTypeJSON || parts[3].Type != work.WorkContentPartTypeText {
		t.Fatalf("content parts = %#v, want audio/image/json/text projections", parts)
	}
	var parsedJSON map[string]any
	if err := json.Unmarshal(parts[2].JSON, &parsedJSON); err != nil || parsedJSON["ok"] != true {
		t.Fatalf("JSON content part = %q, %v", parts[2].JSON, err)
	}
	if parts[0].File != "speech.wav" || parts[1].URL != "https://image" || parts[3].ContentType != "text/plain" {
		t.Fatalf("content part payloads = %#v, want projected payloads", parts)
	}

	result := modelinference.InvokeModelResult{
		ModelName: "model", Operation: "OMNI",
		Content:   []modelinference.InferenceContent{{ContentType: "text/plain", Content: "answer"}},
		Artifacts: []modelinference.InferenceArtifact{{Artifact: ref}},
		Outputs: []modelinference.InferenceOutput{{
			Name: "usage", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"tokens":2}`,
			Artifact: &modelinference.InferenceArtifact{Artifact: ref, Name: "usage.json", MediaType: "application/json", SizeBytes: 7, Properties: map[string]string{"digest": "sha"}},
		}, {
			Name: "empty-artifact", Artifact: &modelinference.InferenceArtifact{},
		}},
	}
	response := genericInvocationResponseFromInferenceResult(result)
	if len(response.Outputs) != 2 || response.Outputs[0].Artifact == nil || response.Outputs[0].Artifact.SizeBytes == nil || *response.Outputs[0].Artifact.SizeBytes != 7 || response.Outputs[1].Artifact != nil {
		t.Fatalf("generic response = %#v, want projected artifact metadata", response)
	}
	if genericCLIStringPointer("") != nil || genericCLIStringPointer(" value ") == nil {
		t.Fatal("generic string pointer normalization did not distinguish empty and non-empty values")
	}

	legacyResponse := modelInvocationResponseFromInferenceResult(result, catalog, "hello")
	if legacyResponse.Worker != "text-worker" || legacyResponse.ProviderLocality != factoryapi.WorkerModelLocalityCloud || len(legacyResponse.Content) != 1 {
		t.Fatalf("legacy response = %#v, want presentation worker/locality/content", legacyResponse)
	}
	if path, err := inferenceArtifactSourcePath(result); err != nil || path != "artifact:usage" {
		t.Fatalf("artifact source = (%q, %v), want artifact:usage", path, err)
	}
	if _, err := inferenceArtifactSourcePath(modelinference.InvokeModelResult{}); err == nil || !strings.Contains(err.Error(), "no streamed audio") {
		t.Fatal("missing artifact source = nil, want missing-audio error")
	}
}
