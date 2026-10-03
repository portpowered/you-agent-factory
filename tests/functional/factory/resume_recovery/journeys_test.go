package resume_recovery_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertInterruptedCron(t *testing.T, process support.Process, routes *sync.Map) {
	session := uuid.NewString()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(cronPayload(t, session, workers.OutcomeFailed), &artifact); err != nil {
		t.Fatal(err)
	}
	factory := failedCronFactory()
	factory["workers"] = []map[string]string{{"name": "cron-worker", "type": "SCRIPT_WORKER", "command": "resume-never-execute"}}
	factory["workstations"].([]map[string]any)[0]["type"] = "SCRIPT_RUN"
	snapshot, err := definitions.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Events[0].Payload, err = json.Marshal(definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: artifact.RecordedAt})
	if err != nil {
		t.Fatal(err)
	}
	var admission work.WorkRequestEventPayload
	if err := json.Unmarshal(artifact.Events[1].Payload, &admission); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	admission.Works[0].Tags[definitions.TimeWorkTagKeyDueAt] = now.Add(-time.Minute).Format(time.RFC3339Nano)
	admission.Works[0].Tags[definitions.TimeWorkTagKeyNominalAt] = now.Add(-time.Minute).Format(time.RFC3339Nano)
	admission.Works[0].Tags[definitions.TimeWorkTagKeyExpiresAt] = now.Add(time.Hour).Format(time.RFC3339Nano)
	artifact.Events[1].Payload, err = json.Marshal(admission)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Events = artifact.Events[:3] // Hard interruption before a final response.
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	gate := support.NewMockWorkerGate(t)
	config, err := json.Marshal(workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		RunType:    workers.MockWorkerRunTypeAccept,
		GateConfig: gate.Config(time.Minute),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "mock.json")
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	host := startResume(t, process, routes, session, payload, factory, configPath)
	// The gate's published arrival file observes mock execution; no timer or
	// provider substitute can observe this customer-configured filesystem gate.
	gate.WaitForArrival(t, time.Minute)
	assertBoard(t, host.list(t), map[string]string{"ordinary-init": "init"})
	gate.Release()
	assertResumedCronEvents(t, host)
	host.finish(t, payload)
	assertDispositionLog(t, host.home, session, 0)
}

func assertResumedCronEvents(t *testing.T, host *resumeHost) {
	t.Helper()
	resumedID := ""
	requests, responses, interruptions := 0, 0, 0
	observe := func(event definitions.FactoryEvent) bool {
		if event.Type == definitions.FactoryEventTypeDispatchRequest {
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
				if event.Context.DispatchID != nil && *event.Context.DispatchID != "historical-dispatch" {
					resumedID = *event.Context.DispatchID
				}
			}
		}
		if event.Type == definitions.FactoryEventTypeDispatchInterrupted && event.Context.DispatchID != nil && *event.Context.DispatchID == "historical-dispatch" {
			interruptions++
		}
		if event.Type == definitions.FactoryEventTypeDispatchResponse && event.Context.DispatchID != nil && resumedID != "" && *event.Context.DispatchID == resumedID {
			var payload workers.DispatchResponseEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Outcome != workers.OutcomeAccepted {
				t.Fatalf("resumed outcome = %s", payload.Outcome)
			}
			responses++
			return true
		}
		return false
	}
	readSessionEvents(t, host.ctx, host.endpoint, host.session, true, observe)
	if requests != 2 || responses != 1 || interruptions != 1 {
		t.Fatalf("original/new requests, response, interruption = %d/%d/%d", requests, responses, interruptions)
	}
	requests, responses, interruptions = 0, 0, 0
	readSessionEvents(t, host.ctx, host.endpoint, host.session, false, func(event definitions.FactoryEvent) bool { observe(event); return false })
	if requests != 2 || responses != 1 || interruptions != 1 {
		t.Fatalf("retained requests/response/interruption = %d/%d/%d", requests, responses, interruptions)
	}
}

func assertMoveRoundTrip(t *testing.T, process support.Process, routes *sync.Map) {
	session := uuid.NewString()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(cronPayload(t, session, workers.OutcomeFailed), &artifact); err != nil {
		t.Fatal(err)
	}
	factory := failedCronFactory()
	types := factory["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "to-complete", "type": "PROCESSING"})
	snapshot, err := definitions.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Events[0].Payload, err = json.Marshal(definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: artifact.RecordedAt})
	if err != nil {
		t.Fatal(err)
	}
	var admission work.WorkRequestEventPayload
	if err := json.Unmarshal(artifact.Events[1].Payload, &admission); err != nil {
		t.Fatal(err)
	}
	admission.Works[1].State = &work.WorkEventState{Name: "complete", Type: "TERMINAL"}
	artifact.Events[1].Payload, err = json.Marshal(admission)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	host := startResume(t, process, routes, session, payload, factory)
	assertBoard(t, host.list(t), map[string]string{"ordinary-init": "complete", "ordinary-blocked": "blocked"})
	inputs := support.FakeInputs(host.ctx, []string{"you", "work", "move", "ordinary-init", "to-complete", "--server", host.endpoint, "--session", session, "--request-id", "move-request", "--json"})
	inputs.Input.Env = host.env
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("move: %v; %s", err, inputs.Stderr())
	}
	assertBoard(t, host.list(t), map[string]string{"ordinary-init": "to-complete", "ordinary-blocked": "blocked"})
	assertMoveHistory(t, host)
	successor := host.finish(t, payload)
	restored := startResume(t, process, routes, uuid.NewString(), successor, factory)
	assertBoard(t, restored.list(t), map[string]string{"ordinary-init": "to-complete", "ordinary-blocked": "blocked"})
	assertMoveHistory(t, restored)
	restored.finish(t, successor)
}

func assertMoveHistory(t *testing.T, host *resumeHost) {
	t.Helper()
	moves, admissionSequence := 0, -1
	readSessionEvents(t, host.ctx, host.endpoint, host.session, false, func(event definitions.FactoryEvent) bool {
		if event.Id == "synthetic/1" {
			admissionSequence = event.Context.Sequence
		}
		if event.Type != definitions.FactoryEventTypeWorkStateChange {
			return false
		}
		var payload definitions.WorkStateChangeEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.WorkID != "ordinary-init" {
			return false
		}
		moves++
		if payload.FromState != "complete" || payload.ToState != "to-complete" || payload.Source != work.WorkStateChangeSourceAPI || event.Context.Sequence <= admissionSequence {
			t.Fatalf("move lost state/source/ordering: %#v; sequence=%d", payload, event.Context.Sequence)
		}
		return false
	})
	if moves != 1 || admissionSequence < 0 {
		t.Fatalf("retained moves/admission = %d/%d", moves, admissionSequence)
	}
}

// Reads the public retained boundary, or waits for a scenario's terminal event.
// SSE synchronization returns on the event, with context only as a safety ceiling.
func readSessionEvents(t *testing.T, ctx context.Context, endpoint, session string, live bool, observe func(definitions.FactoryEvent) bool) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/factory-sessions/"+session+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", response.StatusCode)
	}
	count, err := strconv.Atoi(response.Header.Get("X-Factory-Session-Retained-Event-Count"))
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(response.Body)
	read := 0
	for (live || read < count) && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event definitions.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		read++
		if observe(event) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if live || read != count {
		t.Fatalf("events ended before observation: read=%d retained=%d", read, count)
	}
}

func assertDispositionLog(t *testing.T, home, session string, want int) {
	t.Helper()
	// The process construction boundary has no logger override. Observe the
	// actual customer-facing structured log sink in this scenario's fresh home.
	count := 0
	err := filepath.WalkDir(filepath.Join(home, ".you-agent-factory", "logs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range bytes.Split(data, []byte("\n")) {
			var fields map[string]any
			if json.Unmarshal(line, &fields) != nil || fields["event"] != "run.restore.disposition" {
				continue
			}
			count++
			if fields["level"] != "warn" || fields["msg"] != "restore Work board: retained consumed automation Work in history" || fields["session_id"] != session || fields["work_id"] != "historical-cron" || fields["dispatch_id"] != "historical-dispatch" || fields["outcome"] != "FAILED" || fields["disposition"] != "consumed_automation_without_current_placement" || fields["recording_id"] == nil {
				return fmt.Errorf("unsafe or uncorrelated disposition: %#v", fields)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("disposition warnings = %d, want %d", count, want)
	}
}
