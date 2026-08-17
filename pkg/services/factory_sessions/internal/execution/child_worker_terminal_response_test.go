package factorysessionexecution

import (
	"context"
	"encoding/json"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestChildWorkerExecutor_PublishesExactlyOneDurableTerminalResponseForEveryOutcome(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		result        workers.ExecuteResult
		progress      []workers.ProgressFragment
		wantKind      responseevents.Kind
		wantPhase     responseevents.Phase
		wantErrorCode string
	}{
		{
			name: "success with provider terminal progress",
			result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
			},
			progress: []workers.ProgressFragment{
				{Kind: workers.ProgressFragmentKind, Payload: "provider progress"},
				{Kind: workers.CompletedFragmentKind, Type: "COMPLETED"},
			},
			wantKind:  responseevents.KindRun,
			wantPhase: responseevents.PhaseCompleted,
		},
		{
			name: "failure without provider terminal progress",
			result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeFailed,
				Failure: &workers.ExecutionFailure{
					Type:    workers.WorkFailureTypeInternalServerError,
					Message: "provider failed",
				},
			},
			progress: []workers.ProgressFragment{
				{Kind: workers.ProgressFragmentKind, Payload: "failure progress"},
			},
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "stream_failed",
		},
		{
			name: "cancellation without provider terminal progress",
			result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeCanceled,
			},
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "stream_canceled",
		},
		{
			name: "timeout without provider terminal progress",
			result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeFailed,
				Failure: &workers.ExecutionFailure{
					Type:    workers.WorkFailureTypeTimeout,
					Message: "child timed out",
				},
			},
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "timeout",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			const sessionID = "dur-sess-terminal-bridge"
			service := newDurableResponseEventsService(t)
			state := seedResponseEventSession(t, service, sessionID)
			if err := service.ensureSessionResponseEvents(sessionID, state); err != nil {
				t.Fatalf("ensure response events: %v", err)
			}

			invoker := &recordingWorkerExecution{result: test.result}
			invoker.onExecute = func(request workers.ExecuteRequest) {
				for _, fragment := range test.progress {
					if request.Input.ProgressPublisher == nil {
						t.Fatal("Workers Execute request has no progress publisher")
					}
					request.Input.ProgressPublisher(fragment)
				}
			}
			executor := newChildWorkerExecutor(
				sessionID,
				invoker,
				newChildRecordSink(),
				childTestValues{},
				service.observeWorkerDispatch,
				"/project",
				0,
			)
			executor.publish = func(_ string, fragment workers.ProgressFragment) {
				service.PublishWorkerProgress(fragment)
			}

			_, _ = executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "run"})

			cursor, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{
				SessionID: sessionID,
			})
			if err != nil {
				t.Fatalf("SubscribeResponseEvents: %v", err)
			}
			defer cursor.Detach()
			events, err := cursor.Drain()
			if err != nil {
				t.Fatalf("Drain response events: %v", err)
			}

			terminals := make([]responseevents.FactoryResponseEvent, 0, 2)
			for _, event := range events {
				if event.Phase == responseevents.PhaseCompleted ||
					event.Phase == responseevents.PhaseFailed ||
					event.Phase == responseevents.PhaseCanceled {
					terminals = append(terminals, event)
				}
			}
			if len(terminals) != 1 {
				t.Fatalf("response events = %#v, want exactly one terminal event", events)
			}
			terminal := terminals[0]
			if terminal.Kind != test.wantKind || terminal.Phase != test.wantPhase {
				t.Fatalf("terminal event = %#v, want kind=%q phase=%q", terminal, test.wantKind, test.wantPhase)
			}
			if test.wantErrorCode != "" {
				var payload responseevents.ErrorPayload
				if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
					t.Fatalf("decode terminal error payload: %v", err)
				}
				if payload.Code != test.wantErrorCode {
					t.Fatalf("terminal error code = %q, want %q", payload.Code, test.wantErrorCode)
				}
			}
			if len(test.progress) > 0 && (len(events) < 2 || events[len(events)-1].EventID != terminal.EventID) {
				t.Fatalf("events = %#v, want progress before the terminal event", events)
			}
		})
	}
}
