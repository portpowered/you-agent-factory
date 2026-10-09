package workersessions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/bits"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Dedicated capacity lane: one root, <=552 Work, <=3,305 real attempts,
// controlled Codex execution, temporary profile, no subprocess or remote call.
// Readiness uses Work state; no scoped list warms the measured request.
func TestWorkScopedRetainedSessionsBoundedWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	dir := t.TempDir()
	writeRetainedFactory(t, dir)
	counts := &retainedReadCounts{}
	core, logs := observer.New(zap.DebugLevel)
	ready := make(chan *httptest.Server, 1)
	process, err := root.BuildProcess(ctx, edges.Edges{
		ProviderCommandRunner: &scopedLatencyRunner{}, ProcessLogger: zap.New(core),
		InvocationMetricsRecorder: counts, WorkerRecordingWriter: counts,
		WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { counts.WorkerRecordingStore = store },
		APIServerStarter: func(ctx context.Context, request platformhttp.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			if request.OnBound != nil {
				request.OnBound(platformhttp.Binding{Port: request.Port})
			}
			ready <- server
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	f := scopedLatencyFixture{dir: dir, environment: builtcliacceptance.ProcessEnvForIsolatedHome(filepath.Join(dir, "home"))}
	server := startScopedLatencyHost(t, ctx, process, f, ready)
	var opened factoryapi.OpenFactorySessionResponse
	fleetPOST(t, server.Config.Handler, "/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: dir}, http.StatusOK, &opened)
	if opened.Session == nil {
		t.Fatal("Session absent")
	}
	f.sessionID, f.serverURL = opened.Session.Id, server.URL
	f.workID = submitRetainedWork(t, server.Config.Handler, f.sessionID, "target")
	empty := submitRetainedWork(t, server.Config.Handler, f.sessionID, "idle")
	waitRetainedWork(t, ctx, server.Config.Handler, f.sessionID, f.workID)
	retained := 4
	var selectedIDs []string
	for _, unrelatedWorks := range []int{334, 550} {
		previous := (retained - 4) / 6
		var pending []string
		for index := previous; index < unrelatedWorks; index++ {
			id := submitRetainedWork(t, server.Config.Handler, f.sessionID, "task")
			pending = append(pending, id)
		}
		waitRetainedWorks(t, ctx, server.Config.Handler, f.sessionID, pending)
		retained += 6 * len(pending)
		assertRetainedInventory(t, ctx, counts.WorkerRecordingStore, f.sessionID, retained)
		ids := measureRetainedReads(t, ctx, server.Config.Handler, f, empty, retained, counts, logs)
		if selectedIDs != nil && !slices.Equal(selectedIDs, ids) {
			t.Fatalf("selected identities/order changed with unrelated membership: %v -> %v", selectedIDs, ids)
		}
		selectedIDs = ids
	}
}

func measureRetainedReads(t *testing.T, ctx context.Context, handler http.Handler, f scopedLatencyFixture, empty string, retained int, counts *retainedReadCounts, logs *observer.ObservedLogs) []string {
	t.Helper()
	var selectedIDs []string
	for _, selected := range []struct {
		id   string
		rows int
	}{{f.workID, 4}, {empty, 0}} {
		for sample := range 2 {
			// Commit a new attempt for the same selected Work after its first
			// read at the larger retained count. Readiness observes Work state,
			// never a scoped-list warm-up.
			if retained == 3304 && selected.rows == 4 && sample == 1 {
				var moved factoryapi.WorkRead
				fleetPOST(t, handler, "/factory-sessions/"+f.sessionID+"/work/"+f.workID+"/move", factoryapi.MoveWorkRequest{StateName: "stage-2"}, http.StatusOK, &moved)
				waitRetainedWork(t, ctx, handler, f.sessionID, f.workID)
				assertRetainedInventory(t, ctx, counts.WorkerRecordingStore, f.sessionID, retained+1)
				retained++
				selected.rows++
			}
			counts.reset()
			logs.TakeAll()
			path := "/factory-sessions/" + f.sessionID + "/worker-sessions?workId=" + url.QueryEscape(selected.id)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
			ids := assertRetainedRead(t, response, selected.id, selected.rows)
			if selected.rows != 0 {
				if selected.rows == 5 {
					if len(ids) != len(selectedIDs)+1 || !slices.Equal(selectedIDs, ids[:len(selectedIDs)]) {
						t.Fatalf("selected commit lost/reordered prior attempts: %v -> %v", selectedIDs, ids)
					}
				} else if selectedIDs != nil && !slices.Equal(selectedIDs, ids) {
					t.Fatalf("selected identities/order changed between reads: %v -> %v", selectedIDs, ids)
				}
				if selected.rows == 4 {
					selectedIDs = ids
				}
			}
			assertRetainedRequestWork(t, retained, selected.rows, sample, response.Body.Len(), counts, logs)
		}
	}
	return selectedIDs
}

func assertRetainedRequestWork(t *testing.T, retained, rows, sample, responseBytes int, counts *retainedReadCounts, logs *observer.ObservedLogs) {
	t.Helper()
	visits := counts.dispatchVisits()
	for _, entry := range logs.FilterMessage("worker session observation candidate selection").All() {
		visits += int(entry.ContextMap()["candidate_visits"].(int64))
	}
	sortRows, comparisons := counts.sortWork()
	for _, entry := range logs.FilterMessage("worker session observation sorting").All() {
		sortRows += int(entry.ContextMap()["sort_rows"].(int64))
		comparisons += int(entry.ContextMap()["sort_comparisons"].(int64))
	}
	// Four selected sorts (live identity/output and recorded merge/output)
	// must never sort unrelated retained rows. The comparison allowance
	// covers stable sort's small-input insertion passes and merge passes.
	if sortRows != 4*rows || comparisons > 8*rows*bits.Len(uint(rows)) || (rows > 1 && comparisons == 0) {
		t.Fatalf("N=%d k=%d: sorted rows=%d comparisons=%d", retained, rows, sortRows, comparisons)
	}
	summaries, full := counts.summaries.Load(), counts.full.Load()
	if full != 0 || counts.history.Load() != 0 || summaries > int64(2*rows) || visits > 8*rows+8 {
		t.Fatalf("N=%d k=%d: full=%d history=%d summaries=%d visits=%d", retained, rows, full, counts.history.Load(), summaries, visits)
	}
	if rows == 0 && summaries != 0 {
		t.Fatalf("empty Work read %d summaries", summaries)
	}
	t.Logf("N=%d k=%d sample=%d full=%d history=%d summaries=%d visits=%d sorted_rows=%d comparisons=%d bytes=%d", retained, rows, sample, full, counts.history.Load(), summaries, visits, sortRows, comparisons, responseBytes)
}

// Enumerate committed capture identities independently of station counts and
// the scoped list. This setup observation precedes counter reset and never
// warms the Work-scoped route being measured.
func assertRetainedInventory(t *testing.T, ctx context.Context, store recordings.WorkerRecordingStore, sessionID string, expected int) {
	t.Helper()
	seen := make(map[string]bool, expected)
	request := recordings.WorkerCapturedCatalogRequest{Limit: 1000, RequireCompleteMembership: true}
	for {
		page, err := store.ListWorkerSessionCaptures(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			// Host bootstrap owns a separate default Factory Session in the
			// same profile; it is outside this explicit-session capacity rig.
			if item.Catalog.FactorySessionID != sessionID {
				continue
			}
			id := item.Catalog.WorkerSessionID
			if id == "" || seen[id] || item.Catalog.CommittedPosition == 0 {
				t.Fatalf("invalid retained identity: %#v", item.Catalog)
			}
			seen[id] = true
		}
		if page.NextToken == "" {
			break
		}
		if page.NextToken == request.NextToken {
			t.Fatal("capture inventory did not advance")
		}
		request.NextToken = page.NextToken
	}
	if len(seen) != expected {
		t.Fatalf("actual retained captures=%d, expected=%d", len(seen), expected)
	}
	t.Logf("independent committed retained identities=%d scope=%s", len(seen), sessionID)
}

type retainedReadCounts struct {
	recordings.WorkerRecordingStore
	summaries, full, history      atomic.Int64
	mu                            sync.Mutex
	visits, sortRows, comparisons int
}

func (c *retainedReadCounts) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	c.summaries.Add(1)
	return c.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (c *retainedReadCounts) LoadWorkerRecording(ctx context.Context, id string) (recordings.WorkerRecordingSnapshot, error) {
	c.full.Add(1)
	return c.WorkerRecordingStore.LoadWorkerRecording(ctx, id)
}

func (c *retainedReadCounts) RecoverWorkerOwners(ctx context.Context) error {
	return c.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

func (c *retainedReadCounts) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	switch metric.Name {
	case "factory_runtime.read.canonical_history", "factory_runtime.read.full_history_reduction":
		c.history.Add(1)
	case "worker_sessions.read.selected_sort":
		rows, rowErr := strconv.Atoi(metric.Labels["sort_rows"])
		comparisons, compareErr := strconv.Atoi(metric.Labels["sort_comparisons"])
		if rowErr != nil || compareErr != nil {
			c.history.Add(1)
			return
		}
		c.mu.Lock()
		c.sortRows += rows
		c.comparisons += comparisons
		c.mu.Unlock()
	case "worker_sessions.read.selected_dispatches":
		visits, err := strconv.Atoi(metric.Labels["dispatch_visits"])
		if err != nil {
			c.history.Add(1)
			return
		}
		c.mu.Lock()
		c.visits += visits
		c.mu.Unlock()
	}
}

func (c *retainedReadCounts) reset() {
	c.summaries.Store(0)
	c.full.Store(0)
	c.history.Store(0)
	c.mu.Lock()
	c.visits, c.sortRows, c.comparisons = 0, 0, 0
	c.mu.Unlock()
}

func (c *retainedReadCounts) dispatchVisits() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visits
}

func (c *retainedReadCounts) sortWork() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sortRows, c.comparisons
}

func submitRetainedWork(t *testing.T, handler http.Handler, sessionID, workType string) string {
	t.Helper()
	var submitted factoryapi.SubmitWorkResponse
	name := "retained-" + uuid.NewString()
	fleetPOST(t, handler, "/factory-sessions/"+sessionID+"/work", factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: workType, Payload: map[string]string{"title": "retained " + workType}}, http.StatusCreated, &submitted)
	if submitted.WorkId == nil {
		t.Fatal("Work absent")
	}
	return *submitted.WorkId
}

func waitRetainedWork(t *testing.T, ctx context.Context, handler http.Handler, sessionID, workID string) {
	waitRetainedWorks(t, ctx, handler, sessionID, []string{workID})
}

func waitRetainedWorks(t *testing.T, ctx context.Context, handler http.Handler, sessionID string, workIDs []string) {
	t.Helper()
	// Read the public Work inventory once per observation window, rather than
	// rebuilding its aggregate projection separately for every retained Work.
	// Polling is needed because this host seam exposes no stream subscriber.
	pending := make(map[string]bool, len(workIDs))
	for _, id := range workIDs {
		pending[id] = true
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/work?maxResults=700&includeSuperseded=true", nil).WithContext(ctx))
		var value factoryapi.ListWorkResponse
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &value) != nil {
			t.Fatalf("Work readiness: %d %s", response.Code, response.Body.String())
		}
		for _, row := range value.Results {
			if row.WorkId != nil && pending[*row.WorkId] && row.State != nil {
				if row.State.Name == "failed" {
					t.Fatalf("owned Work failed: %#v", row)
				}
				if row.State.Name == "complete" {
					delete(pending, *row.WorkId)
				}
			}
		}
		if len(pending) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Work readiness: %v %s", ctx.Err(), response.Body.String())
		case <-ticker.C:
		}
	}
}

func assertRetainedRead(t *testing.T, response *httptest.ResponseRecorder, workID string, count int) []string {
	t.Helper()
	var page factoryapi.ListWorkerSessionsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Sessions) != count {
		t.Fatalf("scoped rows: %d %s", response.Code, response.Body.String())
	}
	seen := map[string]bool{}
	ids := make([]string, 0, count)
	for index, row := range page.Sessions {
		if seen[row.WorkerSessionId] || row.WorkId == nil || *row.WorkId != workID || row.State != factoryapi.WorkerSessionObservationStateCompleted || row.TokenUsage == nil {
			t.Fatalf("incorrect selected attempt: %#v", row)
		}
		seen[row.WorkerSessionId] = true
		ids = append(ids, row.WorkerSessionId)
		if row.StartedAt == nil {
			t.Fatalf("execution-derived attempt has no start time: %#v", row)
		}
		if index > 0 {
			previous := page.Sessions[index-1]
			if retainedAttemptBefore(row, previous) {
				t.Fatalf("attempts out of chronological order: %#v then %#v", previous, row)
			}
		}
	}
	return ids
}

func retainedAttemptBefore(left, right factoryapi.WorkerSessionObservation) bool {
	if !left.StartedAt.Equal(*right.StartedAt) {
		return left.StartedAt.Before(*right.StartedAt)
	}
	if left.AttemptId != right.AttemptId {
		return left.AttemptId < right.AttemptId
	}
	return left.WorkerSessionId < right.WorkerSessionId
}

func writeRetainedFactory(t *testing.T, dir string) {
	t.Helper()
	writeFleetFactory(t, dir)
	var types, stations []any
	for _, spec := range []struct {
		name     string
		attempts int
	}{{"task", 6}, {"target", 4}, {"idle", 0}} {
		states := []any{map[string]any{"name": "ready", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}
		input := "ready"
		for index := range spec.attempts {
			output := "complete"
			if index < spec.attempts-1 {
				output = fmt.Sprintf("stage-%d", index)
				states = append(states, map[string]any{"name": output, "type": "PROCESSING"})
			}
			name := fmt.Sprintf("%s-%d", spec.name, index)
			stations = append(stations, map[string]any{"name": name, "worker": "processor", "inputs": []any{map[string]any{"workType": spec.name, "state": input}}, "outputs": []any{map[string]any{"workType": spec.name, "state": output}}, "onFailure": []any{map[string]any{"workType": spec.name, "state": "failed"}}})
			path := filepath.Join(dir, "workstations", name)
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\nComplete the Work.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			input = output
		}
		types = append(types, map[string]any{"name": spec.name, "states": states})
	}
	raw, err := json.Marshal(map[string]any{"name": "retained-scoped", "workTypes": types, "workers": []any{map[string]any{"name": "processor"}}, "workstations": stations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
