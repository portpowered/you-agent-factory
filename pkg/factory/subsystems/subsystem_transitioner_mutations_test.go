package subsystems

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/packages/subagent"
	"github.com/portpowered/infinite-you/pkg/factory/packages/tts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type calculateMutationsFixture struct {
	now         time.Time
	baseHistory factorytoken.History
	transition  *petri.Transition
	consumed    []factorytoken.Token
	inputColors []factorytoken.Color
	transformer *token_transformer.Transformer
}

func newCalculateMutationsFixture() calculateMutationsFixture {
	now := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	places := map[string]*petri.Place{
		"wt-code:init":   {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
		"wt-code:done":   {ID: "wt-code:done", TypeID: "wt-code", State: "done"},
		"wt-code:failed": {ID: "wt-code:failed", TypeID: "wt-code", State: "failed"},
		"wt-review:init": {ID: "wt-review:init", TypeID: "wt-review", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-code":   {ID: "wt-code"},
		"wt-review": {ID: "wt-review"},
	}
	consumed := []factorytoken.Token{{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "w1",
			WorkTypeID: "wt-code",
			Name:       "story-1",
		},
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}
	return calculateMutationsFixture{
		now: now,
		baseHistory: factorytoken.History{
			TotalVisits:         map[string]int{"t0": 1},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{"wt-code:init": 1},
		},
		transition:  &petri.Transition{ID: "t1"},
		consumed:    consumed,
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
	}
}

func (f calculateMutationsFixture) calculate(arcs []petri.Arc, result resolvedWorkResult) ([]interfaces.MarkingMutation, error) {
	return calculateMutations(mutationCalculationInput{
		transition:  f.transition,
		arcs:        arcs,
		consumed:    f.consumed,
		result:      result,
		now:         f.now,
		history:     f.baseHistory,
		inputColors: f.inputColors,
		transformer: f.transformer,
	})
}

func assertCalculatedMutation(t *testing.T, mutations []interfaces.MarkingMutation, want struct {
	name            string
	arcs            []petri.Arc
	result          resolvedWorkResult
	wantPlace       string
	wantWorkTypeID  string
	wantWorkID      string
	wantPayload     []byte
	wantFeedback    string
	wantLastError   string
	wantFailureSize int
	wantCreatedAt   time.Time
}, now time.Time) {
	t.Helper()
	if len(mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(mutations))
	}

	token := mutations[0].NewToken
	if mutations[0].ToPlace != want.wantPlace {
		t.Fatalf("ToPlace = %s, want %s", mutations[0].ToPlace, want.wantPlace)
	}
	if token.Color.WorkTypeID != want.wantWorkTypeID || token.Color.WorkID != want.wantWorkID {
		t.Fatalf("token identity = (%s,%s), want (%s,%s)", token.Color.WorkTypeID, token.Color.WorkID, want.wantWorkTypeID, want.wantWorkID)
	}
	if !token.CreatedAt.Equal(want.wantCreatedAt) || !token.EnteredAt.Equal(now) {
		t.Fatalf("token timestamps = (%v,%v), want (%v,%v)", token.CreatedAt, token.EnteredAt, want.wantCreatedAt, now)
	}
	if !bytes.Equal(token.Color.Payload, want.wantPayload) {
		t.Fatalf("Payload = %q, want %q", token.Color.Payload, want.wantPayload)
	}
	if got := token.Color.Tags[interfaces.RejectionFeedback]; got != want.wantFeedback {
		t.Fatalf("rejection feedback = %q, want %q", got, want.wantFeedback)
	}
	if token.History.LastError != want.wantLastError {
		t.Fatalf("LastError = %q, want %q", token.History.LastError, want.wantLastError)
	}
	if len(token.History.FailureLog) != want.wantFailureSize {
		t.Fatalf("FailureLog length = %d, want %d", len(token.History.FailureLog), want.wantFailureSize)
	}
}

func TestCalculateMutations_PreserveInput_KeepsConsumedPayloadForDownstreamWork(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.consumed[0].Color.Tags = map[string]string{"objective": "goal-1"}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
		&interfaces.FactoryWorkstationConfig{
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModePreserveInput,
			},
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	if string(mutations[0].NewToken.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload", mutations[0].NewToken.Color.Payload)
	}
	if len(mutations[0].NewToken.Color.Content) != 1 || mutations[0].NewToken.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want preserved input content", mutations[0].NewToken.Color.Content)
	}
	if mutations[0].NewToken.Color.Tags["objective"] != "goal-1" {
		t.Fatalf("tags = %#v, want preserved input tags", mutations[0].NewToken.Color.Tags)
	}
}

func TestCalculateMutations_OmittedWorkPropagation_UsesWorkerOutputPayload(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculate(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	if string(mutations[0].NewToken.Color.Payload) != "worker-output" {
		t.Fatalf("payload = %q, want worker-output", mutations[0].NewToken.Color.Payload)
	}
}

func preserveInputWorkstation() *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		WorkPropagation: &interfaces.WorkPropagationConfig{
			Mode: interfaces.WorkPropagationModePreserveInput,
		},
	}
}

func TestCalculateMutations_PreserveInput_OutcomeLanes_KeepConsumedWorkData(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.consumed[0].Color.Tags = map[string]string{"objective": "goal-1"}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	tests := []struct {
		name   string
		arcs   []petri.Arc
		result resolvedWorkResult
	}{
		{
			name: "Continue",
			arcs: []petri.Arc{{ID: "continue", PlaceID: "wt-code:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeContinue,
				output:       "worker-output",
				feedback:     "needs revision",
			},
		},
		{
			name: "Rejected",
			arcs: []petri.Arc{{ID: "reject", PlaceID: "wt-code:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeRejected,
				output:       "worker-output",
				feedback:     "rejected",
			},
		},
		{
			name: "Failed",
			arcs: []petri.Arc{{ID: "fail", PlaceID: "wt-code:failed"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      workerexecution.OutcomeFailed,
				output:       "worker-output",
				err:          "agent crashed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutations, err := fixture.calculateWithWorkstation(tt.arcs, tt.result, preserveInputWorkstation())
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			if len(mutations) != 1 {
				t.Fatalf("mutation count = %d, want 1", len(mutations))
			}
			token := mutations[0].NewToken
			if string(token.Color.Payload) != "input-payload" {
				t.Fatalf("payload = %q, want input-payload", token.Color.Payload)
			}
			if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
				t.Fatalf("content = %#v, want preserved input content", token.Color.Content)
			}
			if token.Color.Tags["objective"] != "goal-1" {
				t.Fatalf("tags = %#v, want preserved input tags", token.Color.Tags)
			}
		})
	}
}

func TestCalculateMutations_PreserveInput_MultiInput_UsesPrimaryNonResourceInput(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed = []factorytoken.Token{
		{
			ID:      "tok-primary",
			PlaceID: "wt-objective:init",
			Color: factorytoken.Color{
				WorkID:     "work-objective",
				WorkTypeID: "wt-objective",
				Payload:    []byte("primary-input-payload"),
				Tags:       map[string]string{"role": "primary"},
			},
			CreatedAt: fixture.now.Add(-2 * time.Hour),
			EnteredAt: fixture.now.Add(-2 * time.Hour),
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
		{
			ID:      "tok-secondary",
			PlaceID: "wt-context:init",
			Color: factorytoken.Color{
				WorkID:     "work-context",
				WorkTypeID: "wt-context",
				Payload:    []byte("secondary-input-payload"),
				Tags:       map[string]string{"role": "secondary"},
			},
			CreatedAt: fixture.now.Add(-time.Hour),
			EnteredAt: fixture.now.Add(-time.Hour),
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
	}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)
	places := map[string]*petri.Place{
		"wt-objective:init": {ID: "wt-objective:init", TypeID: "wt-objective", State: "init"},
		"wt-context:init":   {ID: "wt-context:init", TypeID: "wt-context", State: "init"},
		"wt-review:init":    {ID: "wt-review:init", TypeID: "wt-review", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-objective": {ID: "wt-objective"},
		"wt-context":   {ID: "wt-context"},
		"wt-review":    {ID: "wt-review"},
	}
	fixture.transformer = token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator()))

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
		preserveInputWorkstation(),
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	token := mutations[0].NewToken
	if string(token.Color.Payload) != "primary-input-payload" {
		t.Fatalf("payload = %q, want primary-input-payload", token.Color.Payload)
	}
	if token.Color.Tags["role"] != "primary" {
		t.Fatalf("tags = %#v, want primary input tags", token.Color.Tags)
	}
}

func TestCalculateMutations_PreserveInput_MultiOutput_AllLanesKeepConsumedWorkData(t *testing.T) {
	now := time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	places := map[string]*petri.Place{
		"wt-code:init":     {ID: "wt-code:init", TypeID: "wt-code", State: "init"},
		"wt-review-a:init": {ID: "wt-review-a:init", TypeID: "wt-review-a", State: "init"},
		"wt-review-b:init": {ID: "wt-review-b:init", TypeID: "wt-review-b", State: "init"},
		"wt-review-c:init": {ID: "wt-review-c:init", TypeID: "wt-review-c", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"wt-code":     {ID: "wt-code"},
		"wt-review-a": {ID: "wt-review-a"},
		"wt-review-b": {ID: "wt-review-b"},
		"wt-review-c": {ID: "wt-review-c"},
	}
	consumed := []factorytoken.Token{{
		ID:      "tok-1",
		PlaceID: "wt-code:init",
		Color: factorytoken.Color{
			WorkID:     "work-code-1",
			WorkTypeID: "wt-code",
			Payload:    []byte("input-payload"),
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "input-content",
			}},
			Tags: map[string]string{"objective": "goal-1"},
		},
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  &petri.Transition{ID: "t1"},
		workstation: preserveInputWorkstation(),
		arcs: []petri.Arc{
			{ID: "review-a", PlaceID: "wt-review-a:init"},
			{ID: "review-b", PlaceID: "wt-review-b:init"},
			{ID: "review-c", PlaceID: "wt-review-c:init"},
		},
		consumed:    consumed,
		result:      resolvedWorkResult{transitionID: "t1", outcome: workerexecution.OutcomeAccepted, output: "worker-output"},
		now:         now,
		history:     factorytoken.History{},
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
	})
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 3 {
		t.Fatalf("mutation count = %d, want 3", len(mutations))
	}
	for i, mutation := range mutations {
		token := mutation.NewToken
		if string(token.Color.Payload) != "input-payload" {
			t.Fatalf("mutation %d payload = %q, want input-payload", i, token.Color.Payload)
		}
		if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
			t.Fatalf("mutation %d content = %#v, want preserved input content", i, token.Color.Content)
		}
		if token.Color.Tags["objective"] != "goal-1" {
			t.Fatalf("mutation %d tags = %#v, want preserved input tags", i, token.Color.Tags)
		}
	}
}

func TestCalculateMutations_PreserveInput_RecordedOutputWork_KeepsExplicitContent(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
			recordedOutputWork: []work.FactoryWorkItem{{
				ID:          "work-review-99",
				WorkTypeID:  "wt-review",
				DisplayName: "review-override",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "recorded-content",
				}},
			}},
		},
		preserveInputWorkstation(),
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	token := mutations[0].NewToken
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want preserved input payload", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "recorded-content" {
		t.Fatalf("content = %#v, want explicit recorded content", token.Color.Content)
	}
}

func (f calculateMutationsFixture) calculateWithWorkstation(
	arcs []petri.Arc,
	result resolvedWorkResult,
	workstation *interfaces.FactoryWorkstationConfig,
) ([]interfaces.MarkingMutation, error) {
	return calculateMutations(mutationCalculationInput{
		transition:  f.transition,
		workstation: workstation,
		arcs:        arcs,
		consumed:    f.consumed,
		result:      result,
		now:         f.now,
		history:     f.baseHistory,
		inputColors: f.inputColors,
		transformer: f.transformer,
	})
}

func TestCalculateMutations_OutputAsPayloadExplicit_UsesWorkerOutputPayload(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
		&interfaces.FactoryWorkstationConfig{
			Name: "execute-story",
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModeOutputAsPayload,
			},
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if string(mutations[0].NewToken.Color.Payload) != "worker-output" {
		t.Fatalf("payload = %q, want worker-output", mutations[0].NewToken.Color.Payload)
	}
	if len(mutations[0].NewToken.Color.Content) != 1 || mutations[0].NewToken.Color.Content[0].Text != "worker-output" {
		t.Fatalf("content = %#v, want worker response content", mutations[0].NewToken.Color.Content)
	}
}

func TestCalculateMutations_OutputAsPayload_Continue_UsesNextTurnContent(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "continue", PlaceID: "wt-code:init"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeContinue,
			output:       "next-turn-output",
			feedback:     "needs revision",
		},
		&interfaces.FactoryWorkstationConfig{
			Name: "review-story",
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModeOutputAsPayload,
			},
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	token := mutations[0].NewToken
	if string(token.Color.Payload) != "next-turn-output" {
		t.Fatalf("payload = %q, want next-turn-output", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "next-turn-output" {
		t.Fatalf("content = %#v, want next-turn content not submitted request", token.Color.Content)
	}
	if token.Color.Tags["continue_feedback"] != "needs revision" {
		t.Fatalf("continue feedback tag = %#v, want needs revision", token.Color.Tags)
	}
}

func TestCalculateMutations_OutputAsPayload_Failed_PreservesRequestContentAndDiagnostics(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "fail", PlaceID: "wt-code:failed"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeFailed,
			output:       "worker-output",
			err:          "agent crashed",
		},
		&interfaces.FactoryWorkstationConfig{
			Name: "execute-story",
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModeOutputAsPayload,
			},
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	token := mutations[0].NewToken
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want preserved request payload", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want preserved request content not worker output", token.Color.Content)
	}
	if token.History.LastError != "agent crashed" {
		t.Fatalf("LastError = %q, want failure diagnostics", token.History.LastError)
	}
}

func TestCalculateMutations_PreserveInput_WithoutConsumedWorkInput_ReturnsDiagnostic(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed = nil
	fixture.inputColors = nil

	_, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
		&interfaces.FactoryWorkstationConfig{
			Name: "review-story",
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModePreserveInput,
			},
		},
	)
	if err == nil {
		t.Fatal("calculateMutations() error = nil, want preserve-input diagnostic")
	}
	if !strings.Contains(err.Error(), `workstation "review-story" cannot apply work propagation PRESERVE_INPUT`) {
		t.Fatalf("error = %q, want workstation-targeted preserve-input diagnostic", err.Error())
	}
}

func TestCalculateMutations_PreserveInput_OnlyResourceInputs_ReturnsDiagnostic(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed = []factorytoken.Token{{
		ID:      "resource-1",
		PlaceID: "resource:available",
		Color: factorytoken.Color{
			WorkID:     "resource-1",
			WorkTypeID: "resource",
			DataType:   factorytoken.DataTypeResource,
		},
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	_, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "worker-output",
		},
		&interfaces.FactoryWorkstationConfig{
			Name: "review-story",
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModePreserveInput,
			},
		},
	)
	if err == nil {
		t.Fatal("calculateMutations() error = nil, want preserve-input diagnostic")
	}
	if !strings.Contains(err.Error(), "preserve-input requires consumed non-resource input work") {
		t.Fatalf("error = %q, want preserve-input requirement explanation", err.Error())
	}
}

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
		result:      resolvedWorkResult{outcome: workerexecution.OutcomeContinue, output: audioOutput},
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
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != work.WorkContentPartTypeText {
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
		Name:           tts.PackagedInvokeWorkstationName,
		Type:           interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "tts-executor",
		Operation:      "TTS",
	}
	audioOutput := `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "wt-code:done",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: workerexecution.OutcomeContinue, output: audioOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*workerconfig.Config{
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
		result:      resolvedWorkResult{outcome: workerexecution.OutcomeAccepted, output: workerOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*workerconfig.Config{
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
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one text summary part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "Final goal summary." {
		t.Fatalf("terminal content = %q, want worker summary without stop token", token.Color.Content[0].Text)
	}
	if len(token.Color.Payload) != 0 {
		t.Fatalf("terminal payload = %q, want cleared so submitted input does not leak", string(token.Color.Payload))
	}
}

func TestCalculateMutations_PackagedSubagentReplacesTerminalContentWithAgentResponse(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           subagent.PackagedRunWorkstationName,
		Type:           interfaces.WorkstationTypeAgent,
		WorkerTypeName: subagent.PackagedWorkerName,
	}
	workerOutput := "mock worker accepted\nCOMPLETE"

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: subagent.PackagedWorkTypeName + ":complete",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: workerexecution.OutcomeAccepted, output: workerOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*workerconfig.Config{
				subagent.PackagedWorkerName: {
					Name:      subagent.PackagedWorkerName,
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
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one text agent response part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "mock worker accepted" {
		t.Fatalf("terminal content = %q, want worker response without stop token", token.Color.Content[0].Text)
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
	consumed := []factorytoken.Token{{
		ID:        "tok-1",
		PlaceID:   "goal:review",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "w1",
			WorkTypeID: goal.PackagedGoalWorkTypeName,
			Name:       "story-1",
			Content:    summaryContent,
		},
		History: factorytoken.History{
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
		result:      resolvedWorkResult{outcome: workerexecution.OutcomeAccepted, output: "accepted"},
		now:         now,
		history:     consumed[0].History,
		inputColors: tokenColorsFromTokens(consumed),
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*workerconfig.Config{
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
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one carried text summary part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "Final goal summary." {
		t.Fatalf("terminal content = %q, want carried execution summary not classifier label", token.Color.Content[0].Text)
	}
}
