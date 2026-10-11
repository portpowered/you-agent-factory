//go:build functionallong

package replay_contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type replayNoProvider struct{}

func (replayNoProvider) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	panic("historical replay launched a provider")
}

func TestResumedHistoricalReplayPublicBoundaries(t *testing.T) {
	var reader recordings.Service
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:  replayNoProvider{},
		RecordingsRootObserver: func(service recordings.Service) { reader = service },
	})
	for _, cell := range []resumedHistoryCell{
		{"canceled-prefix", 11, false, "init", "submitted"},
		{"accepted-prefix", 21, false, "complete", "accepted"},
		{"complete-prefix", 22, false, "complete", "accepted"},
		{"distinct-peer", 31, false, "complete", "accepted"},
		{"failed-history", 22, true, "failed", "submitted"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			assertResumedHistoryCell(t, process, reader, cell)
		})
	}
	for _, missing := range []bool{false, true} {
		for _, prefix := range []bool{false, true} {
			t.Run(fmt.Sprintf("json-missing=%t-prefix=%t", missing, prefix), func(t *testing.T) {
				t.Parallel()
				path, _ := writeResumedHistory(t, uuid.NewString(), 22, false)
				if missing {
					path = filepath.Join(t.TempDir(), "missing.jsonl")
				}
				inputs := replayHistoryInputs(t, path)
				if prefix {
					inputs.Args = append([]string{"you", "--json"}, inputs.Args[1:]...)
				} else {
					inputs.Args = append(inputs.Args, "--json")
				}
				if err := process.Execute(inputs.Input); err == nil {
					t.Fatal("historical JSON succeeded")
				}
				if inputs.Stdout() != "" {
					t.Fatalf("JSON stdout = %q", inputs.Stdout())
				}
				var diagnostic struct{ Code, Message string }
				first := strings.Split(strings.TrimSpace(inputs.Stderr()), "\n")[0]
				if err := json.Unmarshal([]byte(first), &diagnostic); err != nil || diagnostic.Code != "INVOCATION_OUTPUT_UNSUPPORTED" || strings.Contains(diagnostic.Message, path) {
					t.Fatalf("JSON diagnostic = %s", inputs.Stderr())
				}
			})
		}
	}
}

type resumedHistoryCell struct {
	name           string
	end            int
	failed         bool
	state, content string
}

func assertResumedHistoryCell(t *testing.T, process support.Process, reader recordings.Service, cell resumedHistoryCell) {
	t.Helper()
	sessionID := uuid.NewString()
	path, events := writeResumedHistory(t, sessionID, cell.end, cell.failed)
	before, _ := os.ReadFile(path)
	inputs := replayHistoryInputs(t, path)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("historical replay: %v; %s", err, inputs.Stderr())
	}
	want := fmt.Sprintf(`Work: id="resumed" type="task" state=%q trace="trace-resumed"`, cell.state)
	if !strings.Contains(inputs.Stdout(), want) || !strings.Contains(inputs.Stdout(), `Lineage: work="resumed" current="trace-resumed"`) {
		t.Fatalf("historical Work/lineage missing: %s", inputs.Stdout())
	}
	last := -1
	for index, event := range events {
		row := fmt.Sprintf("Event %d: %s (%s)", index, event.Type, event.Id)
		position := strings.Index(inputs.Stdout(), row)
		if position <= last {
			t.Fatalf("lost ordered row %q: %s", row, inputs.Stdout())
		}
		last = position
	}
	assertResumedCanonicalRead(t, reader, path, sessionID, cell)
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("historical replay mutated recording")
	}
}

func replayHistoryInputs(t *testing.T, path string) *support.CapturedInputs {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--replay", path, "--no-record"})
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.WorkingDirectory = dir
	inputs.Stdin = strings.NewReader("")
	return inputs
}

func writeResumedHistory(t *testing.T, sessionID string, end int, failed bool) (string, []interfaces.FactoryEvent) {
	t.Helper()
	const timestamp = "2026-10-11T12:00:00Z"
	dir := t.TempDir()
	factory := map[string]any{"name": "history", "factoryDirectory": dir, "sourceDirectory": dir,
		"workTypes": []any{map[string]any{"name": "task", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}}}}

	var output bytes.Buffer
	encode := json.NewEncoder(&output)
	if err := encode.Encode(map[string]any{"recordType": "header", "schemaVersion": "agent-factory.replay.v2", "recordedAt": timestamp, "sessionId": sessionID,
		"factoryIdentity": map[string]any{"id": "history", "name": "history", "factoryDirectory": dir, "sourceDirectory": dir},
		"hashes":          map[string]string{"factory_hash": "fixture", "workers_hash": "fixture", "workstations_hash": "fixture", "runtime_config_hash": "fixture"}}); err != nil {
		t.Fatal(err)
	}
	events := make([]interfaces.FactoryEvent, 0, end+1)
	for sequence := 0; sequence <= end; sequence++ {
		event := resumedHistoryEvent(t, sessionID, timestamp, factory, sequence, failed)
		events = append(events, event)
		if err := encode.Encode(map[string]any{"recordType": "event", "event": event}); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "history.jsonl")
	if err := os.WriteFile(path, output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, events
}

func resumedHistoryWork(id, state, text string) map[string]any {
	return map[string]any{"name": id, "workId": id, "workTypeName": "task", "state": map[string]any{"name": state}, "traceId": "trace-" + id, "currentChainingTraceId": "trace-" + id, "content": []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}}}
}

func resumedHistoryEvent(t *testing.T, sessionID, timestamp string, factory any, sequence int, failed bool) interfaces.FactoryEvent {
	t.Helper()
	kind, tick, dispatch, id := "FACTORY_STATE_RESPONSE", 0, "", "resumed"
	payload := map[string]any{"state": "RUNNING"}
	switch sequence {
	case 0:
		kind, payload = "RUN_REQUEST", map[string]any{"factory": factory, "recordedAt": timestamp}
	case 1:
		kind, payload = "INITIAL_STRUCTURE_REQUEST", map[string]any{"factory": factory}
	case 2, 25:
		if sequence == 25 {
			id = "peer"
		}
		kind, tick, payload = "WORK_REQUEST", 1, map[string]any{"type": "FACTORY_REQUEST_BATCH", "works": []any{resumedHistoryWork(id, "init", "submitted")}}
	case 3, 14, 26:
		dispatch = "original"
		if sequence == 14 {
			dispatch = "retry"
		}
		if sequence == 26 {
			dispatch, id = "peer-dispatch", "peer"
		}
		kind, tick, payload = "DISPATCH_REQUEST", 2, map[string]any{"transitionId": "review", "inputs": []any{map[string]any{"workId": id}}}
	case 11, 21, 30:
		kind = "DISPATCH_RESPONSE"
		dispatch, tick, id, payload = resumedHistoryResponse(sequence, failed)
	case 22, 31:
		if sequence == 31 {
			id = "peer"
		}
		state := "complete"
		if failed {
			state = "failed"
		}
		kind, tick, payload = "WORK_STATE_CHANGE", 3, map[string]any{"workId": id, "workTypeName": "task", "fromState": "init", "fromPlaceId": "task:init", "toState": state, "toPlaceId": "task:" + state, "source": "dispatch"}
	}
	context := map[string]any{"eventTime": timestamp, "sequence": sequence, "tick": tick, "sessionId": sessionID, "requestId": "request-" + id, "workIds": []string{id}, "traceIds": []string{"trace-" + id}}
	if dispatch != "" {
		context["dispatchId"] = dispatch
	}
	encoded, err := json.Marshal(map[string]any{"id": fmt.Sprintf("history/%d", sequence), "schemaVersion": "agent-factory.event.v1", "type": kind, "context": context, "payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	var event interfaces.FactoryEvent
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func assertResumedCanonicalRead(t *testing.T, reader recordings.Service, path, sessionID string, cell resumedHistoryCell) {
	t.Helper()
	query, err := reader.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: recordings.RecordingID(sessionID), Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}},
	})
	if err != nil {
		t.Fatalf("public historical read: %v", err)
	}
	var state interfaces.FactoryWorldState
	if err := json.Unmarshal([]byte(query.WorldState.Payload), &state); err != nil {
		t.Fatal(err)
	}
	item := state.WorkItemsByID["resumed"]
	if item.State != cell.state || len(item.Content) != 1 || item.Content[0].Text != cell.content {
		t.Fatalf("canonical Work = %#v", item)
	}
	selected := state.PayloadLineage.ResolveSelectedWorkSnapshot("resumed")
	if selected.Snapshot == nil || selected.Snapshot.WorkItem.Content[0].Text != cell.content {
		t.Fatalf("canonical selected content = %#v", selected)
	}
	if cell.end == 31 && state.WorkItemsByID["peer"].TraceID != "trace-peer" {
		t.Fatalf("distinct peer lost: %#v", state.WorkItemsByID)
	}
	if cell.end == 11 && len(state.TerminalWorkByID) != 0 {
		t.Fatal("cancellation fabricated terminal Work")
	}
}

func resumedHistoryResponse(sequence int, failed bool) (string, int, string, map[string]any) {
	dispatch, tick, id := "retry", 3, "resumed"
	outcome, state, content := "ACCEPTED", "complete", "accepted"
	if sequence == 11 {
		dispatch, tick, outcome, state, content = "original", 4, "CANCELED", "init", "submitted"
	}
	if sequence == 30 {
		dispatch, id = "peer-dispatch", "peer"
	}
	if failed && sequence == 21 {
		outcome, state, content = "FAILED", "failed", "submitted"
	}
	payload := map[string]any{"transitionId": "review", "outcome": outcome, "outputWork": []any{resumedHistoryWork(id, state, content)}}
	if failed && sequence == 21 {
		payload["error"] = "controlled failure"
	}
	return dispatch, tick, id, payload
}
