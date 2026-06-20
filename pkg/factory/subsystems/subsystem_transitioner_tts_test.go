package subsystems

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
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

func TestCalculateMutations_PackagedGoalReplacesTerminalContentWithSummary(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           goal.PackagedInvokeWorkstationName,
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "goal-executor",
	}
	workerOutput := "Final goal summary.\nCOMPLETE"

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "goal:complete",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: interfaces.OutcomeAccepted, output: workerOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.WorkerConfig{
				"goal-executor": {
					Name:      "goal-executor",
					StopToken: "COMPLETE",
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

	token := mutations[0].NewToken
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != interfaces.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one text summary part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "Final goal summary." {
		t.Fatalf("terminal content = %q, want worker summary without stop token", token.Color.Content[0].Text)
	}
	if len(token.Color.Payload) != 0 {
		t.Fatalf("terminal payload = %q, want cleared so submitted input does not leak", string(token.Color.Payload))
	}
}

func TestCalculateMutations_PackagedGoalReviewClassifierPreservesCarriedSummary(t *testing.T) {
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	places := map[string]*petri.Place{
		"goal:review":   {ID: "goal:review", TypeID: goal.PackagedGoalWorkTypeName, State: "review"},
		"goal:complete": {ID: "goal:complete", TypeID: goal.PackagedGoalWorkTypeName, State: "complete"},
	}
	workTypes := map[string]*state.WorkType{
		goal.PackagedGoalWorkTypeName: {ID: goal.PackagedGoalWorkTypeName},
	}
	summaryContent, err := goal.SummaryContentFromWorkerOutput("Final goal summary.\nCOMPLETE", "COMPLETE")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}
	consumed := []interfaces.Token{{
		ID:        "tok-1",
		PlaceID:   "goal:review",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "w1",
			WorkTypeID: goal.PackagedGoalWorkTypeName,
			Name:       "story-1",
			Content:    summaryContent,
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           goal.PackagedReviewWorkstationName,
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "goal-reviewer",
	}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  &petri.Transition{ID: "t1"},
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "goal:complete",
		}},
		consumed:    consumed,
		result:      resolvedWorkResult{outcome: interfaces.OutcomeAccepted, output: "accepted"},
		now:         now,
		history:     consumed[0].History,
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.WorkerConfig{
				"goal-reviewer": {
					Name:      "goal-reviewer",
					StopToken: "COMPLETE",
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

	token := mutations[0].NewToken
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != interfaces.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one carried text summary part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "Final goal summary." {
		t.Fatalf("terminal content = %q, want carried execution summary not classifier label", token.Color.Content[0].Text)
	}
}
