package subsystems

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestCalculateMutations_PackagedTTSReplacesTerminalContentWithMetadata(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:      tts.PackagedInvokeWorkstationName,
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
	}
	audioOutput := `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "wt-code:done",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: interfaces.OutcomeContinue, output: audioOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
	})
	if err != nil {
		t.Fatalf("calculateMutations: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}

	token := mutations[0].NewToken
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != interfaces.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one text metadata part", token.Color.Content)
	}
	if len(token.Color.Payload) != 0 {
		t.Fatalf("terminal payload = %q, want cleared so raw audio does not leak", string(token.Color.Payload))
	}
	if strings.Contains(token.Color.Content[0].Text, `"type":"AUDIO"`) {
		t.Fatalf("terminal content = %q, want metadata JSON not raw audio content", token.Color.Content[0].Text)
	}

	var metadata tts.InvocationMetadata
	if err := json.Unmarshal([]byte(token.Color.Content[0].Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.ArtifactPath != "/tmp/speech.wav" || metadata.MediaType != "audio/wav" {
		t.Fatalf("metadata = %#v, want speech artifact metadata", metadata)
	}
}

func TestCalculateMutations_PackagedTTSUsesEditedWorkerBackendFromRuntimeConfig(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:      tts.PackagedInvokeWorkstationName,
		Type:      interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "tts-executor",
		Operation: "TTS",
	}
	audioOutput := `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "wt-code:done",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: interfaces.OutcomeContinue, output: audioOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.WorkerConfig{
				"tts-executor": {
					Name:  "tts-executor",
					Model: "CUSTOMER_EDITED_TTS_MODEL",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("calculateMutations: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}

	var metadata tts.InvocationMetadata
	if err := json.Unmarshal([]byte(mutations[0].NewToken.Color.Content[0].Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.Backend != "CUSTOMER_EDITED_TTS_MODEL/LLAMACPP" {
		t.Fatalf("metadata backend = %q, want edited worker backend label", metadata.Backend)
	}
}
