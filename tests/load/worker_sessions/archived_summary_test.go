package workersessions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Seed 26 ended captures with 5/15 MiB histories (less than 30 MiB total).
// No binary, provider or subprocess executes. Startup is measured separately;
// each selected HTTP read consumes its entire response. One cold and two warm
// samples per capture are diagnostics, never wall-clock assertions.
func BenchmarkArchivedDirectSummary(b *testing.B) {
	dir := b.TempDir()
	writeFleetFactory(b, dir)
	bytes := seedArchivedSummaryProfile(b, dir)
	ready := make(chan http.Handler, 1)
	started := time.Now()
	process, err := root.BuildProcess(b.Context(), edges.Edges{
		FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: dir},
		APIServerStarter: func(ctx context.Context, req platformhttp.StartRequest) error {
			ready <- req.Handler
			if req.OnBound != nil {
				req.OnBound(platformhttp.Binding{Port: req.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = process.Close(context.Background()) })
	handler := startArchivedHost(b, process, dir, ready)
	b.Logf("captures=26 profileBytes=%d startup=%s", bytes, time.Since(started))
	for _, size := range []int{5, 15} {
		b.Run(fmt.Sprintf("history-%dMiB", size), func(b *testing.B) {
			path := "/worker-sessions/archived-" + strconv.Itoa(size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for _, phase := range []string{"cold", "warm-1", "warm-2"} {
					started := time.Now()
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
					elapsed := time.Since(started)
					assertArchivedBenchmarkResponse(b, response, "archived-"+strconv.Itoa(size))
					b.Logf("%s %s %s responseBytes=%d", path, phase, elapsed, response.Body.Len())
				}
			}
			b.StopTimer()
		})
	}
}

func assertArchivedBenchmarkResponse(b *testing.B, response *httptest.ResponseRecorder, id string) {
	b.Helper()
	var observation factoryapi.WorkerSessionObservation
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &observation) != nil ||
		observation.WorkerSessionId != id || observation.Model == nil || *observation.Model != "summary-model" ||
		observation.State != factoryapi.WorkerSessionObservationStateCompleted || observation.TokenUsage == nil ||
		observation.TokenUsage.TotalTokens == nil || *observation.TokenUsage.TotalTokens != 12 ||
		observation.TokenUsage.InputTokens == nil || *observation.TokenUsage.InputTokens != 0 {
		b.Fatalf("summary did not preserve response: %d %s", response.Code, response.Body.String())
	}
}

func seedArchivedSummaryProfile(b *testing.B, dir string) int {
	b.Helper()
	storeRoot := filepath.Join(dir, ".you-agent-factory", "worker-recordings")
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		b.Fatal(err)
	}
	bytes := 0
	for index := range 26 {
		size := 0
		if index == 5 || index == 15 {
			size = index
		}
		id := "archived-" + strconv.Itoa(index)
		snapshot := archivedSummaryFixture(id, size)
		if _, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot, WorkerSessionID: id}); err != nil {
			b.Fatalf("seed invalid: %v", err)
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			b.Fatal(err)
		}
		bytes += len(data)
		digest := sha256.Sum256([]byte(id))
		if err := os.WriteFile(filepath.Join(storeRoot, hex.EncodeToString(digest[:])+".worker.json"), data, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	if bytes > 30<<20 {
		b.Fatalf("fixture exceeded 30 MiB: %d", bytes)
	}
	return bytes
}

func archivedSummaryFixture(id string, size int) recordings.WorkerRecordingSnapshot {
	topic := events.Topic("worker-session/" + id + "/events")
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	opening, _ := json.Marshal(workers.SessionPayload{WorkerSessionID: id, AttemptID: "attempt", Status: "STARTING", Model: "summary-model", StartedAt: &start})
	records := []events.Record{archivedFixtureRecord(topic, 1, workers.KindSession, workers.PhaseStarted, opening)}
	// Many source records expose full-snapshot copying and repeated draft decode;
	// the bounded summary excludes every output record and retains no copy.
	for index := range size * 16 {
		payload, _ := json.Marshal(map[string]string{"label": strings.Repeat("x", 64<<10)})
		records = append(records, archivedFixtureRecord(topic, index+2, workers.KindProgress, workers.PhaseUpdated, payload))
	}
	records = append(records, archivedFixtureRecord(topic, len(records)+1, workers.KindUsage, workers.PhaseUpdated, []byte(`{"inputTokens":0,"totalTokens":12}`)))
	records = append(records, archivedFixtureRecord(topic, len(records)+1, workers.KindSession, workers.PhaseCompleted, []byte(`{"status":"COMPLETED"}`)))
	return recordings.WorkerRecordingSnapshot{RecordingID: id, Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: id, Topic: topic, Records: records}}}
}

func archivedFixtureRecord(topic events.Topic, position int, kind workers.Kind, phase workers.Phase, payload json.RawMessage) events.Record {
	draft, _ := json.Marshal(workers.Draft{Kind: kind, Phase: phase, Payload: payload})
	source, sourceID, sequence, eventID := events.SourceType("fixture"), events.SourceID("fixture"), events.SourceSequence(position), events.SourceEventID(strconv.Itoa(position))
	if kind == workers.KindSession {
		source, sourceID = "worker_session_lifecycle", events.SourceID(strings.TrimSuffix(strings.TrimPrefix(string(topic), "worker-session/"), "/events"))
		sequence, eventID = 1, "started"
		if phase != workers.PhaseStarted {
			sequence, eventID = 2, "terminal"
		}
	}
	return events.Record{ID: events.RecordID{Topic: topic, Position: events.AggregateSequence(position)}, SourceType: source, SourceID: sourceID, SourceSequence: sequence, SourceEventID: eventID, SchemaID: "workers.draft.v1", Payload: draft}
}

func startArchivedHost(b *testing.B, process *application.Process, dir string, ready <-chan http.Handler) http.Handler {
	b.Helper()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(filepath.Join(dir, "home"))
	_ = process.Execute(application.Input{Args: []string{"you", "run", "--factory", filepath.Join(dir, "missing.json")}, Env: env, WorkingDirectory: dir, Context: b.Context(), Stdout: io.Discard, Stderr: io.Discard})
	ctx, cancel := context.WithCancel(b.Context())
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(application.Input{Args: []string{"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record"}, Env: env, WorkingDirectory: dir, Context: ctx, Stdout: io.Discard, Stderr: io.Discard})
	}()
	b.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				b.Errorf("host cleanup: %v", err)
			}
		case <-time.After(10 * time.Second):
			b.Error("host cleanup deadline")
		}
	})
	select {
	case handler := <-ready:
		return handler
	case err := <-done:
		b.Fatalf("host exited: %v", err)
	case <-time.After(time.Minute):
		b.Fatal("host readiness deadline")
	}
	return nil
}
