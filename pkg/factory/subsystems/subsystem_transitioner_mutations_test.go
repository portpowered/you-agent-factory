package subsystems

import (
	"bytes"
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
