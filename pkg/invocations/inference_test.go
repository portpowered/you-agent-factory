package invocations

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestResolveInferenceOperationBindings_InferenceAndLegacyWorkstationTypesAlign(t *testing.T) {
	worker := inferenceBindingWorkerFixture()
	inputTokens := inferenceBindingInputTokensFixture()

	inferenceBindings := mustResolveInferenceBindings(t, interfaces.WorkstationTypeInference, worker, inputTokens)
	legacyBindings := mustResolveInferenceBindings(t, interfaces.WorkstationTypeInvoke, worker, inputTokens)

	assertInferenceBindingFixtureBindings(t, inferenceBindings)
	assertInferenceBindingFixtureBindings(t, legacyBindings)
	assertInferenceBindingsAligned(t, inferenceBindings, legacyBindings)
}

func TestWorkContentFromInferenceOutput_OrdersAudioBeforeExtraParts(t *testing.T) {
	t.Parallel()

	operation := interfaces.ModelOperation{
		Name: "TTS",
		Outputs: []interfaces.ModelOperationSlot{
			{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}},
			{Name: "meta", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
		},
	}
	raw, err := json.Marshal([]interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeJSON, JSON: []byte(`{"voice":"alloy"}`)},
		{Type: interfaces.WorkContentPartTypeAudio, File: "/tmp/speech.wav", ContentType: "audio/wav"},
	})
	if err != nil {
		t.Fatalf("marshal fixture output: %v", err)
	}

	got, err := WorkContentFromInferenceOutput(string(raw), operation)
	if err != nil {
		t.Fatalf("WorkContentFromInferenceOutput: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("content = %#v, want 2 ordered parts", got)
	}
	if got[0].Type != interfaces.WorkContentPartTypeAudio || got[1].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("ordered content = %#v, want audio before json", got)
	}
}

func TestWorkContentFromInferenceOutput_PreservesTextFallbackForTextOnlyOperations(t *testing.T) {
	t.Parallel()

	got, err := WorkContentFromInferenceOutput("plain answer", interfaces.ModelOperation{
		Name:    "GENERATE",
		Outputs: []interfaces.ModelOperationSlot{{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}}},
	})
	if err != nil {
		t.Fatalf("WorkContentFromInferenceOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != interfaces.WorkContentPartTypeText || got[0].Text != "plain answer" {
		t.Fatalf("content = %#v, want text fallback", got)
	}
}

func TestDirectAndSessionInferenceOutputShapingStayAligned(t *testing.T) {
	t.Parallel()

	audioPath := "/tmp/direct-session-parity.wav"
	providerRaw, err := json.Marshal([]interfaces.WorkContentPart{{
		Type:        interfaces.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal provider output: %v", err)
	}

	operation := interfaces.ModelOperation{
		Name: "TTS",
		Outputs: []interfaces.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
		}},
	}

	directContent, err := WorkContentFromInferenceOutput(string(providerRaw), operation)
	if err != nil {
		t.Fatalf("direct WorkContentFromInferenceOutput: %v", err)
	}
	sessionContent, err := WorkContentFromInferenceOutput(string(providerRaw), operation)
	if err != nil {
		t.Fatalf("session WorkContentFromInferenceOutput: %v", err)
	}
	if len(directContent) != len(sessionContent) {
		t.Fatalf("direct vs session content count = %d vs %d", len(directContent), len(sessionContent))
	}
	if directContent[0].Type != sessionContent[0].Type || directContent[0].File != sessionContent[0].File {
		t.Fatalf("direct = %#v session = %#v, want equivalent canonical output", directContent, sessionContent)
	}
}

func TestOperationBindingsFromGenerated_MapsSelectorFields(t *testing.T) {
	t.Parallel()

	textType := factoryapi.ModelOperationContentTypeText
	bindings := OperationBindingsFromGenerated(&[]factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Label: stringPtr("utterance"),
			Type:  &textType,
		},
	}})
	if len(bindings) != 1 || bindings[0].Slot != "text" || bindings[0].Selector.Label != "utterance" {
		t.Fatalf("bindings = %#v, want mapped selector", bindings)
	}
}

func inferenceBindingInputTokensFixture() []interfaces.Token {
	return []interfaces.Token{{
		ID: "token-tts",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{{
				Type:  interfaces.WorkContentPartTypeText,
				Label: "utterance",
				Text:  "hello world",
			}},
		},
	}}
}

func mustResolveInferenceBindings(
	t *testing.T,
	workstationType string,
	worker *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) []interfaces.ResolvedModelOperationBinding {
	t.Helper()
	bindings, err := ResolveInferenceOperationBindings(
		inferenceBindingWorkstationFixture(workstationType),
		worker,
		inputTokens,
	)
	if err != nil {
		t.Fatalf("ResolveInferenceOperationBindings %s: %v", workstationType, err)
	}
	return bindings
}

func assertInferenceBindingFixtureBindings(t *testing.T, bindings []interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if len(bindings) != 2 {
		t.Fatalf("bindings = %#v, want 2 resolved slots", bindings)
	}
	assertInferenceBindingTextSlot(t, bindings[0])
	assertInferenceBindingVoiceSlot(t, bindings[1])
}

func assertInferenceBindingTextSlot(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "text" || binding.Source != interfaces.ModelOperationBindingSourceInput || binding.Content[0].Text != "hello world" {
		t.Fatalf("text binding = %#v, want input text binding", binding)
	}
}

func assertInferenceBindingVoiceSlot(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "voice" || binding.Source != interfaces.ModelOperationBindingSourceConfig || string(binding.Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want config voice binding", binding)
	}
}

func assertInferenceBindingsAligned(t *testing.T, inference, legacy []interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if len(legacy) != len(inference) {
		t.Fatalf("legacy vs inference binding count = %d vs %d", len(legacy), len(inference))
	}
	for i := range legacy {
		if legacy[i].Slot != inference[i].Slot || legacy[i].Source != inference[i].Source {
			t.Fatalf("binding[%d] = %#v vs %#v, want aligned slot/source", i, legacy[i], inference[i])
		}
		if len(legacy[i].Content) != len(inference[i].Content) || legacy[i].Content[0].Text != inference[i].Content[0].Text {
			t.Fatalf("binding[%d] content = %#v vs %#v, want aligned content", i, legacy[i].Content, inference[i].Content)
		}
	}
}

func inferenceBindingWorkerFixture() *interfaces.WorkerConfig {
	return &interfaces.WorkerConfig{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeInference,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
				{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
			},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}
}

func inferenceBindingWorkstationFixture(workstationType string) *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Type:      workstationType,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{
			{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
