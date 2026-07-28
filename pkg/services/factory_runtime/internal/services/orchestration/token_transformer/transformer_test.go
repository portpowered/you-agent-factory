package token_transformer

import (
	"regexp"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{
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
	if token.Color.ChainingTraceDepth != 1 {
		t.Fatalf("ChainingTraceDepth = %d, want 1 for initial submitted work", token.Color.ChainingTraceDepth)
	}
	if got := token.Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("PreviousChainingTraceIDs = %#v, want [chain-a chain-z]", got)
	}
}

func TestInitialTokenFromSubmit_PreservesCanonicalContentOrdering(t *testing.T) {
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-content",
		Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "caption"},
			{Type: work.WorkContentPartTypeImage, File: "fixtures/diagram.png"},
		},
		Payload: []byte("caption"),
	}, time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	if len(token.Color.Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(token.Color.Content))
	}
	if token.Color.Content[0].Type != work.WorkContentPartTypeText || token.Color.Content[0].Text != "caption" {
		t.Fatalf("first content part = %#v, want ordered text part", token.Color.Content[0])
	}
	if token.Color.Content[1].Type != work.WorkContentPartTypeImage || token.Color.Content[1].File != "fixtures/diagram.png" {
		t.Fatalf("second content part = %#v, want ordered image part", token.Color.Content[1])
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    "trace-1",
		Relations: []work.Relation{{
			Type:         work.RelationParentChild,
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

func TestInitialTokenFromSubmit_DetachesMutableRuntimeFields(t *testing.T) {
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
		petri.NewWorkIDGenerator(),
	)

	req := work.SubmitRequest{
		WorkTypeID: "story",
		TraceID:    "trace-1",
		Tags:       map[string]string{"scope": "alpha"},
		Relations: []work.Relation{{
			Type:          work.RelationDependsOn,
			TargetWorkID:  "work-parent-1",
			RequiredState: "complete",
		}},
		Payload: []byte("draft"),
	}

	token, err := transformer.InitialTokenFromSubmit(req, time.Date(2026, time.May, 24, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit() error = %v", err)
	}

	req.Tags["scope"] = "mutated"
	req.Relations[0].TargetWorkID = "work-mutated"
	req.Payload[0] = 'D'

	if token.Color.Tags["scope"] != "alpha" {
		t.Fatalf("token tags = %#v, want detached original tag value", token.Color.Tags)
	}
	if len(token.Color.Relations) != 1 || token.Color.Relations[0].TargetWorkID != "work-parent-1" {
		t.Fatalf("token relations = %#v, want detached original relation", token.Color.Relations)
	}
	if string(token.Color.Payload) != "draft" {
		t.Fatalf("token payload = %q, want detached original payload", token.Color.Payload)
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
		gen,
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []factorytoken.Color{
			{WorkTypeID: "source-type", WorkID: "work-source-type-1", TraceID: "trace-1", Name: "item-a", ChainingTraceDepth: 3},
		},
		Outcome: workerexecution.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: factorytoken.History{
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
	if token.Color.DataType != factorytoken.DataTypeWork {
		t.Errorf("DataType = %q, want %q", token.Color.DataType, factorytoken.DataTypeWork)
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
	if token.Color.ChainingTraceDepth != 4 {
		t.Errorf("ChainingTraceDepth = %d, want 4", token.Color.ChainingTraceDepth)
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []factorytoken.Color{
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
		Outcome: workerexecution.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: factorytoken.History{
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
		petri.NewWorkIDGenerator(),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-same", Direction: petri.ArcOutput},
		},
		InputColors: []factorytoken.Color{
			{
				WorkTypeID:               "my-type",
				WorkID:                   "work-my-type-42",
				ChainingTraceDepth:       7,
				CurrentChainingTraceID:   "chain-current",
				PreviousChainingTraceIDs: []string{"chain-a", "chain-z"},
				TraceID:                  "trace-1",
				ParentID:                 "parent-1",
			},
		},
		Outcome: workerexecution.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: factorytoken.History{
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
	if token.Color.ChainingTraceDepth != 7 {
		t.Errorf("same-type ChainingTraceDepth = %d, want 7", token.Color.ChainingTraceDepth)
	}
	if got := token.Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Errorf("same-type PreviousChainingTraceIDs = %#v, want [chain-a chain-z]", got)
	}
}

func TestOutputToken_NilGeneratorFailsClosed(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"place-target": {ID: "place-target", TypeID: "target-type"},
		},
		map[string]*state.WorkType{
			"source-type": {ID: "source-type"},
			"target-type": {ID: "target-type"},
		}, nil)

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("OutputToken() with nil Work ID generator did not fail closed")
		}
	}()
	_, _ = transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "place-target", Direction: petri.ArcOutput},
		},
		InputColors: []factorytoken.Color{
			{WorkTypeID: "source-type", WorkID: "work-source-1", TraceID: "trace-1"},
		},
		Outcome: workerexecution.OutcomeAccepted,
		Now:     time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
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
		}, petri.NewWorkIDGenerator())

	consumed := factorytoken.Token{
		ID:        "slot:resource:0",
		PlaceID:   "slot:busy",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "slot:0",
			WorkTypeID: "slot",
			DataType:   factorytoken.DataTypeResource,
			Tags:       map[string]string{"pool": "executor"},
		},
		History: factorytoken.History{
			PlaceVisits: map[string]int{"slot:available": 1},
		},
	}

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "slot:available", Direction: petri.ArcOutput},
		},
		ConsumedTokens: []factorytoken.Token{consumed},
		InputColors:    []factorytoken.Color{consumed.Color},
		Outcome:        workerexecution.OutcomeAccepted,
		Now:            now,
		History: factorytoken.History{
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

	consumed.Color.Tags["pool"] = "mutated"
	consumed.History.PlaceVisits["slot:available"] = 9
	if token.Color.Tags["pool"] != "executor" {
		t.Fatalf("tag pool after source mutation = %q, want detached original", token.Color.Tags["pool"])
	}
	if token.History.PlaceVisits["slot:available"] != 1 {
		t.Fatalf("PlaceVisits after source mutation = %#v, want detached original", token.History.PlaceVisits)
	}
}

func TestOutputToken_Resource_DoesNotInventWorkChainingLineage(t *testing.T) {
	now := time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC)
	transformer := New(
		map[string]*petri.Place{
			"slot:available": {ID: "slot:available", TypeID: "slot", State: "available"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		}, petri.NewWorkIDGenerator())

	resource := factorytoken.Token{
		ID:      "slot:resource:0",
		PlaceID: "slot:busy",
		Color: factorytoken.Color{
			WorkID:     "slot:0",
			WorkTypeID: "slot",
			DataType:   factorytoken.DataTypeResource,
		},
	}
	work := factorytoken.Token{
		ID:      "work-task-1",
		PlaceID: "task:busy",
		Color: factorytoken.Color{
			WorkID:                   "work-task-1",
			WorkTypeID:               "task",
			DataType:                 factorytoken.DataTypeWork,
			CurrentChainingTraceID:   "chain-task",
			PreviousChainingTraceIDs: []string{"chain-root"},
			TraceID:                  "trace-task",
		},
	}

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "slot:available", Direction: petri.ArcOutput},
		},
		ConsumedTokens: []factorytoken.Token{resource, work},
		InputColors:    []factorytoken.Color{resource.Color, work.Color},
		Outcome:        workerexecution.OutcomeAccepted,
		Now:            now,
		History: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}

	if token.Color.CurrentChainingTraceID != "" {
		t.Fatalf("CurrentChainingTraceID = %q, want empty for resource output", token.Color.CurrentChainingTraceID)
	}
	if len(token.Color.PreviousChainingTraceIDs) != 0 {
		t.Fatalf("PreviousChainingTraceIDs = %#v, want empty for resource output", token.Color.PreviousChainingTraceIDs)
	}
	if token.Color.TraceID != "" {
		t.Fatalf("TraceID = %q, want empty for resource output", token.Color.TraceID)
	}
}

type mixedWorkResourceOutputFixture struct {
	transformer *Transformer
	resource    factorytoken.Token
	work        factorytoken.Token
	arcs        []petri.Arc
	baseHistory factorytoken.History
	now         time.Time
}

func newMixedWorkResourceTraceLineageFixture() mixedWorkResourceOutputFixture {
	now := time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	return mixedWorkResourceOutputFixture{
		transformer: New(
			map[string]*petri.Place{
				"story:complete":       {ID: "story:complete", TypeID: "story", State: "complete"},
				"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
			},
			map[string]*state.WorkType{
				"story": {ID: "story"},
			},
			petri.NewWorkIDGenerator(),
		),
		resource: factorytoken.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
			},
		},
		work: factorytoken.Token{
			ID:        "work-story-1",
			PlaceID:   "story:in-review",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:                   "work-story-1",
				WorkTypeID:               "story",
				TraceID:                  "trace-batch-idea-001",
				CurrentChainingTraceID:   "trace-batch-idea-001",
				PreviousChainingTraceIDs: []string{"trace-batch-idea-001"},
				ChainingTraceDepth:       3,
			},
		},
		arcs: []petri.Arc{
			{ID: "work-out", PlaceID: "story:complete", Direction: petri.ArcOutput},
			{ID: "slot-out", PlaceID: "agent-slot:available", Direction: petri.ArcOutput},
		},
		baseHistory: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
		now: now,
	}
}

func assertMixedWorkOutputPreservesTraceLineage(t *testing.T, workToken *factorytoken.Token) {
	t.Helper()
	if workToken.Color.DataType != factorytoken.DataTypeWork {
		t.Fatalf("work DataType = %q, want %q", workToken.Color.DataType, factorytoken.DataTypeWork)
	}
	if workToken.Color.TraceID != "trace-batch-idea-001" {
		t.Fatalf("work TraceID = %q, want trace-batch-idea-001", workToken.Color.TraceID)
	}
	if workToken.Color.CurrentChainingTraceID != "trace-batch-idea-001" {
		t.Fatalf("work CurrentChainingTraceID = %q, want trace-batch-idea-001", workToken.Color.CurrentChainingTraceID)
	}
	if workToken.Color.WorkID != "work-story-1" {
		t.Fatalf("work WorkID = %q, want work-story-1", workToken.Color.WorkID)
	}
}

func assertMixedResourceOutputPreservesIdentity(t *testing.T, resourceToken *factorytoken.Token, consumed factorytoken.Token) {
	t.Helper()
	if resourceToken.ID != consumed.ID {
		t.Fatalf("resource ID = %q, want %q", resourceToken.ID, consumed.ID)
	}
	if resourceToken.Color.DataType != factorytoken.DataTypeResource {
		t.Fatalf("resource DataType = %q, want %q", resourceToken.Color.DataType, factorytoken.DataTypeResource)
	}
	if resourceToken.Color.WorkID != "agent-slot:0" {
		t.Fatalf("resource WorkID = %q, want agent-slot:0", resourceToken.Color.WorkID)
	}
	if resourceToken.Color.TraceID != "" || resourceToken.Color.CurrentChainingTraceID != "" {
		t.Fatalf("resource trace fields = (%q,%q), want empty", resourceToken.Color.TraceID, resourceToken.Color.CurrentChainingTraceID)
	}
}

func TestOutputToken_MixedWorkResource_PreservesWorkTraceLineageRegardlessOfInputOrder(t *testing.T) {
	fixture := newMixedWorkResourceTraceLineageFixture()
	orderings := []struct {
		name          string
		consumed      []factorytoken.Token
		inputColors   []factorytoken.Color
		resourceIndex int
	}{
		{
			name:          "resource-first",
			consumed:      []factorytoken.Token{fixture.resource, fixture.work},
			inputColors:   []factorytoken.Color{fixture.resource.Color, fixture.work.Color},
			resourceIndex: 0,
		},
		{
			name:          "work-first",
			consumed:      []factorytoken.Token{fixture.work, fixture.resource},
			inputColors:   []factorytoken.Color{fixture.work.Color, fixture.resource.Color},
			resourceIndex: 0,
		},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			workToken, err := fixture.transformer.OutputToken(OutputTokenInput{
				ArcIndex:       0,
				Arcs:           fixture.arcs,
				ConsumedTokens: ordering.consumed,
				InputColors:    ordering.inputColors,
				Outcome:        workerexecution.OutcomeAccepted,
				Now:            fixture.now,
				History:        fixture.baseHistory,
			})
			if err != nil {
				t.Fatalf("work OutputToken() error = %v", err)
			}
			assertMixedWorkOutputPreservesTraceLineage(t, workToken)

			resourceToken, err := fixture.transformer.OutputToken(OutputTokenInput{
				ArcIndex:           1,
				Arcs:               fixture.arcs,
				ConsumedTokens:     ordering.consumed,
				InputColors:        ordering.inputColors,
				ResourceTokenIndex: ordering.resourceIndex,
				Outcome:            workerexecution.OutcomeAccepted,
				Now:                fixture.now,
				History:            fixture.baseHistory,
			})
			if err != nil {
				t.Fatalf("resource OutputToken() error = %v", err)
			}
			assertMixedResourceOutputPreservesIdentity(t, resourceToken, fixture.resource)
		})
	}
}

func TestOutputToken_CrossTypeWithResource_PreservesWorkTraceRegardlessOfInputOrder(t *testing.T) {
	now := time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC)
	transformer := New(
		map[string]*petri.Place{
			"prd:init": {ID: "prd:init", TypeID: "prd", State: "init"},
		},
		map[string]*state.WorkType{
			"idea": {ID: "idea"},
			"prd":  {ID: "prd"},
		},
		petri.NewWorkIDGenerator(),
	)
	resource := factorytoken.Color{
		WorkID:     "agent-slot:0",
		WorkTypeID: "agent-slot",
		DataType:   factorytoken.DataTypeResource,
	}
	work := factorytoken.Color{
		WorkID:     "work-idea-1",
		WorkTypeID: "idea",
		TraceID:    "trace-batch-idea-002",
	}
	orderings := [][]factorytoken.Color{
		{resource, work},
		{work, resource},
	}

	for i, inputColors := range orderings {
		token, err := transformer.OutputToken(OutputTokenInput{
			ArcIndex:    0,
			Arcs:        []petri.Arc{{PlaceID: "prd:init", Direction: petri.ArcOutput}},
			InputColors: inputColors,
			Outcome:     workerexecution.OutcomeAccepted,
			Now:         now,
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		})
		if err != nil {
			t.Fatalf("ordering %d OutputToken() error = %v", i, err)
		}
		if token.Color.DataType != factorytoken.DataTypeWork {
			t.Fatalf("ordering %d DataType = %q, want work", i, token.Color.DataType)
		}
		if token.Color.TraceID != "trace-batch-idea-002" {
			t.Fatalf("ordering %d TraceID = %q, want trace-batch-idea-002", i, token.Color.TraceID)
		}
		if token.Color.WorkTypeID != "prd" {
			t.Fatalf("ordering %d WorkTypeID = %q, want prd", i, token.Color.WorkTypeID)
		}
		if token.Color.ParentID != "work-idea-1" {
			t.Fatalf("ordering %d ParentID = %q, want work-idea-1", i, token.Color.ParentID)
		}
	}
}

func TestReleasedResourceToken_PreservesConsumedTokenIdentity(t *testing.T) {
	now := time.Date(2026, time.April, 7, 13, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	transformer := New(nil, nil, petri.NewWorkIDGenerator())
	consumed := factorytoken.Token{
		ID:        "executor-slot:resource:1",
		PlaceID:   "executor-slot:available",
		CreatedAt: createdAt,
		EnteredAt: createdAt.Add(15 * time.Minute),
		Color: factorytoken.Color{
			WorkID:     "executor-slot:1",
			WorkTypeID: "executor-slot",
			DataType:   factorytoken.DataTypeResource,
		},
		History: factorytoken.History{
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

	consumed.History.PlaceVisits["executor-slot:available"] = 7
	if released.History.PlaceVisits["executor-slot:available"] != 2 {
		t.Fatalf("PlaceVisits after source mutation = %#v, want detached original", released.History.PlaceVisits)
	}
}

func newMixedWorkResourceReleaseFixture() mixedWorkResourceOutputFixture {
	now := time.Date(2026, time.April, 7, 14, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	return mixedWorkResourceOutputFixture{
		transformer: New(
			map[string]*petri.Place{
				"story:complete":       {ID: "story:complete", TypeID: "story", State: "complete"},
				"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
			},
			map[string]*state.WorkType{
				"story": {ID: "story"},
			},
			petri.NewWorkIDGenerator(),
		),
		resource: factorytoken.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
			},
			History: factorytoken.History{
				PlaceVisits: map[string]int{"agent-slot:available": 3},
			},
		},
		work: factorytoken.Token{
			ID:        "work-story-1",
			PlaceID:   "story:in-review",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "work-story-1",
				WorkTypeID: "story",
				DataType:   factorytoken.DataTypeWork,
				TraceID:    "trace-batch-idea-001",
			},
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
		arcs: []petri.Arc{
			{ID: "work-out", PlaceID: "story:complete", Direction: petri.ArcOutput},
			{ID: "slot-out", PlaceID: "agent-slot:available", Direction: petri.ArcOutput},
		},
		baseHistory: factorytoken.History{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
		now: now,
	}
}

func assertMixedResourceOutputPreservesConsumedHistory(t *testing.T, resourceToken *factorytoken.Token, consumed factorytoken.Token, now time.Time) {
	t.Helper()
	assertMixedResourceOutputPreservesIdentity(t, resourceToken, consumed)
	if resourceToken.PlaceID != "agent-slot:available" {
		t.Fatalf("resource PlaceID = %q, want agent-slot:available", resourceToken.PlaceID)
	}
	if !resourceToken.CreatedAt.Equal(consumed.CreatedAt) {
		t.Fatalf("resource CreatedAt = %v, want %v", resourceToken.CreatedAt, consumed.CreatedAt)
	}
	if !resourceToken.EnteredAt.Equal(now) {
		t.Fatalf("resource EnteredAt = %v, want %v", resourceToken.EnteredAt, now)
	}
	if resourceToken.History.PlaceVisits["agent-slot:available"] != 3 {
		t.Fatalf("resource PlaceVisits = %#v, want preserved history", resourceToken.History.PlaceVisits)
	}
}

func TestOutputToken_MixedWorkResource_ReleasesConsumedResourceIdentityRegardlessOfInputOrder(t *testing.T) {
	fixture := newMixedWorkResourceReleaseFixture()
	orderings := []struct {
		name     string
		consumed []factorytoken.Token
	}{
		{name: "resource-first", consumed: []factorytoken.Token{fixture.resource, fixture.work}},
		{name: "work-first", consumed: []factorytoken.Token{fixture.work, fixture.resource}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			resourceToken, err := fixture.transformer.OutputToken(OutputTokenInput{
				ArcIndex:           1,
				Arcs:               fixture.arcs,
				ConsumedTokens:     ordering.consumed,
				InputColors:        tokenColorsFromTokens(ordering.consumed),
				ResourceTokenIndex: 0,
				Outcome:            workerexecution.OutcomeAccepted,
				Now:                fixture.now,
				History:            fixture.baseHistory,
			})
			if err != nil {
				t.Fatalf("resource OutputToken() error = %v", err)
			}
			assertMixedResourceOutputPreservesConsumedHistory(t, resourceToken, fixture.resource, fixture.now)
		})
	}
}

func tokenColorsFromTokens(tokens []factorytoken.Token) []factorytoken.Color {
	colors := make([]factorytoken.Color, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}
