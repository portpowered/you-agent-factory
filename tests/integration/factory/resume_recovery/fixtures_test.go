package resume_recovery_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const workersOutcomeFailed = workers.OutcomeFailed

func assertBoard(t *testing.T, board factoryapi.ListWorkResponse, wants map[string]string) {
	t.Helper()
	seen := map[string]int{}
	for _, item := range board.Results {
		if item.WorkId == nil {
			continue
		}
		if want, ok := wants[*item.WorkId]; ok {
			seen[*item.WorkId]++
			if item.State == nil || item.State.Name != want {
				t.Fatalf("Work %s state = %#v, want %s", *item.WorkId, item.State, want)
			}
		}
	}
	for id := range wants {
		if seen[id] != 1 {
			t.Fatalf("Work %s listed %d times, want once", id, seen[id])
		}
	}
}

func assertCronHistory(t *testing.T, ctx context.Context, endpoint, session string, outcome workers.WorkOutcome) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/factory-sessions/"+session+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	// This observer owns the published SSE history contract. Its retained count
	// provides a deterministic boundary, without waiting for stream quiescence.
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", response.StatusCode)
	}
	count, err := strconv.Atoi(response.Header.Get("X-Factory-Session-Retained-Event-Count"))
	if err != nil || count < 4 {
		t.Fatalf("retained history count = %d (%v)", count, err)
	}
	scanner := bufio.NewScanner(response.Body)
	requests, failures, admissions, read := 0, 0, 0, 0
	for read < count && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event definitions.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		read++
		switch event.Type {
		case definitions.FactoryEventTypeWorkRequest:
			var payload work.WorkRequestEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			for _, item := range payload.Works {
				if item.WorkID == "historical-cron" && event.Id == "synthetic/1" {
					admissions++
				}
			}
		case definitions.FactoryEventTypeDispatchRequest:
			var payload definitions.DispatchRequestEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			matches := false
			if event.Context.WorkIDs != nil {
				for _, id := range *event.Context.WorkIDs {
					if id == "historical-cron" {
						matches = true
					}
				}
			}
			for _, input := range payload.Inputs {
				if input.WorkID == "historical-cron" {
					matches = true
				}
			}
			if matches {
				requests++
			}

		case definitions.FactoryEventTypeDispatchResponse:
			if event.Context.DispatchID == nil || *event.Context.DispatchID != "historical-dispatch" {
				continue
			}
			var payload workers.DispatchResponseEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Outcome == outcome && (outcome != workers.OutcomeFailed || payload.Error != nil && *payload.Error == "synthetic failure") {
				failures++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if read != count || admissions != 1 || requests != 1 || failures != 1 {
		t.Fatalf("history read/admission/dispatch/failure = %d/%d/%d/%d; retained=%d", read, admissions, requests, failures, count)
	}
}

func failedCronFactory() map[string]any {
	return map[string]any{
		"name": "synthetic-failed-cron",
		"workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "init", "type": "INITIAL"}, {"name": "blocked", "type": "PROCESSING"}, {"name": "complete", "type": "TERMINAL"},
		}}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{{"name": "cron-refresh", "behavior": "CRON", "worker": "cron-worker",
			"cron":    map[string]any{"schedule": "0 0 1 1 *", "triggerAtStart": false},
			"outputs": []map[string]string{{"workType": "task", "state": "init"}},
		}},
	}
}

func cronPayload(t *testing.T, session string, outcome workers.WorkOutcome) []byte {
	t.Helper()
	snapshot, err := definitions.NewFactorySnapshot(failedCronFactory())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cron := work.FactoryWorkItem{ID: "historical-cron", WorkTypeID: definitions.SystemTimeWorkTypeID,
		State: definitions.SystemTimePendingState, Tags: map[string]string{definitions.TimeWorkTagKeySource: definitions.TimeWorkSourceCron, definitions.TimeWorkTagKeyCronWorkstation: "cron-refresh"}}
	values := []struct {
		kind     definitions.FactoryEventType
		payload  any
		dispatch string
	}{
		{definitions.FactoryEventTypeRunRequest, definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base}, ""},
		{definitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{Type: work.WorkRequestTypeFactoryRequestBatch, Works: []work.WorkRequestEventWork{
			{WorkID: cron.ID, RequestID: "cron-request", WorkTypeID: cron.WorkTypeID, State: &work.WorkEventState{Name: cron.State, Type: "PROCESSING"}, Tags: cron.Tags},
			{WorkID: "ordinary-init", RequestID: "cron-request", WorkTypeID: "task", State: &work.WorkEventState{Name: "init", Type: "INITIAL"}},
		}}, ""},
		{definitions.FactoryEventTypeDispatchRequest, definitions.DispatchRequestEventPayload{TransitionID: "cron-refresh", Inputs: []definitions.DispatchConsumedWorkRef{{WorkID: cron.ID}}}, "historical-dispatch"},
		{definitions.FactoryEventTypeDispatchResponse, workers.DispatchResponseEventPayload{TransitionID: "cron-refresh", Outcome: outcome, Error: stringPtr("synthetic failure"), OutputWork: &[]work.WorkRequestEventWork{{WorkID: "ordinary-blocked", WorkTypeID: "task", State: &work.WorkEventState{Name: "blocked", Type: "PROCESSING"}}}}, "historical-dispatch"},
	}
	events := make([]definitions.FactoryEvent, 0, len(values))
	for i, v := range values {
		data, err := json.Marshal(v.payload)
		if err != nil {
			t.Fatal(err)
		}
		context := definitions.FactoryEventContext{SessionID: &session, Sequence: i, Tick: i, EventTime: base.Add(time.Duration(i) * time.Second)}
		if v.kind == definitions.FactoryEventTypeWorkRequest {
			context.RequestID = stringPtr("cron-request")
		}
		if v.dispatch != "" {
			context.DispatchID = &v.dispatch
		}
		events = append(events, definitions.FactoryEvent{Id: fmt.Sprintf("synthetic/%d", i), Type: v.kind, SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: data, Context: context})
	}
	data, err := json.Marshal(definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stringPtr(s string) *string { return &s }

func corruptResumeFactory() map[string]any {
	return map[string]any{
		"name": "synthetic-corrupt-resume",
		"workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "ready", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"},
		}}},
		"workers": []any{}, "workstations": []any{},
	}
}

func corruptResumePayload(t *testing.T, sessionID string) []byte {
	t.Helper()
	snapshot, err := definitions.NewFactorySnapshot(corruptResumeFactory())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := []struct {
		kind    definitions.FactoryEventType
		payload any
	}{
		{definitions.FactoryEventTypeRunRequest, definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base}},
		{definitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Type: work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{WorkID: "work-corrupt", RequestID: "request-corrupt", Name: "synthetic", WorkTypeID: "task",
				State:   &work.WorkEventState{Name: "missing", Type: "PROCESSING"},
				Content: []work.WorkContentPart{{Type: "text", Text: "PRIVATE-PROMPT"}},
			}},
		}},
	}
	events := make([]definitions.FactoryEvent, 0, len(values))
	for i, value := range values {
		data, err := json.Marshal(value.payload)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, definitions.FactoryEvent{Id: fmt.Sprintf("synthetic/%d", i), Type: value.kind,
			SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: data,
			Context: definitions.FactoryEventContext{EventTime: base.Add(time.Duration(i) * time.Second),
				Sequence: i, Tick: i, SessionID: &sessionID},
		})
	}
	payload, err := json.Marshal(definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
