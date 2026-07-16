package inference

import (
	"encoding/json"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestResolveInferenceOperationBindings_InferenceAndLegacyWorkstationTypesAlign(t *testing.T) {
	worker := inferenceBindingWorkerFixture()
	inputTokens := inferenceBindingInputTokensFixture()

	inferenceBindings := mustResolveInferenceBindings(t, workertaxonomy.WorkstationTypeInference, worker, inputTokens)
	legacyBindings := mustResolveInferenceBindings(t, workertaxonomy.WorkstationTypeInvoke, worker, inputTokens)

	assertInferenceBindingFixtureBindings(t, inferenceBindings)
	assertInferenceBindingFixtureBindings(t, legacyBindings)
	assertInferenceBindingsAligned(t, inferenceBindings, legacyBindings)
}

func TestWorkContentFromInferenceOutput_OrdersAudioBeforeExtraParts(t *testing.T) {
	t.Parallel()

	operation := workerconfig.ModelOperation{
		Name: "TTS",
		Outputs: []workerconfig.ModelOperationSlot{
			{Name: "audio", ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio}},
			{Name: "meta", ContentTypes: []string{workerconfig.ModelOperationContentTypeJSON}},
		},
	}
	raw, err := json.Marshal([]work.WorkContentPart{
		{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"voice":"alloy"}`)},
		{Type: work.WorkContentPartTypeAudio, File: "/tmp/speech.wav", ContentType: "audio/wav"},
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
	if got[0].Type != work.WorkContentPartTypeAudio || got[1].Type != work.WorkContentPartTypeJSON {
		t.Fatalf("ordered content = %#v, want audio before json", got)
	}
}

func TestWorkContentFromInferenceOutput_PreservesTextFallbackForTextOnlyOperations(t *testing.T) {
	t.Parallel()

	got, err := WorkContentFromInferenceOutput("plain answer", workerconfig.ModelOperation{
		Name:    "GENERATE",
		Outputs: []workerconfig.ModelOperationSlot{{Name: "text", ContentTypes: []string{workerconfig.ModelOperationContentTypeText}}},
	})
	if err != nil {
		t.Fatalf("WorkContentFromInferenceOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeText || got[0].Text != "plain answer" {
		t.Fatalf("content = %#v, want text fallback", got)
	}
}

func TestWorkContentFromInferenceOutputAcceptsGeneratedUnionJSON(t *testing.T) {
	t.Parallel()

	var generatedPart factoryapi.WorkContentPart
	if err := generatedPart.FromWorkJsonContentPart(factoryapi.WorkJsonContentPart{
		Type: factoryapi.WorkContentPartTypeJSON,
		Json: map[string]any{"voice": "alloy"},
	}); err != nil {
		t.Fatalf("build generated content: %v", err)
	}
	raw, err := json.Marshal(factoryapi.WorkContent{generatedPart})
	if err != nil {
		t.Fatalf("marshal generated content: %v", err)
	}

	got, err := WorkContentFromInferenceOutput(string(raw), workerconfig.ModelOperation{
		Name: "TTS",
		Outputs: []workerconfig.ModelOperationSlot{{
			Name:         "metadata",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeJSON},
		}},
	})
	if err != nil {
		t.Fatalf("WorkContentFromInferenceOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeJSON || string(got[0].JSON) != `{"voice":"alloy"}` {
		t.Fatalf("content = %#v, want generated JSON mapped to canonical content", got)
	}
}

func TestDirectAndSessionInferenceOutputShapingStayAligned(t *testing.T) {
	t.Parallel()

	audioPath := "/tmp/direct-session-parity.wav"
	providerRaw, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal provider output: %v", err)
	}

	operation := workerconfig.ModelOperation{
		Name: "TTS",
		Outputs: []workerconfig.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
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

func inferenceBindingInputTokensFixture() []factorytoken.Token {
	return []factorytoken.Token{{
		ID: "token-tts",
		Color: factorytoken.Color{
			Content: []work.WorkContentPart{{
				Type:  work.WorkContentPartTypeText,
				Label: "utterance",
				Text:  "hello world",
			}},
		},
	}}
}

func mustResolveInferenceBindings(
	t *testing.T,
	workstationType string,
	worker *workerconfig.Config,
	inputTokens []factorytoken.Token,
) []workerexecution.ResolvedModelOperationBinding {
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

func assertInferenceBindingFixtureBindings(t *testing.T, bindings []workerexecution.ResolvedModelOperationBinding) {
	t.Helper()
	if len(bindings) != 2 {
		t.Fatalf("bindings = %#v, want 2 resolved slots", bindings)
	}
	assertInferenceBindingTextSlot(t, bindings[0])
	assertInferenceBindingVoiceSlot(t, bindings[1])
}

func assertInferenceBindingTextSlot(t *testing.T, binding workerexecution.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "text" || binding.Source != workerexecution.ModelOperationBindingSourceInput || binding.Content[0].Text != "hello world" {
		t.Fatalf("text binding = %#v, want input text binding", binding)
	}
}

func assertInferenceBindingVoiceSlot(t *testing.T, binding workerexecution.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "voice" || binding.Source != workerexecution.ModelOperationBindingSourceConfig || string(binding.Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want config voice binding", binding)
	}
}

func assertInferenceBindingsAligned(t *testing.T, inference, legacy []workerexecution.ResolvedModelOperationBinding) {
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

func inferenceBindingWorkerFixture() *workerconfig.Config {
	return &workerconfig.Config{
		Name: "tts-worker",
		Type: workertaxonomy.WorkerTypeInference,
		Operations: []workerconfig.ModelOperation{{
			Name: "TTS",
			Inputs: []workerconfig.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{workerconfig.ModelOperationContentTypeText}, Required: true},
				{Name: "voice", ContentTypes: []string{workerconfig.ModelOperationContentTypeJSON}},
			},
			Outputs: []workerconfig.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
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
					Type:  workerconfig.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Config: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		},
	}
}
