package invocations

import (
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this binding-alignment test keeps legacy and inference workstation assertions together on one contract surface.
func TestResolveInferenceOperationBindings_InferenceAndLegacyWorkstationTypesAlign(t *testing.T) {
	worker := inferenceBindingWorkerFixture()
	inputTokens := []interfaces.Token{{
		ID: "token-tts",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{{
				Type:  interfaces.WorkContentPartTypeText,
				Label: "utterance",
				Text:  "hello world",
			}},
		},
	}}

	inferenceBindings, err := ResolveInferenceOperationBindings(
		inferenceBindingWorkstationFixture(interfaces.WorkstationTypeInference),
		worker,
		inputTokens,
	)
	if err != nil {
		t.Fatalf("ResolveInferenceOperationBindings inference run: %v", err)
	}
	legacyBindings, err := ResolveInferenceOperationBindings(
		inferenceBindingWorkstationFixture(interfaces.WorkstationTypeInvoke),
		worker,
		inputTokens,
	)
	if err != nil {
		t.Fatalf("ResolveInferenceOperationBindings legacy model invoke: %v", err)
	}

	for _, got := range [][]interfaces.ResolvedModelOperationBinding{inferenceBindings, legacyBindings} {
		if len(got) != 2 {
			t.Fatalf("bindings = %#v, want 2 resolved slots", got)
		}
		if got[0].Slot != "text" || got[0].Source != interfaces.ModelOperationBindingSourceInput || got[0].Content[0].Text != "hello world" {
			t.Fatalf("text binding = %#v, want input text binding", got[0])
		}
		if got[1].Slot != "voice" || got[1].Source != interfaces.ModelOperationBindingSourceConfig || string(got[1].Content[0].JSON) != `{"name":"alloy"}` {
			t.Fatalf("voice binding = %#v, want config voice binding", got[1])
		}
	}

	if len(legacyBindings) != len(inferenceBindings) {
		t.Fatalf("legacy vs inference binding count = %d vs %d", len(legacyBindings), len(inferenceBindings))
	}
	for i := range legacyBindings {
		if legacyBindings[i].Slot != inferenceBindings[i].Slot || legacyBindings[i].Source != inferenceBindings[i].Source {
			t.Fatalf("binding[%d] = %#v vs %#v, want aligned slot/source", i, legacyBindings[i], inferenceBindings[i])
		}
		if len(legacyBindings[i].Content) != len(inferenceBindings[i].Content) || legacyBindings[i].Content[0].Text != inferenceBindings[i].Content[0].Text {
			t.Fatalf("binding[%d] content = %#v vs %#v, want aligned content", i, legacyBindings[i].Content, inferenceBindings[i].Content)
		}
	}
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
