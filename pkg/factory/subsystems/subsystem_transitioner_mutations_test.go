package subsystems

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type calculateMutationsFixture struct {
	now         time.Time
	baseHistory interfaces.TokenHistory
	transition  *petri.Transition
	consumed    []interfaces.Token
	inputColors []interfaces.TokenColor
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
	consumed := []interfaces.Token{{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "w1",
			WorkTypeID: "wt-code",
			Name:       "story-1",
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}}
	return calculateMutationsFixture{
		now: now,
		baseHistory: interfaces.TokenHistory{
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
	fixture.consumed[0].Color.Content = []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.consumed[0].Color.Tags = map[string]string{"objective": "goal-1"}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
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
			outcome:      interfaces.OutcomeAccepted,
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
	fixture.consumed[0].Color.Content = []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
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
				outcome:      interfaces.OutcomeContinue,
				output:       "worker-output",
				feedback:     "needs revision",
			},
		},
		{
			name: "Rejected",
			arcs: []petri.Arc{{ID: "reject", PlaceID: "wt-code:init"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      interfaces.OutcomeRejected,
				output:       "worker-output",
				feedback:     "rejected",
			},
		},
		{
			name: "Failed",
			arcs: []petri.Arc{{ID: "fail", PlaceID: "wt-code:failed"}},
			result: resolvedWorkResult{
				transitionID: "t1",
				outcome:      interfaces.OutcomeFailed,
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
	fixture.consumed = []interfaces.Token{
		{
			ID:      "tok-primary",
			PlaceID: "wt-objective:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-objective",
				WorkTypeID: "wt-objective",
				Payload:    []byte("primary-input-payload"),
				Tags:       map[string]string{"role": "primary"},
			},
			CreatedAt: fixture.now.Add(-2 * time.Hour),
			EnteredAt: fixture.now.Add(-2 * time.Hour),
			History: interfaces.TokenHistory{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
		{
			ID:      "tok-secondary",
			PlaceID: "wt-context:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-context",
				WorkTypeID: "wt-context",
				Payload:    []byte("secondary-input-payload"),
				Tags:       map[string]string{"role": "secondary"},
			},
			CreatedAt: fixture.now.Add(-time.Hour),
			EnteredAt: fixture.now.Add(-time.Hour),
			History: interfaces.TokenHistory{
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
			outcome:      interfaces.OutcomeAccepted,
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
	consumed := []interfaces.Token{{
		ID:      "tok-1",
		PlaceID: "wt-code:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-code-1",
			WorkTypeID: "wt-code",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
			Tags: map[string]string{"objective": "goal-1"},
		},
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		History: interfaces.TokenHistory{
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
		result:      resolvedWorkResult{transitionID: "t1", outcome: interfaces.OutcomeAccepted, output: "worker-output"},
		now:         now,
		history:     interfaces.TokenHistory{},
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
	fixture.consumed[0].Color.Content = []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "cross", PlaceID: "wt-review:init"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
			output:       "worker-output",
			recordedOutputWork: []interfaces.FactoryWorkItem{{
				ID:          "work-review-99",
				WorkTypeID:  "wt-review",
				DisplayName: "review-override",
				Content: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
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
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
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
}

func TestCalculateMutations_PreserveInput_WithoutConsumedWorkInput_ReturnsDiagnostic(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed = nil
	fixture.inputColors = nil

	_, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
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
	fixture.consumed = []interfaces.Token{{
		ID:      "resource-1",
		PlaceID: "resource:available",
		Color: interfaces.TokenColor{
			WorkID:     "resource-1",
			WorkTypeID: "resource",
			DataType:   interfaces.DataTypeResource,
		},
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	_, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
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
