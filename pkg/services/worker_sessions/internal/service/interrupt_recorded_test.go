package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestInterruptRecordedContextReadsFrozenBoundedPrefix(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"empty", "bounded", "read-failure", "owner-lost", "degraded", "foreign-generation"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			target, fake := interruptContextFixture(t, cell)
			r := &registry{logs: &LogReader{reader: fake}}
			contextText, truncated, err := r.readInterruptContext(t.Context(), interruptPlan{request: workersessions.InterruptRequest{SourceWorkerSessionID: "source"}}, target)
			assertInterruptContextPrefix(t, cell, contextText, truncated, err)
		})
	}
}

func TestInterruptRecordedInputBoundaries(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"", "captured output", strings.Repeat("世", interruptContextMaxBytes/3+1)} {
		t.Run(text[:min(len(text), 9)], func(t *testing.T) {
			t.Parallel()
			_, plan, _ := newDurableInterruptFixture(t)
			plan.request.ResumeMode = "recorded"
			plan.request.ReplacementMessage = "  complete replacement\n世  "
			plan.context, plan.truncated = boundedInterruptContext(text, interruptContextMaxBytes)
			payload, err := encodeInterruptInput(plan)
			if err != nil {
				t.Fatal(err)
			}
			assertInterruptInputSchema(t, payload)
			var input durableInterruptInput
			if err := json.Unmarshal(payload, &input); err != nil {
				t.Fatal(err)
			}
			if input.Version != 2 || input.ResumeMode != "recorded" || input.ProviderReference != nil ||
				input.RecordedContext == nil || input.ContextTruncated == nil || *input.RecordedContext != plan.context ||
				*input.ContextTruncated != plan.truncated || !utf8.ValidString(*input.RecordedContext) || len(*input.RecordedContext) > interruptContextMaxBytes ||
				input.ReplacementMessage != plan.request.ReplacementMessage {
				t.Fatalf("lost recorded input: %+v", input)
			}
			req := plan.request
			req.ResumeMode = "provider"
			if err := validateCapturedInterruptInput(payload, nil, req, plan.dispatchID); !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
				t.Fatalf("changed mode replay: %v", err)
			}
		})
	}
}

func TestInterruptLegacyV1RecipeRemainsProviderOnly(t *testing.T) {
	t.Parallel()
	_, plan, _ := newDurableInterruptFixture(t)
	payload, err := encodeInterruptInput(plan)
	if err != nil {
		t.Fatal(err)
	}
	var input durableInterruptInput
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatal(err)
	}
	input.Version, input.ResumeMode = 1, ""
	payload, err = json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCapturedInterruptInput(payload, nil, plan.request, plan.dispatchID); err != nil {
		t.Fatalf("legacy recipe refused: %v", err)
	}
	plan.request.ResumeMode = "recorded"
	if err := validateCapturedInterruptInput(payload, nil, plan.request, plan.dispatchID); !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
		t.Fatalf("legacy recipe authorized recorded mode: %v", err)
	}
}

func TestInterruptRecordedContextCannotPersistInheritedSecret(t *testing.T) {
	t.Parallel()
	_, plan, _ := newDurableInterruptFixture(t)
	plan.request.ResumeMode = "recorded"
	plan.execution.Execution.ProcessEnvironment = []string{"API_KEY=private-context-secret"}
	plan.context = "private-context-secret"
	if _, err := encodeInterruptInput(plan); !errors.Is(err, recordings.ErrInvalidRecordingRedactionRequest) {
		t.Fatalf("secret context persisted: %v", err)
	}
}

func interruptContextFixture(t *testing.T, cell string) (recordings.WorkerControlTarget, *capturedActivityFake) {
	t.Helper()
	text := ""
	if cell == "bounded" {
		text = strings.Repeat("世", interruptContextMaxBytes/3+1)
	}
	payload, err := json.Marshal(workers.Draft{Kind: workers.KindMessage, Payload: json.RawMessage(`{"text":"` + text + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	target := recordings.WorkerControlTarget{WorkerSessionID: "source", RecordingID: "recording", RecordingGenerationID: "generation", OwnerEpoch: "owner"}
	fake := &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
		Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: "source", RecordingID: "recording", RecordingGenerationID: "generation", OwnerEpoch: "owner", CommittedPosition: 1},
		Health:  recordings.WorkerRecordingStatusIncomplete, NextToken: "live-follow-cursor",
		Records: []recordings.WorkerCapturedRecord{{Record: events.Record{ID: events.RecordID{Position: 1}, Payload: payload}}},
	}}
	if cell == "empty" {
		fake.page.Records[0].Record.Payload = []byte(`{"kind":"SESSION"}`)
	}
	switch cell {
	case "read-failure":
		fake.err = errors.New("private-reader-error")
	case "owner-lost":
		fake.page.OwnerLost = true
	case "degraded":
		fake.page.Health = recordings.WorkerRecordingStatusDegraded
	case "foreign-generation":
		fake.page.Catalog.RecordingGenerationID = "other"
	}
	return target, fake
}

func assertInterruptContextPrefix(t *testing.T, cell, contextText string, truncated bool, err error) {
	t.Helper()
	if cell != "empty" && cell != "bounded" {
		if !errors.Is(err, workersessions.ErrInterruptExecutionUnavailable) || contextText != "" {
			t.Fatalf("unsafe prefix accepted: %q %v", contextText, err)
		}
		return
	}
	if err != nil || len(contextText) > interruptContextMaxBytes || !utf8.ValidString(contextText) || truncated != (cell == "bounded") {
		t.Fatalf("bounded prefix bytes=%d truncated=%v err=%v", len(contextText), truncated, err)
	}
	if cell == "empty" && contextText != "" {
		t.Fatalf("empty prefix = %q", contextText)
	}
}

func TestInterruptSuccessorCollisionIsAdmissionFailure(t *testing.T) {
	t.Parallel()
	for _, collision := range []string{"session", "dispatch"} {
		t.Run(collision, func(t *testing.T) {
			t.Parallel()
			r := newTestRegistry(t)
			r.sessions["source"] = workersessions.Session{ID: "source", State: workersessions.StateCanceled}
			plan := interruptPlan{request: workersessions.InterruptRequest{
				RequestID: "interrupt", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: "replacement",
			}, dispatchID: "source-attempt"}
			if collision == "session" {
				r.sessions["successor"] = workersessions.Session{ID: "successor", State: workersessions.StateRunning}
			} else {
				r.dispatchOwners[continuationDispatchID(plan.dispatchID, "successor")] = "other-session"
			}
			_, err := r.admitInterruptSuccessor(plan)
			if !errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed) || errors.Is(err, workersessions.ErrInterruptSourceConflict) {
				t.Fatalf("successor collision classification = %v", err)
			}
			if r.sessions["source"].State != workersessions.StateCanceled || r.sessions["source"].SuccessorWorkerSessionID != "" {
				t.Fatal("failed admission changed source truth or published lineage")
			}
		})
	}
}
