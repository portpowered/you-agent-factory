package token_transformer

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

func TestOutputToken_PreserveInput_SameType_KeepsConsumedPayload(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:done": {ID: "task:done", TypeID: "task"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:done", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want preserved input content", token.Color.Content)
	}
	if token.Color.Tags["_last_output"] != "worker-output" {
		t.Fatalf("last output = %#v, want worker-output", token.Color.Tags)
	}
}

func TestOutputToken_PreserveInput_CrossType_CopiesConsumedPayload(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"review:init": {ID: "review:init", TypeID: "review"},
		},
		map[string]*state.WorkType{
			"task":   {ID: "task"},
			"review": {ID: "review"},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "review:init", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-task-1",
			Payload:    []byte("input-payload"),
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload", token.Color.Payload)
	}
}

func TestOutputToken_OutputAsPayload_UsesWorkerOutput(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:done": {ID: "task:done", TypeID: "task"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:done", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "worker-output" {
		t.Fatalf("payload = %q, want worker-output", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "worker-output" {
		t.Fatalf("content = %#v, want response content not submitted request", token.Color.Content)
	}
}

func TestOutputToken_OutputAsPayload_Continue_UsesNextTurnContent(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:init", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "next-turn-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeContinue,
		Feedback:            "needs revision",
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
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

func TestOutputToken_OutputAsPayload_RejectedLoopback_UsesNextTurnContent(t *testing.T) {
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
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:init", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "next-turn-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeRejected,
		Feedback:            "try again",
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "next-turn-output" {
		t.Fatalf("content = %#v, want next-turn content for loopback rejection", token.Color.Content)
	}
}

func TestOutputToken_OutputAsPayload_RejectedFailurePlace_KeepsRequestContent(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:failed": {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:failed", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeRejected,
		Feedback:            "rejected",
		TransitionID:        "t1",
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want request content preserved on failure rejection", token.Color.Content)
	}
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload preserved on failure rejection", token.Color.Payload)
	}
}

func TestOutputToken_OutputAsPayload_Failed_KeepsRequestContentAndDiagnostics(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:failed": {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:failed", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeFailed,
		Error:               "agent crashed",
		TransitionID:        "t1",
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload preserved on failure", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want request content preserved on failure", token.Color.Content)
	}
	if token.History.LastError != "agent crashed" {
		t.Fatalf("LastError = %q, want failure diagnostics", token.History.LastError)
	}
	if len(token.History.FailureLog) != 1 || token.History.FailureLog[0].Error != "agent crashed" {
		t.Fatalf("FailureLog = %#v, want failure record", token.History.FailureLog)
	}
}

func TestOutputToken_PreserveInput_KeepsConsumedTags(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"review:init": {ID: "review:init", TypeID: "review"},
		},
		map[string]*state.WorkType{
			"task":   {ID: "task"},
			"review": {ID: "review"},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "review:init", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-task-1",
			Payload:    []byte("input-payload"),
			Tags:       map[string]string{"objective": "goal-1", "lane": "review"},
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if token.Color.Tags["objective"] != "goal-1" || token.Color.Tags["lane"] != "review" {
		t.Fatalf("tags = %#v, want preserved input tags", token.Color.Tags)
	}
}

func TestOutputToken_PreserveInput_OutcomeLanes_KeepConsumedPayload(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:init":   {ID: "task:init", TypeID: "task", State: "init"},
			"task:failed": {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	)

	tests := []struct {
		name     string
		placeID  string
		outcome  interfaces.WorkOutcome
		feedback string
		errText  string
	}{
		{name: "Continue", placeID: "task:init", outcome: interfaces.OutcomeContinue, feedback: "needs revision"},
		{name: "Rejected", placeID: "task:init", outcome: interfaces.OutcomeRejected, feedback: "rejected"},
		{name: "Failed", placeID: "task:failed", outcome: interfaces.OutcomeFailed, errText: "agent crashed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := transformer.OutputToken(OutputTokenInput{
				ArcIndex: 0,
				Arcs: []petri.Arc{
					{PlaceID: tt.placeID, Direction: petri.ArcOutput},
				},
				InputColors: []interfaces.TokenColor{{
					WorkTypeID: "task",
					WorkID:     "work-1",
					Payload:    []byte("input-payload"),
				}},
				Output:              "worker-output",
				WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
				Outcome:             tt.outcome,
				Feedback:            tt.feedback,
				Error:               tt.errText,
				TransitionID:        "t1",
				Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
				History: interfaces.TokenHistory{
					TotalVisits:         map[string]int{},
					ConsecutiveFailures: map[string]int{},
					PlaceVisits:         map[string]int{},
				},
			})
			if err != nil {
				t.Fatalf("OutputToken() error = %v", err)
			}
			if string(token.Color.Payload) != "input-payload" {
				t.Fatalf("payload = %q, want input-payload", token.Color.Payload)
			}
		})
	}
}

func TestOutputToken_PreserveInput_MultiInput_UsesPrimaryNonResourceInput(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"review:init": {ID: "review:init", TypeID: "review"},
		},
		map[string]*state.WorkType{
			"objective": {ID: "objective"},
			"context":   {ID: "context"},
			"review":    {ID: "review"},
		},
		WithWorkIDGenerator(petri.NewWorkIDGenerator()),
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "review:init", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{
			{
				WorkTypeID: "objective",
				WorkID:     "work-objective",
				Payload:    []byte("primary-input-payload"),
				Tags:       map[string]string{"role": "primary"},
			},
			{
				WorkTypeID: "context",
				WorkID:     "work-context",
				Payload:    []byte("secondary-input-payload"),
				Tags:       map[string]string{"role": "secondary"},
			},
		},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "primary-input-payload" {
		t.Fatalf("payload = %q, want primary-input-payload", token.Color.Payload)
	}
	if token.Color.Tags["role"] != "primary" {
		t.Fatalf("tags = %#v, want primary input tags", token.Color.Tags)
	}
}

func TestOutputToken_PreserveInput_MultiOutput_EachLaneKeepsConsumedWorkData(t *testing.T) {
	now := time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC)
	places := map[string]*petri.Place{
		"review-a:init": {ID: "review-a:init", TypeID: "review-a", State: "init"},
		"review-b:init": {ID: "review-b:init", TypeID: "review-b", State: "init"},
		"review-c:init": {ID: "review-c:init", TypeID: "review-c", State: "init"},
	}
	workTypes := map[string]*state.WorkType{
		"task":     {ID: "task"},
		"review-a": {ID: "review-a"},
		"review-b": {ID: "review-b"},
		"review-c": {ID: "review-c"},
	}
	transformer := New(places, workTypes, WithWorkIDGenerator(petri.NewWorkIDGenerator()))
	inputColors := []interfaces.TokenColor{{
		WorkTypeID: "task",
		WorkID:     "work-task-1",
		Payload:    []byte("input-payload"),
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "input-content",
		}},
		Tags: map[string]string{"objective": "goal-1"},
	}}
	arcs := []petri.Arc{
		{PlaceID: "review-a:init", Direction: petri.ArcOutput},
		{PlaceID: "review-b:init", Direction: petri.ArcOutput},
		{PlaceID: "review-c:init", Direction: petri.ArcOutput},
	}

	for arcIdx := range arcs {
		token, err := transformer.OutputToken(OutputTokenInput{
			ArcIndex:            arcIdx,
			Arcs:                arcs,
			InputColors:         inputColors,
			Output:              "worker-output",
			WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
			Outcome:             interfaces.OutcomeAccepted,
			Now:                 now,
			History: interfaces.TokenHistory{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		})
		if err != nil {
			t.Fatalf("OutputToken(arc %d) error = %v", arcIdx, err)
		}
		if string(token.Color.Payload) != "input-payload" {
			t.Fatalf("arc %d payload = %q, want input-payload", arcIdx, token.Color.Payload)
		}
		if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "input-content" {
			t.Fatalf("arc %d content = %#v, want preserved input content", arcIdx, token.Color.Content)
		}
		if token.Color.Tags["objective"] != "goal-1" {
			t.Fatalf("arc %d tags = %#v, want preserved input tags", arcIdx, token.Color.Tags)
		}
	}
}

func TestOutputToken_PreserveInput_WithoutConsumedWorkInput_ReturnsDiagnostic(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:done": {ID: "task:done", TypeID: "task"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)

	_, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:done", Direction: petri.ArcOutput},
		},
		InputColors:         nil,
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		WorkstationName:     "review-story",
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err == nil {
		t.Fatal("OutputToken() error = nil, want preserve-input diagnostic")
	}
	var preserveErr *PreserveInputApplicationError
	if !errors.As(err, &preserveErr) {
		t.Fatalf("OutputToken() error = %v, want *PreserveInputApplicationError", err)
	}
	if preserveErr.WorkstationName != "review-story" {
		t.Fatalf("workstation name = %q, want review-story", preserveErr.WorkstationName)
	}
	if !strings.Contains(err.Error(), "preserve-input requires consumed non-resource input work") {
		t.Fatalf("error = %q, want preserve-input requirement explanation", err.Error())
	}
}

func TestOutputToken_PreserveInput_OnlyResourceInputs_ReturnsDiagnostic(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:done": {ID: "task:done", TypeID: "task"},
		},
		map[string]*state.WorkType{
			"task":     {ID: "task"},
			"resource": {ID: "resource"},
		},
	)

	_, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:done", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "resource",
			WorkID:     "resource-1",
			DataType:   interfaces.DataTypeResource,
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModePreserveInput,
		WorkstationName:     "review-story",
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err == nil {
		t.Fatal("OutputToken() error = nil, want preserve-input diagnostic")
	}
	if !strings.Contains(err.Error(), `workstation "review-story" cannot apply work propagation PRESERVE_INPUT`) {
		t.Fatalf("error = %q, want workstation-targeted preserve-input diagnostic", err.Error())
	}
}

func TestOutputToken_OutputAsPayloadExplicit_UsesWorkerOutput(t *testing.T) {
	transformer := New(
		map[string]*petri.Place{
			"task:done": {ID: "task:done", TypeID: "task"},
		},
		map[string]*state.WorkType{
			"task": {ID: "task"},
		},
	)

	token, err := transformer.OutputToken(OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:done", Direction: petri.ArcOutput},
		},
		InputColors: []interfaces.TokenColor{{
			WorkTypeID: "task",
			WorkID:     "work-1",
			Payload:    []byte("input-payload"),
		}},
		Output:              "worker-output",
		WorkPropagationMode: interfaces.WorkPropagationModeOutputAsPayload,
		Outcome:             interfaces.OutcomeAccepted,
		Now:                 time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC),
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("OutputToken() error = %v", err)
	}
	if string(token.Color.Payload) != "worker-output" {
		t.Fatalf("payload = %q, want worker-output", token.Color.Payload)
	}
	if len(token.Color.Content) != 1 || token.Color.Content[0].Text != "worker-output" {
		t.Fatalf("content = %#v, want response content not submitted request", token.Color.Content)
	}
}
