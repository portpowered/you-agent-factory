package customer_journeys_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// CLI admission/control and API-owned retained reads share one inert process.
// Immutable prompt routes hold all four attempts before scoped cancellation;
// no arrival-order selector or process-wide invocation lock selects an effect.
func TestKeyedSupervisionFourSessionsRetainHistoryBesideLivePeers(t *testing.T) {
	t.Parallel()
	runner := newKeyedRetentionRunner()
	server, capture := keyedRetentionServer(t, runner)
	var sessions [4]string
	var streams [4]*support.FactoryEventStream
	var entries [4]*w4DispatchObservation
	var cursors [4]int
	for i, gate := range runner.gates[:4] {
		sessions[i] = identityOpenSession(t, server, gate.marker)
		streams[i] = support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessions[i]))
		t.Cleanup(streams[i].Close)
	}
	for i, gate := range runner.gates[:4] {
		batch := fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":%q}}]}`, gate.marker, gate.marker, gate.marker)
		identityCLI(t, server, "--session", sessions[i], "submit", "batch", batch)
	}
	for i, gate := range runner.gates[:4] {
		keyedRetentionSignal(t, gate.started, "all four provider admissions")
		entries[i] = identityOnlyDispatch(t, observeW4Dispatches(t, identityLedger(t, server, sessions[i])))
		if entries[i].sessionID == "" || entries[i].result != nil {
			t.Fatalf("attempt %d not independently live: %+v", i, entries[i])
		}
		events := support.GetFactoryEventsForSessionAt(t, server.URL(), sessions[i])
		cursors[i] = support.ReconnectSequenceForFactoryEvent(events[len(events)-1])
	}
	identityCLI(t, server, "--remote", "worker-sessions", "cancel", entries[0].sessionID)
	keyedRetentionSignal(t, runner.gates[0].returned, "target provider joined")
	identityAwaitResponse(t, streams[0])
	keyedRetentionAssertPeersLive(t, runner)
	selected := keyedRetentionWorkerHistory(t, server, sessions[0], entries[0].sessionID, runner.gates[0].marker, "CANCELED")
	selectedRecording := keyedRetentionCanceledRecording(t, server, capture, sessions[0], entries[0].sessionID)
	identityCLI(t, server, "--remote", "session", "terminate", sessions[0])
	keyedRetentionAssertPeersLive(t, runner)
	afterClose := keyedRetentionWorkerHistory(t, server, sessions[0], entries[0].sessionID, runner.gates[0].marker, "CANCELED")
	if !reflect.DeepEqual(selected, afterClose) {
		t.Fatal("scoped close changed retained Worker observations")
	}
	if !reflect.DeepEqual(selectedRecording, keyedRetentionCanceledRecording(t, server, capture, sessions[0], entries[0].sessionID)) {
		t.Fatal("scoped close changed retained canceled recording")
	}
	keyedRetentionCursor(t, server, sessions[0], entries[0], cursors[0])
	// A separate Factory Session control proves the complete durable capture
	// even when stopping the engine prevents a Factory dispatch response.
	// Historical Factory read decorators have a separate later-owner cutover;
	// this assertion observes the real Recordings reader, not that projection.
	gate := runner.gates[4]
	canceledFactory := identityOpenSession(t, server, gate.marker)
	batch := fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":%q}}]}`, gate.marker, gate.marker, gate.marker)
	identityCLI(t, server, "--session", canceledFactory, "submit", "batch", batch)
	keyedRetentionSignal(t, gate.started, "Factory-cancel provider admission")
	canceled := identityOnlyDispatch(t, observeW4Dispatches(t, identityLedger(t, server, canceledFactory)))
	identityCLI(t, server, "--remote", "session", "cancel", canceledFactory)
	keyedRetentionSignal(t, gate.returned, "Factory-cancel provider joined")
	keyedRetentionCanceledRecording(t, server, capture, canceledFactory, canceled.sessionID)
	keyedRetentionAssertPeersLive(t, runner)
	for i := 1; i < len(sessions); i++ {
		runner.gates[i].unblock()
		identityAwaitResponse(t, streams[i])
		entry := identityOnlyDispatch(t, observeW4Dispatches(t, identityLedger(t, server, sessions[i])))
		w4AssertDispatch(t, entry, "trace-"+runner.gates[i].marker, runner.gates[i].marker+" COMPLETE")
		keyedRetentionWorkerHistory(t, server, sessions[i], entry.sessionID, runner.gates[i].marker, "COMPLETED")
		identityInspectRecording(t, server, capture, sessions[i], entry.sessionID, runner.gates[i].marker, runner.gates[0].marker)
		keyedRetentionCursor(t, server, sessions[i], entry, cursors[i])
		for j := 0; j < i; j++ {
			if entry.sessionID == entries[j].sessionID || entry.dispatchID == entries[j].dispatchID {
				t.Fatal("production UUID identities collapsed across explicit sessions")
			}
		}
		identityCLI(t, server, "--remote", "session", "terminate", sessions[i])
	}
}

type keyedRetentionGate struct {
	marker                     string
	started, returned, release chan struct{}
	once                       sync.Once
	calls                      atomic.Int32
}

func (g *keyedRetentionGate) unblock() { g.once.Do(func() { close(g.release) }) }

type keyedRetentionRunner struct{ gates [5]*keyedRetentionGate }

func newKeyedRetentionRunner() *keyedRetentionRunner {
	r := &keyedRetentionRunner{}
	for i := range r.gates {
		r.gates[i] = &keyedRetentionGate{marker: fmt.Sprintf("t16-retained-%c", 'A'+i), started: make(chan struct{}), returned: make(chan struct{}), release: make(chan struct{})}
	}
	return r
}

func (r *keyedRetentionRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := string(req.Stdin) + strings.Join(req.Args, "\n")
	for _, gate := range r.gates {
		if !strings.Contains(prompt, gate.marker) {
			continue
		}
		// Canceling one Worker restores unfinished Work to Petri scheduling.
		// Any replacement remains owned by this target until scoped close;
		// never confuse its completion with the first admitted physical attempt.
		if gate.calls.Add(1) > 1 {
			<-ctx.Done()
			return platformprocess.CommandResult{}, ctx.Err()
		}
		close(gate.started)
		defer close(gate.returned)
		select {
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		case <-gate.release:
			stdout := append([]byte(fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", "provider-"+gate.marker)), support.CodexSuccessStdout(gate.marker+" COMPLETE")...)
			return platformprocess.CommandResult{Stdout: stdout}, nil
		}
	}
	return platformprocess.CommandResult{}, fmt.Errorf("unowned provider request")
}

func keyedRetentionServer(t *testing.T, runner *keyedRetentionRunner) (*identityFixture, *identityRecordingWriter) {
	t.Helper()
	capture := &identityRecordingWriter{}
	dir := support.ScaffoldFactory(t, keyedSupervisionFactoryConfig())
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE=", "HOMEPATH="+home)
	server := &identityFixture{env: env, dir: dir}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: env,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner, WorkerRecordingWriter: capture,
			WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) {
				capture.WorkerRecordingStore = store
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	for _, gate := range runner.gates {
		t.Cleanup(gate.unblock)
	}
	return server, capture
}

func keyedRetentionSignal(t *testing.T, signal <-chan struct{}, property string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatalf("waiting for %s", property)
	}
}

func keyedRetentionAssertPeersLive(t *testing.T, runner *keyedRetentionRunner) {
	t.Helper()
	for i := 1; i < 4; i++ {
		select {
		case <-runner.gates[i].returned:
			t.Fatalf("scoped control joined peer %d", i)
		default:
		}
	}
}

func keyedRetentionWorkerHistory(t *testing.T, server *identityFixture, factoryID, workerID, marker, state string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	identityInspectWorker(t, server, factoryID, workerID, "batch-"+marker+"-"+marker)
	list := support.ListSessionWorkerSessions(t, server.URL(), factoryID, "batch-"+marker+"-"+marker)
	encoded, _ := json.Marshal(list)
	if !strings.Contains(string(encoded), `"state":"`+state+`"`) {
		t.Fatalf("Worker state: %s, want %s", encoded, state)
	}
	if state == "COMPLETED" && !strings.Contains(string(encoded), "provider-"+marker) {
		t.Fatalf("missing exact provider association: %s", encoded)
	}
	events := support.GetWorkerSessionEventsForSessionByIDAt(t, server.URL(), factoryID, workerID)
	for _, event := range events {
		if event.Event.Cursor.WorkerSessionId != nil && *event.Event.Cursor.WorkerSessionId != workerID {
			t.Fatalf("peer Worker cursor: %+v", event)
		}
	}
	keyedRetentionWorkerCursor(t, server, factoryID, workerID, events)
	return events
}

func keyedRetentionCanceledRecording(t *testing.T, server *identityFixture, writer *identityRecordingWriter, factoryID, workerID string) recordings.WorkerSessionRecordingSnapshot {
	t.Helper()
	writer.mu.Lock()
	var recordingID string
	for _, record := range writer.records {
		if record.WorkerSessionID == workerID {
			if record.FactorySessionID != factoryID {
				t.Fatal("canceled capture crossed Factory scope")
			}
			recordingID = record.RecordingID
		}
	}
	writer.mu.Unlock()
	if recordingID == "" {
		t.Fatal("missing canceled recording association")
	}
	snapshot, err := server.WorkerRecordingReader().LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != workerID {
			continue
		}
		if session.Status != recordings.WorkerRecordingStatusComplete || session.ExecutionTerminal == nil || session.ExecutionTerminal.Status != "CANCELED" {
			t.Fatalf("canceled recording lost terminal identity: %+v", session)
		}
		return session
	}
	t.Fatal("canceled recording lost Worker identity")
	return recordings.WorkerSessionRecordingSnapshot{}
}

// Reconnect with the exact acknowledged public cursor and compare retained
// suffix positions. The summary must stay complete after scoped Runtime close.
func keyedRetentionWorkerCursor(t *testing.T, server *identityFixture, factoryID, workerID string, events []factoryapi.WorkerSessionEvent) {
	t.Helper()
	cursor := events[0].Event.Cursor
	endpoint := server.URL() + "/factory-sessions/" + factoryID + "/worker-sessions/" + workerID + "/events?replayOnly=true&after_position=" + strconv.FormatInt(cursor.Position, 10)
	if cursor.StreamGenerationId != nil {
		endpoint += "&stream_generation_id=" + *cursor.StreamGenerationId
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Worker cursor HTTP status %d", response.StatusCode)
	}
	positions, complete := keyedRetentionReadWorkerSuffix(t, response, factoryID, workerID)
	var expected []int64
	for _, event := range events {
		if event.ReplaySummary == nil && event.Event.Position > cursor.Position {
			expected = append(expected, event.Event.Position)
		}
	}
	if !complete || !reflect.DeepEqual(positions, expected) {
		t.Fatalf("Worker cursor suffix=%v complete=%t, want %v complete=true", positions, complete, expected)
	}
}

func keyedRetentionReadWorkerSuffix(t *testing.T, response *http.Response, factoryID, workerID string) ([]int64, bool) {
	t.Helper()
	scanner := bufio.NewScanner(response.Body)
	var positions []int64
	complete := false
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event.ReplaySummary != nil {
			complete = event.ReplaySummary.Complete
			continue
		}
		if event.WorkerSessionId != workerID || event.FactorySessionId == nil || *event.FactorySessionId != factoryID {
			t.Fatalf("Worker reconnect crossed scope: %+v", event)
		}
		positions = append(positions, event.Event.Position)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return positions, complete
}

func keyedRetentionCursor(t *testing.T, server *identityFixture, sessionID string, entry *w4DispatchObservation, cursor int) {
	t.Helper()
	suffix := support.GetFactoryEventsAfterForSessionAt(t, server.URL(), sessionID, support.FactoryEventReadCursor{AfterSequence: &cursor})
	if len(suffix) == 0 {
		t.Fatal("retained Factory cursor lost terminal suffix")
	}
	for _, event := range suffix {
		if support.ReconnectSequenceForFactoryEvent(event) <= cursor {
			t.Fatalf("replayed acknowledged event: %+v", event)
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("peer session in scoped suffix: %+v", event)
		}
		if event.Context.WorkIds != nil && !reflect.DeepEqual(*event.Context.WorkIds, entry.workIDs) {
			t.Fatalf("peer Work in scoped suffix: %+v", event)
		}
	}
}
