package token_transformer

import (
	"regexp"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestInitialTokenFromSubmit_UsesInitialPlaceAndWorkIDGenerator(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
				},
			},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.InitialTokenFromSubmit(interfaces.SubmitRequest{
		RequestID:  "request-1",
		WorkTypeID: "task",
		Name:       "story-1",
		TraceID:    "trace-1",
	}, time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	if token.ID != "tok-task-1" {
		t.Fatalf("ID = %q, want %q", token.ID, "tok-task-1")
	}
	if token.PlaceID != "task:init" {
		t.Fatalf("PlaceID = %q, want %q", token.PlaceID, "task:init")
	}
	if token.Color.WorkID != "work-task-1" {
		t.Fatalf("WorkID = %q, want %q", token.Color.WorkID, "work-task-1")
	}
	if token.Color.RequestID != "request-1" {
		t.Fatalf("RequestID = %q, want %q", token.Color.RequestID, "request-1")
	}
	if token.Color.Name != "story-1" {
		t.Fatalf("Name = %q, want %q", token.Color.Name, "story-1")
	}
}

func TestInitialTokenFromSubmit_PreservesExplicitChainingLineage(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
				},
			},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.InitialTokenFromSubmit(interfaces.SubmitRequest{
		WorkTypeID:               "task",
		CurrentChainingTraceID:   "chain-current",
		PreviousChainingTraceIDs: []string{"chain-z", "chain-a", "chain-z"},
		TraceID:                  "trace-current",
	}, time.Date(2026, time.April, 22, 19, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	if token.Color.CurrentChainingTraceID != "chain-current" {
		t.Fatalf("CurrentChainingTraceID = %q, want chain-current", token.Color.CurrentChainingTraceID)
	}
	if got := token.Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("PreviousChainingTraceIDs = %#v, want [chain-a chain-z]", got)
	}
}

func TestInitialTokenFromSubmit_TargetStateUsesConfiguredPlace(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:init":  {ID: "task:init", TypeID: "task", State: "init"},
			"task:ready": {ID: "task:ready", TypeID: "task", State: "ready"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "ready", Category: state.StateCategoryProcessing},
				},
			},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.InitialTokenFromSubmit(interfaces.SubmitRequest{
		WorkTypeID:  "task",
		TargetState: "ready",
		TraceID:     "trace-1",
	}, time.Date(2026, time.April, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	if token.PlaceID != "task:ready" {
		t.Fatalf("PlaceID = %q, want %q", token.PlaceID, "task:ready")
	}
	if token.Color.WorkID != "work-task-1" {
		t.Fatalf("WorkID = %q, want %q", token.Color.WorkID, "work-task-1")
	}
}

func TestInitialTokenFromSubmit_ParentChildRelationSetsParentID(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"story:init": {ID: "story:init", TypeID: "story", State: "init"},
		},
		map[string]*state.WorkType{
			"story": {
				ID: "story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
				},
			},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.InitialTokenFromSubmit(interfaces.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    "trace-1",
		Relations: []interfaces.Relation{{
			Type:         interfaces.RelationParentChild,
			TargetWorkID: "work-parent-1",
		}},
	}, time.Date(2026, time.April, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	if token.Color.ParentID != "work-parent-1" {
		t.Fatalf("ParentID = %q, want %q", token.Color.ParentID, "work-parent-1")
	}
}

func TestOutputToken_CrossType_UsesWorkIDGenerator(t *testing.T) {
	gen := petri.NewWorkIDGenerator()
	pattern := regexp.MustCompile(`^work-target-type-\d+$`)

	transformer := New(
		map[string]*petri.Place{
			"place-target": {ID: "place-target", TypeID: "target-type"},
		},
		map[string]*state.WorkType{
			"source-type": {ID: "source-type"},
			"target-type": {ID: "target-type"},
		},
		WithWorkIDGenerator(gen),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{
			{WorkTypeID: "source-type", WorkID: "work-source-type-1", TraceID: "trace-1", Name: "item-a"},
		},
		Outcome: interfaces.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if !pattern.MatchString(token.Color.WorkID) {
		t.Errorf("cross-type WorkID = %q, want pattern work-target-type-{N}", token.Color.WorkID)
	}
	if token.Color.WorkTypeID != "target-type" {
		t.Errorf("WorkTypeID = %q, want %q", token.Color.WorkTypeID, "target-type")
	}
	if token.Color.ParentID != "work-source-type-1" {
		t.Errorf("ParentID = %q, want %q", token.Color.ParentID, "work-source-type-1")
	}
	if token.Color.TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want %q", token.Color.TraceID, "trace-1")
	}
	if token.Color.CurrentChainingTraceID != "trace-1" {
		t.Errorf("CurrentChainingTraceID = %q, want %q", token.Color.CurrentChainingTraceID, "trace-1")
	}
	if got := token.Color.PreviousChainingTraceIDs; len(got) != 1 || got[0] != "trace-1" {
		t.Errorf("PreviousChainingTraceIDs = %#v, want [trace-1]", got)
	}
}

func TestOutputToken_CrossType_PrefersCustomerInputOverCronTimeToken(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"place-target": {ID: "place-target", TypeID: "target-type"},
		},
		map[string]*state.WorkType{
			"signal":      {ID: "signal"},
			"target-type": {ID: "target-type"},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{
			{
				WorkTypeID: interfaces.SystemTimeWorkTypeID,
				WorkID:     "time-work",
				RequestID:  "request-time",
				TraceID:    "trace-time",
				Name:       "cron:poll",
			},
			{
				WorkTypeID: "signal",
				WorkID:     "signal-work",
				RequestID:  "request-signal",
				TraceID:    "trace-signal",
				Name:       "signal",
			},
		},
		Outcome: interfaces.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if token.Color.ParentID != "signal-work" {
		t.Errorf("ParentID = %q, want %q", token.Color.ParentID, "signal-work")
	}
	if token.Color.RequestID != "request-signal" {
		t.Errorf("RequestID = %q, want %q", token.Color.RequestID, "request-signal")
	}
	if token.Color.TraceID != "trace-signal" {
		t.Errorf("TraceID = %q, want %q", token.Color.TraceID, "trace-signal")
	}
	if token.Color.Name != "signal" {
		t.Errorf("Name = %q, want %q", token.Color.Name, "signal")
	}
}

func TestOutputToken_SameType_PreservesWorkID(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"place-same": {ID: "place-same", TypeID: "my-type"},
		},
		map[string]*state.WorkType{
			"my-type": {ID: "my-type"},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-same", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{
			{
				WorkTypeID:               "my-type",
				WorkID:                   "work-my-type-42",
				CurrentChainingTraceID:   "chain-current",
				PreviousChainingTraceIDs: []string{"chain-a", "chain-z"},
				TraceID:                  "trace-1",
				ParentID:                 "parent-1",
			},
		},
		Outcome: interfaces.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if token.Color.WorkID != "work-my-type-42" {
		t.Errorf("same-type WorkID = %q, want %q", token.Color.WorkID, "work-my-type-42")
	}
	if token.Color.ParentID != "parent-1" {
		t.Errorf("same-type ParentID = %q, want %q", token.Color.ParentID, "parent-1")
	}
	if token.Color.CurrentChainingTraceID != "chain-current" {
		t.Errorf("same-type CurrentChainingTraceID = %q, want %q", token.Color.CurrentChainingTraceID, "chain-current")
	}
	if got := token.Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Errorf("same-type PreviousChainingTraceIDs = %#v, want [chain-a chain-z]", got)
	}
}

func TestOutputToken_NilGenerator_FallsBackToUUID(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"place-target": {ID: "place-target", TypeID: "target-type"},
		},
		map[string]*state.WorkType{
			"source-type": {ID: "source-type"},
			"target-type": {ID: "target-type"},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{
			{WorkTypeID: "source-type", WorkID: "work-source-1", TraceID: "trace-1"},
		},
		Outcome: interfaces.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if token.Color.WorkID == "" {
		t.Fatal("WorkID should not be empty even without generator")
	}
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(token.Color.WorkID) {
		t.Errorf("without generator, WorkID = %q, want UUID format", token.Color.WorkID)
	}
}

func TestOutputToken_Resource_PreservesConsumedTokenIdentity(t *testing.T) {
	now := time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	transformer := New(
		map[string]*petri.Place{
			"slot:available": {ID: "slot:available", TypeID: "slot", State: "available"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)
	consumed := interfaces.Token{
		ID:        "slot:resource:0",
		PlaceID:   "slot:busy",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "slot:0",
			WorkTypeID: "slot",
			DataType:   interfaces.DataTypeResource,
			Tags:       map[string]string{"pool": "executor"},
		},
		History: interfaces.TokenHistory{
			PlaceVisits: map[string]int{"slot:available": 1},
		},
	}

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "slot:available", Direction: petri.ArcOutput},
		},
		ConsumedTokens: []interfaces.Token{consumed},
		InputColors:    []interfaces.TokenColor{consumed.Color},
		Outcome:        interfaces.OutcomeAccepted,
		Now:            now,
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if token.ID != consumed.ID {
		t.Fatalf("ID = %q, want %q", token.ID, consumed.ID)
	}
	if token.Color.WorkID != consumed.Color.WorkID {
		t.Fatalf("WorkID = %q, want %q", token.Color.WorkID, consumed.Color.WorkID)
	}
	if token.PlaceID != "slot:available" {
		t.Fatalf("PlaceID = %q, want %q", token.PlaceID, "slot:available")
	}
	if !token.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", token.CreatedAt, createdAt)
	}
	if !token.EnteredAt.Equal(now) {
		t.Fatalf("EnteredAt = %v, want %v", token.EnteredAt, now)
	}
	if token.Color.Tags["pool"] != "executor" {
		t.Fatalf("tag pool = %q, want %q", token.Color.Tags["pool"], "executor")
	}
	if token.History.PlaceVisits["slot:available"] != 1 {
		t.Fatalf("PlaceVisits = %#v, want original history", token.History.PlaceVisits)
	}
}

func TestReleasedResourceToken_PreservesConsumedTokenIdentity(t *testing.T) {
	now := time.Date(2026, time.April, 7, 13, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	transformer := New(nil, nil)
	consumed := interfaces.Token{
		ID:        "executor-slot:resource:1",
		PlaceID:   "executor-slot:available",
		CreatedAt: createdAt,
		EnteredAt: createdAt.Add(15 * time.Minute),
		Color: interfaces.TokenColor{
			WorkID:     "executor-slot:1",
			WorkTypeID: "executor-slot",
			DataType:   interfaces.DataTypeResource,
		},
		History: interfaces.TokenHistory{
			PlaceVisits: map[string]int{"executor-slot:available": 2},
		},
	}

	released := transformer.ReleasedResourceToken(consumed, "executor-slot:available", now)
	if released.ID != consumed.ID {
		t.Fatalf("ID = %q, want %q", released.ID, consumed.ID)
	}
	if released.Color.WorkID != consumed.Color.WorkID {
		t.Fatalf("WorkID = %q, want %q", released.Color.WorkID, consumed.Color.WorkID)
	}
	if !released.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", released.CreatedAt, createdAt)
	}
	if !released.EnteredAt.Equal(now) {
		t.Fatalf("EnteredAt = %v, want %v", released.EnteredAt, now)
	}
	if released.History.PlaceVisits["executor-slot:available"] != 2 {
		t.Fatalf("PlaceVisits = %#v, want preserved history", released.History.PlaceVisits)
	}
}
