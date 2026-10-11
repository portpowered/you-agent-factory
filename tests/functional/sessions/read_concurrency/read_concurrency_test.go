package read_concurrency_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One canonical process/edge shape serves parallel, addressed session scenarios.
// Each leaf owns its directories, Work names, streams and cleanup. All assertions
// observe HTTP bodies or SSE; the existing recorder capability injects workload.
func TestSessionReads(t *testing.T) {
	var ledger recordings.Ledger
	runner := support.NewRecordingCommandRunner("peer COMPLETE")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: readFactory(t, false),
		Edges:      edges.Edges{ProviderCommandRunner: runner, RecordingsRootObserver: func(service recordings.Service) { ledger = service.(recordings.Ledger) }},
	})
	for _, scenario := range []struct {
		name string
		run  func(*testing.T, string)
	}{
		{"F01 gated replay and F06 peer dispatch", func(t *testing.T, base string) { testGatedReads(t, base, ledger, runner) }},
		{"F02 ordered replay live and reconnect", testReplayLive},
		{"F03 canceled slow consumer and healthy peer", testCanceledConsumer},
		{"F04 empty and absent addressed reads", testEmptyAbsent},
		{"F05 invalid cursor and terminal replay", testTerminalReplay},
		{"F06 replacement keeps separate generation", testReplacement},
	} {
		t.Run(scenario.name, func(t *testing.T) { t.Parallel(); scenario.run(t, server.URL()) })
	}
}

func readFactory(t *testing.T, provider bool) string {
	t.Helper()
	config := map[string]any{"name": "read-concurrency", "workTypes": []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}}, "workers": []any{}, "workstations": []any{}}
	if provider {
		config["workers"] = []map[string]string{{"name": "processor"}}
		config["workstations"] = []map[string]any{{"name": "process", "worker": "processor", "inputs": []map[string]string{{"workType": "task", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}}}
	}
	dir := support.ScaffoldFactory(t, config)
	if provider {
		support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "gpt-5-codex"))
	}
	return dir
}

func openReadSession(t *testing.T, base string) string {
	t.Helper()
	id := support.OpenFactorySessionAt(t, base, readFactory(t, false)).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, base, id) })
	return id
}

func submitReadWork(t *testing.T, base, id, name string) string {
	t.Helper()
	result := support.SubmitSessionWorkAt(t, base, id, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]any{"message": name}})
	if !result.Accepted || result.WorkId == nil {
		t.Fatalf("Work admission = %#v", result)
	}
	return *result.WorkId
}

func testGatedReads(t *testing.T, base string, ledger recordings.Ledger, runner *support.RecordingCommandRunner) {
	id := openReadSession(t, base)
	marker := uuid.NewString()
	workID := submitReadWork(t, base, id, marker)
	assertReadWork(t, base, id, workID)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() {
		ledger.AddEventRecorder(func(event factorydefinitions.FactoryEvent) {
			if bytes.Contains(event.Payload, []byte(`"name":"`+marker+`"`)) {
				once.Do(func() { close(entered); <-release })
			}
		})
		close(done)
	}()
	defer func() { close(release); <-done }()
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("owned recorder replay never entered gate")
	}
	var listing factoryapi.ListFactorySessionsResponse
	readJSON(t, base+"/factory-sessions", http.StatusOK, &listing)
	found := false
	for _, session := range listing.Sessions {
		if session.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("owned session missing from fleet")
	}
	var detail factoryapi.FactorySessionSummary
	readJSON(t, base+"/factory-sessions/"+id, http.StatusOK, &detail)
	if detail.Id != id {
		t.Fatalf("addressed detail = %s, want %s", detail.Id, id)
	}
	assertReadWork(t, base, id, workID)
	peer := support.OpenFactorySessionAt(t, base, readFactory(t, true)).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, base, peer) })
	peerID := submitReadWork(t, base, peer, "peer-"+marker)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := runner.WaitForCall(ctx, 1); err != nil {
		t.Fatalf("peer provider command while replay held: %v", err)
	}
	assertReadWork(t, base, peer, peerID)
}

func testReplayLive(t *testing.T, base string) {
	id := openReadSession(t, base)
	submitReadWork(t, base, id, "prefix")
	prefix := support.GetFactoryEventsForSessionAt(t, base, id)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(base, id))
	for _, want := range prefix {
		got := stream.NextEvent(15 * time.Second)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("replay differs: got %s want %s", got.Id, want.Id)
		}
	}
	submitReadWork(t, base, id, "live")
	live := readUntilWorkName(t, stream, "live")
	all := support.GetFactoryEventsForSessionAt(t, base, id)
	assertOrderedEvents(t, all)
	last := prefix[len(prefix)-1]
	sequence := support.ReconnectSequenceForFactoryEvent(last)
	suffix := support.GetFactoryEventsAfterForSessionAt(t, base, id, support.FactoryEventReadCursor{AfterEventID: last.Id, AfterSequence: &sequence})
	if !reflect.DeepEqual(suffix, all[len(prefix):]) {
		t.Fatal("reconnect lost/duplicated/reordered suffix")
	}
	if live.Id != suffix[len(suffix)-1].Id {
		t.Fatal("live tail differs from reconnect")
	}
}

func testCanceledConsumer(t *testing.T, base string) {
	id := openReadSession(t, base)
	ctx, cancel := context.WithCancel(t.Context())
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, support.SessionEventsURL(base, id), nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("slow SSE HTTP %d", response.StatusCode)
	}
	peer := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(base, id))
	submitReadWork(t, base, id, "healthy-before-cancel")
	readUntilWorkName(t, peer, "healthy-before-cancel")
	cancel()
	_, err = io.Copy(io.Discard, response.Body)
	if err == nil || ctx.Err() != context.Canceled {
		t.Fatalf("canceled stream error = %v, context = %v", err, ctx.Err())
	}
	workID := submitReadWork(t, base, id, "healthy-after-cancel")
	readUntilWorkName(t, peer, "healthy-after-cancel")
	assertReadWork(t, base, id, workID)
}

func testEmptyAbsent(t *testing.T, base string) {
	id := openReadSession(t, base)
	var work factoryapi.ListWorkResponse
	readJSON(t, base+"/factory-sessions/"+id+"/work", http.StatusOK, &work)
	if len(work.Results) != 0 {
		t.Fatal("new session contains Work")
	}
	for _, suffix := range []string{"", "/work"} {
		var failure factoryapi.ErrorResponse
		readJSON(t, base+"/factory-sessions/"+uuid.NewString()+suffix, http.StatusNotFound, &failure)
		if failure.Code != "NOT_FOUND" {
			t.Fatalf("missing session error = %#v", failure)
		}
	}
}

func testTerminalReplay(t *testing.T, base string) {
	id := openReadSession(t, base)
	submitReadWork(t, base, id, "retained")
	failure := support.GetFactoryEventsInvalidCursorErrorAt(t, base, id, support.FactoryEventReadCursor{AfterEventID: "factory-event/missing"})
	if failure.Response.Code != "BAD_REQUEST" || failure.Response.Family != "BAD_REQUEST" || failure.Response.Message != "invalid event reconnect cursor" {
		t.Fatalf("invalid cursor = %#v", failure)
	}
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(base, id))
	support.TerminateFactorySessionAt(t, base, id)
	var delivered []factoryapi.FactoryEvent
	for {
		event, ok := stream.TryNextEvent(15 * time.Second)
		if !ok {
			break
		}
		delivered = append(delivered, event)
	}
	stream.WaitClosed(15 * time.Second)
	retained := support.GetFactoryEventsForSessionAt(t, base, id)
	assertOrderedEvents(t, retained)
	if !reflect.DeepEqual(delivered, retained) {
		t.Fatal("terminal drain differs from retained replay")
	}
}

func testReplacement(t *testing.T, base string) {
	dir := readFactory(t, false)
	old := support.OpenFactorySessionAt(t, base, dir).Session.Id
	submitReadWork(t, base, old, "old-generation-only")
	var before factoryapi.FactorySessionSummary
	readJSON(t, base+"/factory-sessions/"+old, http.StatusOK, &before)
	support.CloseFactorySessionAt(t, base, old)
	id := support.OpenFactorySessionAt(t, base, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, base, id) })
	var after factoryapi.FactorySessionSummary
	readJSON(t, base+"/factory-sessions/"+id, http.StatusOK, &after)
	if old == id || before.Runtime == nil || after.Runtime == nil || before.Runtime.StreamIdentity == nil || after.Runtime.StreamIdentity == nil || before.Runtime.StreamIdentity.StreamGenerationID == after.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatal("replacement reused old generation")
	}
	var work factoryapi.ListWorkResponse
	readJSON(t, base+"/factory-sessions/"+id+"/work", http.StatusOK, &work)
	if len(work.Results) != 0 {
		t.Fatal("old Work populated replacement")
	}
	for _, event := range support.GetFactoryEventsForSessionAt(t, base, id) {
		if strings.Contains(eventPayloadJSON(event), "old-generation-only") {
			t.Fatal("old event populated replacement")
		}
	}
}

func readUntilWorkName(t *testing.T, stream *support.FactoryEventStream, name string) factoryapi.FactoryEvent {
	t.Helper()
	for range 64 {
		event := stream.NextEvent(15 * time.Second)
		if event.Type == factoryapi.FactoryEventTypeWorkRequest && strings.Contains(eventPayloadJSON(event), `"name":"`+name+`"`) {
			return event
		}
	}
	t.Fatalf("stream omitted Work %q", name)
	return factoryapi.FactoryEvent{}
}

func assertOrderedEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	seen := map[int]bool{}
	for index, event := range events {
		if seen[event.Context.Sequence] || (index > 0 && event.Context.Sequence <= events[index-1].Context.Sequence) {
			t.Fatalf("reordered/duplicated event %s sequence %d", event.Id, event.Context.Sequence)
		}
		seen[event.Context.Sequence] = true
	}
}

func assertReadWork(t *testing.T, base, id, workID string) {
	t.Helper()
	var result factoryapi.ListWorkResponse
	readJSON(t, base+"/factory-sessions/"+id+"/work", http.StatusOK, &result)
	for _, item := range result.Results {
		if item.WorkId != nil && *item.WorkId == workID {
			return
		}
	}
	t.Fatalf("addressed session %s omitted Work %s", id, workID)
}

func readJSON(t *testing.T, endpoint string, status int, target any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("read must progress before gate release: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s HTTP %d want %d: %s", endpoint, response.StatusCode, status, body)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func eventPayloadJSON(event factoryapi.FactoryEvent) string {
	payload, _ := json.Marshal(event.Payload)
	return string(payload)
}
