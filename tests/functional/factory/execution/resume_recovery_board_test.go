package execution_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertFailedCronBoard(t *testing.T, process support.Process, routes *sync.Map) {
	session := uuid.NewString()
	resumeCronBoard(t, process, routes, session, cronPayload(t, session, workers.OutcomeFailed), workers.OutcomeFailed)
}

func assertFailedCronSuccessor(t *testing.T, process support.Process, routes *sync.Map) {
	session := uuid.NewString()
	successor := resumeCronBoard(t, process, routes, session, cronPayload(t, session, workers.OutcomeFailed), workers.OutcomeFailed)
	resumeCronBoard(t, process, routes, uuid.NewString(), successor, workers.OutcomeFailed)
}

func assertAcceptedCronBoard(t *testing.T, process support.Process, routes *sync.Map) {
	session := uuid.NewString()
	resumeCronBoard(t, process, routes, session, cronPayload(t, session, workers.OutcomeAccepted), workers.OutcomeAccepted)
}

func resumeCronBoard(t *testing.T, process support.Process, routes *sync.Map, session string, payload []byte, outcome workers.WorkOutcome) []byte {
	t.Helper()
	host := startResume(t, process, routes, session, payload, failedCronFactory())
	board := host.list(t)
	assertBoard(t, board, map[string]string{"ordinary-init": "init", "ordinary-blocked": "blocked"})
	for _, item := range board.Results {
		if item.WorkId != nil && *item.WorkId == "historical-cron" {
			t.Fatal("consumed cron was reseeded on the live board")
		}
	}
	assertCronHistory(t, host.ctx, host.endpoint, session, outcome)
	data := host.finish(t, payload)
	if outcome == workers.OutcomeFailed {
		assertDispositionLog(t, host.home, session, 1)
	}
	return data
}

type resumeHost struct {
	process                                    support.Process
	ctx                                        context.Context
	command                                    *support.ProcessCommand
	endpoint, session, home, source, successor string
	env                                        []string
}

func startResume(t *testing.T, process support.Process, routes *sync.Map, session string, payload []byte, factory map[string]any, mockConfig ...string) *resumeHost {
	t.Helper()
	dir, home := support.ScaffoldFactory(t, factory), t.TempDir()
	source, successor := filepath.Join(dir, "source.json"), filepath.Join(dir, "successor.json")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	routes.Store(port, ready)
	t.Cleanup(func() { routes.Delete(port) })
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)

	inputs := support.FakeInputs(ctx, []string{"you", "run", "--continuously", "--session", session, "--dir", dir,
		"--resume", source, "--record", successor, "--with-mock-workers", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--quiet"})
	if len(mockConfig) > 0 {
		inputs.Input.Args = append(inputs.Input.Args, mockConfig[0])
	}
	inputs.Input.Stdin = strings.NewReader("")
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE=", "HOMEPATH=")
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	var endpoint string
	select {
	case endpoint = <-ready:
	case <-command.Done():
		for cause := command.Err(); cause != nil; cause = errors.Unwrap(cause) {
			t.Logf("synthetic resume cause: %T: %v", cause, cause)
		}
		command.AcceptError()
		t.Fatalf("resume failed before binding: %v; %s", command.Err(), inputs.Stderr())
	case <-ctx.Done():
		t.Fatal("resume never bound")
	}
	return &resumeHost{process: process, ctx: ctx, command: command, endpoint: endpoint, session: session,
		home: home, source: source, successor: successor, env: inputs.Input.Env}
}

func (host *resumeHost) list(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	list := support.FakeInputs(host.ctx, []string{"you", "work", "list", "--server", host.endpoint, "--session", host.session, "--json"})
	list.Input.Env = host.env
	if err := host.process.Execute(list.Input); err != nil {
		t.Fatalf("work list: %v; %s", err, list.Stderr())
	}
	var board factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(list.Stdout()), &board); err != nil {
		t.Fatalf("board: %v; %s", err, list.Stdout())
	}
	return board
}

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

func (host *resumeHost) finish(t *testing.T, payload []byte) []byte {
	t.Helper()
	host.command.Stop(t)
	listener, err := net.Listen("tcp", strings.TrimPrefix(host.endpoint, "http://"))
	if err != nil {
		t.Fatalf("owned listener was not released: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(host.source); err != nil || !bytes.Equal(after, payload) {
		t.Fatalf("source changed: %v", err)
	}
	data, err := os.ReadFile(host.successor)
	if err != nil {
		t.Fatal(err)
	}
	return data
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
