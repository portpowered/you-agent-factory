package recording

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProviderRecorderUsesDispatchPromptProjection(t *testing.T) {
	var events []workers.InferenceEvent
	runner := NewProviderRunner(
		runnerFunc(func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			return workers.RunnerExecutionResult{Content: "done"}, nil
		}),
		func(event workers.InferenceEvent) { events = append(events, event) },
		func() time.Time { return time.Unix(900, 0).UTC() },
	)

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "dispatch-secret"},
		UserMessage: "visible before=token-secret after",
		PromptRedaction: &workers.PromptRedaction{
			UserMessage:       "visible before=<redacted> after",
			RedactUserMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(events) != 2 || events[0].Request == nil || events[1].Response == nil {
		t.Fatalf("events = %#v, want request and response", events)
	}
	if events[0].Request.Prompt != "visible before=<redacted> after" {
		t.Fatalf("recorded prompt = %q, want adjacent visible text preserved", events[0].Request.Prompt)
	}
	if len(events[0].DeclaredSecretJSONPointers) != 0 || len(events[1].DeclaredSecretJSONPointers) != 0 {
		t.Fatalf("event pointers = %#v/%#v, want no whole-field fallback for valid provenance", events[0].DeclaredSecretJSONPointers, events[1].DeclaredSecretJSONPointers)
	}
}

func TestProviderRecorderIgnoresWorkLevelSecretListWithoutPromptProvenance(t *testing.T) {
	var events []workers.InferenceEvent
	runner := NewProviderRunner(
		runnerFunc(func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			return workers.RunnerExecutionResult{}, nil
		}),
		func(event workers.InferenceEvent) { events = append(events, event) },
		func() time.Time { return time.Unix(901, 0).UTC() },
	)

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:                           work.WorkDispatch{DispatchID: "dispatch-unrelated"},
		UserMessage:                        "unrelated dispatch prompt",
		DeclaredSecretInvocationParameters: []string{"secret"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(events) != 2 || events[0].Request == nil {
		t.Fatalf("events = %#v, want request and response", events)
	}
	if events[0].Request.Prompt != "unrelated dispatch prompt" || len(events[0].DeclaredSecretJSONPointers) != 0 {
		t.Fatalf("unrelated event = %#v, want complete prompt with no provenance", events[0])
	}
}

func TestProviderRecorderFailsClosedForInconsistentPromptProjection(t *testing.T) {
	var events []workers.InferenceEvent
	runner := NewProviderRunner(
		runnerFunc(func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			return workers.RunnerExecutionResult{}, nil
		}),
		func(event workers.InferenceEvent) { events = append(events, event) },
		func() time.Time { return time.Unix(902, 0).UTC() },
	)

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "dispatch-invalid"},
		UserMessage: "secret prompt",
		PromptRedaction: &workers.PromptRedaction{
			RedactUserMessage: true,
			FailClosed:        true,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(events) != 2 || len(events[0].DeclaredSecretJSONPointers) != 1 || events[0].DeclaredSecretJSONPointers[0] != "/prompt" {
		t.Fatalf("request provenance = %#v, want whole prompt fallback", events)
	}
}

func TestProviderRecorderKeepsEmptyPromptAndRetryDecisionsDispatchScoped(t *testing.T) {
	var events []workers.InferenceEvent
	attempts := make(map[string]int)
	runner := NewProviderRunner(
		runnerFunc(func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts[request.Dispatch.DispatchID]++
			if request.Dispatch.DispatchID == "retry-dispatch" && attempts[request.Dispatch.DispatchID] == 1 {
				return workers.RunnerExecutionResult{}, workers.NewProviderError(
					workers.WorkFailureTypeInternalServerError,
					"retry",
					errors.New("retry"),
				)
			}
			return workers.RunnerExecutionResult{}, nil
		}),
		func(event workers.InferenceEvent) { events = append(events, event) },
		func() time.Time { return time.Unix(903, 0).UTC() },
	)
	retryRequest := workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "retry-dispatch"},
		UserMessage: "before secret after",
		PromptRedaction: &workers.PromptRedaction{
			UserMessage:       "before <redacted> after",
			RedactUserMessage: true,
		},
	}
	if _, err := runner.Execute(context.Background(), retryRequest); err == nil {
		t.Fatal("first retry attempt error = nil, want retryable provider error")
	}
	if _, err := runner.Execute(context.Background(), retryRequest); err != nil {
		t.Fatalf("second retry attempt error = %v", err)
	}
	if _, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "empty-dispatch"},
		PromptRedaction: &workers.PromptRedaction{
			RedactUserMessage: true,
		},
	}); err != nil {
		t.Fatalf("empty prompt attempt error = %v", err)
	}

	if len(events) != 6 {
		t.Fatalf("events = %d, want two retry attempts plus one independent attempt", len(events))
	}
	assertDispatchScopedPromptEvents(t, events)
}

func assertDispatchScopedPromptEvents(t *testing.T, events []workers.InferenceEvent) {
	t.Helper()
	wantPrompts := map[string]string{
		"retry-dispatch": "before <redacted> after",
		"empty-dispatch": "",
	}
	for index, event := range events {
		if event.Kind != workers.InferenceEventKindRequest {
			continue
		}
		want, ok := wantPrompts[event.DispatchID]
		if !ok {
			t.Fatalf("unexpected dispatch %q in event %d", event.DispatchID, index)
		}
		if event.Request == nil || event.Request.Prompt != want || len(event.DeclaredSecretJSONPointers) != 0 {
			t.Fatalf("dispatch event %d = %#v, want prompt %q without whole-field provenance", index, event, want)
		}
	}
}
