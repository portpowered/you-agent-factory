package recording_summary_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type readGate struct {
	denied     atomic.Bool
	attempts   atomic.Int64
	unreadable string
}

func (gate *readGate) read(path string) ([]byte, error) {
	if strings.HasSuffix(path, ".worker.jsonl") || strings.HasSuffix(path, ".worker.json") {
		if gate.denied.Load() {
			gate.attempts.Add(1)
			return nil, errors.New("recording reads denied")
		}
		if filepath.Base(path) == gate.unreadable {
			return nil, errors.New("private journal path unavailable")
		}
	}
	return os.ReadFile(path)
}

// One root-built host serves independent archived identities concurrently.
// Restart gets a second graph because reconstructing retained state is the
// behavior under test. All reads begin only after /status proves readiness.
func TestArchivedDirectSummaryWithoutRecordingReads(t *testing.T) {
	profile := t.TempDir()
	storeRoot := filepath.Join(profile, ".you-agent-factory", "worker-recordings")
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "peer", "stopped", "owner-lost", "scoped", "damaged", "ambiguous", "unreadable"} {
		factory := ""
		if id == "scoped" {
			factory = "retained-factory"
		}
		seedCapture(t, storeRoot, id, id, factory, id == "stopped")
	}
	seedCapture(t, storeRoot, "collision", "ambiguous", "", false)
	if err := appendFixture(journalPath(storeRoot, "damaged"), []byte("damaged\n")); err != nil {
		t.Fatal(err)
	}
	factory := support.ScaffoldSingleStepFactory(t, "recording-summary")
	gate := &readGate{unreadable: filepath.Base(journalPath(storeRoot, "unreadable"))}
	host := startHost(t, factory, profile, gate)
	gate.denied.Store(true)
	t.Run("first-and-repeated-independent-identities", func(t *testing.T) {
		for _, id := range []string{"first", "peer", "stopped", "owner-lost"} {
			t.Run(id, func(t *testing.T) {
				t.Parallel()
				for i := 0; i < 3; i++ {
					got := readSummary(t, host.Endpoint()+"/worker-sessions/"+id, http.StatusOK)
					assertSummary(t, got, id, id == "stopped")
					*got.Model = "client mutation"
				}
			})
		}
	})
	t.Run("unavailable-is-private", func(t *testing.T) {
		for _, id := range []string{"damaged", "ambiguous", "unreadable"} {
			readSummary(t, host.Endpoint()+"/worker-sessions/"+id, http.StatusInternalServerError)
		}
	})
	t.Run("foreign-factory-is-not-found", func(t *testing.T) {
		readSummary(t, host.Endpoint()+"/factory-sessions/foreign/worker-sessions/scoped", http.StatusNotFound)
	})
	if attempts := gate.attempts.Load(); attempts != 0 {
		t.Fatalf("detail tried %d recording reads", attempts)
	}
	stopHost(t, host)
	// Remove the damaged fixtures from the restart profile so absence is proved
	// from a complete healthy inventory rather than inferred from corrupt bytes.
	clean := t.TempDir()
	cleanRoot := filepath.Join(clean, ".you-agent-factory", "worker-recordings")
	if err := os.MkdirAll(cleanRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "peer", "stopped", "owner-lost"} {
		data, err := os.ReadFile(journalPath(storeRoot, id))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(journalPath(cleanRoot, id), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restartGate := &readGate{}
	restarted := startHost(t, factory, clean, restartGate)
	restartGate.denied.Store(true)
	for _, id := range []string{"first", "stopped", "owner-lost"} {
		assertSummary(t, readSummary(t, restarted.Endpoint()+"/worker-sessions/"+id, http.StatusOK), id, id == "stopped")
	}
	readSummary(t, restarted.Endpoint()+"/worker-sessions/unknown", http.StatusNotFound)
	if attempts := restartGate.attempts.Load(); attempts != 0 {
		t.Fatalf("restart tried %d recording reads", attempts)
	}
	stopHost(t, restarted)
}

func startHost(t *testing.T, factory, profile string, gate *readGate) *support.RootRunFunctionalHost {
	t.Helper()
	host, err := support.StartRootRunFunctionalHost(t.Context(), support.RootRunFunctionalHostConfig{
		FactoryRoot: factory, SystemRoot: t.TempDir(), StartupTimeout: 60 * time.Second,
		FunctionalEdges: serviceedges.Edges{
			FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: profile},
			RecordingReadFile:               gate.read,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopHost(t, host) })
	return host
}

func stopHost(t *testing.T, host *support.RootRunFunctionalHost) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := host.Shutdown(ctx); err != nil {
		t.Error(err)
	}
}

func readSummary(t *testing.T, endpoint string, want int) factoryapi.WorkerSessionObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
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
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("GET %s: %d %s, want %d", endpoint, response.StatusCode, data, want)
	}
	var got factoryapi.WorkerSessionObservation
	if want == http.StatusOK {
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
	} else if strings.Contains(string(data), "private") || strings.Contains(string(data), "damaged\\n") {
		t.Fatalf("private capture error exposed: %s", data)
	}
	return got
}

func assertSummary(t *testing.T, got factoryapi.WorkerSessionObservation, id string, stopped bool) {
	t.Helper()
	state, cause := "COMPLETED", "COMPLETED"
	if stopped {
		state, cause = "TERMINATED", "OPERATOR_TERMINATE"
	}
	if id == "owner-lost" {
		state, cause = "FAILED", "OWNER_LOST"
		assertOwnerLossHealth(t, got)
	}
	if got.WorkerSessionId != id || string(got.State) != state || got.Model == nil || *got.Model != "later-model" || got.TerminalCause == nil || string(*got.TerminalCause) != cause {
		t.Fatalf("durable identity/model/cause lost: %+v", got)
	}
	assertSummaryUsage(t, got)
	if got.PredecessorWorkerSessionId == nil || *got.PredecessorWorkerSessionId != "prior-"+id {
		t.Fatalf("durable lineage lost: %+v", got)
	}
	assertSummaryTiming(t, got, id == "owner-lost")
}

func assertOwnerLossHealth(t *testing.T, got factoryapi.WorkerSessionObservation) {
	t.Helper()
	if got.RecordingHealth == nil || string(*got.RecordingHealth) != "INCOMPLETE" || got.RecordingHealthReason == nil || *got.RecordingHealthReason != "OWNER_LOST" || got.Failure == nil {
		t.Fatalf("owner-loss health/failure lost: %+v", got)
	}
}

func assertSummaryTiming(t *testing.T, got factoryapi.WorkerSessionObservation, ownerLost bool) {
	t.Helper()
	if got.StartedAt == nil || got.ProviderSessionAvailable || got.ProviderSession != nil {
		t.Fatalf("timing/provider availability lost: %+v", got)
	}
	if ownerLost {
		if got.EndedAt != nil || got.DurationMillis != nil {
			t.Fatalf("owner loss invented callback timing: %+v", got)
		}
	} else if got.EndedAt == nil || got.DurationMillis == nil {
		t.Fatalf("committed terminal timing lost: %+v", got)
	}
}

func assertSummaryUsage(t *testing.T, got factoryapi.WorkerSessionObservation) {
	t.Helper()
	if got.TokenUsage == nil || got.TokenUsage.InputTokens == nil || *got.TokenUsage.InputTokens != 0 || got.TokenUsage.TotalTokens == nil || *got.TokenUsage.TotalTokens != 12 || got.TokenUsage.OutputTokens != nil {
		t.Fatalf("usage presence lost: %+v", got.TokenUsage)
	}
}

func journalPath(root, id string) string {
	digest := sha256.Sum256([]byte(id))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".worker.jsonl")
}

func seedCapture(t *testing.T, storeRoot string, recordingID, id, factory string, stopped bool) {
	t.Helper()
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	opening, _ := json.Marshal(workers.SessionPayload{WorkerSessionID: id, Status: "STARTING", FactorySessionID: factory, AttemptID: "physical-attempt", DispatchID: "physical-attempt", AttemptReason: workers.AttemptReasonResume, StartedAt: &start, Model: "initial-model", Lineage: &workers.SessionLineage{PredecessorWorkerSessionID: "prior-" + id, PreviousDispatchID: "prior-attempt", PreviousAttemptID: "prior-attempt"}})
	payloads := []json.RawMessage{opening, []byte(`{"inputTokens":0,"totalTokens":12}`), []byte(`{"model":"later-model"}`), []byte(`{"status":"COMPLETED"}`)}
	if id == "owner-lost" {
		payloads = payloads[:3]
	}
	for index, payload := range payloads {
		position := index + 1
		kind, phase, source := workers.KindUsage, workers.PhaseUpdated, "provider"
		sequence, eventID := events.SourceSequence(position), strconv.Itoa(position)
		if index == 0 {
			kind, phase, source, sequence, eventID = workers.KindSession, workers.PhaseStarted, "worker_session_lifecycle", 1, "started"
		}
		if index == 3 {
			kind, phase, source, sequence, eventID = workers.KindSession, workers.PhaseCompleted, "worker_session_lifecycle", 2, "terminal"
			if stopped {
				phase, payload = workers.PhaseCanceled, []byte(`{"status":"TERMINATED"}`)
			}
		}
		draft, _ := json.Marshal(workers.Draft{Kind: kind, Phase: phase, DispatchID: "physical-attempt", Payload: payload})
		record := recordings.WorkerRecordingRecord{RecordingID: recordingID, WorkerSessionID: id, Record: events.Record{
			ID:         events.RecordID{Topic: events.Topic("worker-session/" + id + "/events"), Position: events.AggregateSequence(position)},
			SourceType: events.SourceType(source), SourceID: events.SourceID(id), SourceSequence: sequence, SourceEventID: events.SourceEventID(eventID), SchemaID: "workers.draft.v1", Payload: draft,
		}}
		stamp := start.Add(time.Duration(position) * time.Second)
		envelope := map[string]any{"version": 1, "kind": "record", "recordingId": recordingID, "workerSessionId": id, "record": record.Record, "capturedAt": stamp}
		if index == 0 {
			envelope["recordingGenerationId"] = "generation-" + recordingID
			envelope["ownerEpoch"] = "fixture"
			if id == "owner-lost" {
				envelope["ownerEpoch"] = lostOwnerEpoch("prior", 123)
			}
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if err := appendFixture(journalPath(storeRoot, recordingID), append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if stopped {
		seedStop(t, storeRoot, id)
	}
	if id == "owner-lost" {
		seedOwnerLoss(t, storeRoot, id)
	}
}

func lostOwnerEpoch(instance string, pid int) string {
	return fmt.Sprintf(`{"version":1,"runtimeInstanceId":%q,"process":{"host":"fixture-host","pid":%d,"start":%q}}`, instance, pid, instance)
}

func seedOwnerLoss(t *testing.T, root, id string) {
	t.Helper()
	// The owner-loss contract requires canonical field order as well as values.
	data := fmt.Sprintf(`{"recordingGenerationId":%q,"ownerEpoch":%q,"capturedAt":"2026-10-05T00:00:05Z","version":2,"kind":"owner-loss","recordingId":%q,"workerSessionId":%q,"ownerLoss":{"recoveryOwnerEpoch":%q,"childLiveness":"UNKNOWN"}}`,
		"generation-"+id, lostOwnerEpoch("prior", 123), id, id, lostOwnerEpoch("recovered", 456))
	if err := appendFixture(journalPath(root, id), append([]byte(data), '\n')); err != nil {
		t.Fatal(err)
	}
}

func seedStop(t *testing.T, storeRoot string, id string) {
	t.Helper()
	target := recordings.WorkerControlTarget{RecordingID: id, WorkerSessionID: id, RecordingGenerationID: "generation-" + id, OwnerEpoch: "fixture", ExpectedAttemptID: "physical-attempt"}
	record := recordings.WorkerControlOperationRecord{Target: target, Revision: 1, Operation: recordings.WorkerControlOperation{Version: 1, RequestID: "stop", WorkerSessionID: id, ExpectedAttemptID: target.ExpectedAttemptID, Action: "terminate", Phase: "INTENT", InputDigest: fmt.Sprintf("%064x", 1)}}
	writeControlFixture(t, storeRoot, record)
	record.Revision, record.Operation.Phase = 2, "COMPLETED"
	record.Result, _ = json.Marshal(workersessions.ControlResult{Session: workersessions.Session{ID: id, State: workersessions.StateTerminated}, Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "physical-attempt"})
	writeControlFixture(t, storeRoot, record)
}

func writeControlFixture(t *testing.T, root string, record recordings.WorkerControlOperationRecord) {
	t.Helper()
	envelope := map[string]any{"version": 2, "kind": "control-operation", "recordingId": record.Target.RecordingID, "workerSessionId": record.Target.WorkerSessionID, "recordingGenerationId": record.Target.RecordingGenerationID, "ownerEpoch": record.Target.OwnerEpoch, "capturedAt": time.Date(2026, 10, 5, 0, 0, 5, 0, time.UTC), "controlOperation": record}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendFixture(journalPath(root, record.Target.RecordingID), append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func appendFixture(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}
