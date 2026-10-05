package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	l1Active              = 100
	l1Archives            = 10000
	l1ArchivePayloadBytes = 500 * 1024
	l1DiskBudget          = 5 << 30
	l1Window              = 10 * time.Second
	l1SeedConcurrency     = 64
)

// L1 is a dedicated real-storage workload, including under -short. The normal
// stress target excludes it; test-worker-sessions-l1 owns explicit execution.
// One isolated canonical host owns 100 Direct scripts and 10,000 compatible
// archived captures. Sixty-four seed writers bound preparation concurrency; no
// filesystem substitute, provider files, executable builds or paid calls.
func TestL1FleetDiscoveryAndCapture(t *testing.T) {
	runL1Fleet(t, l1Archives, l1ArchivePayloadBytes)
}

// This bounded probe exercises the same 100 live streams and measurement
// observers with a tiny retained corpus. It verifies harness/runtime operation,
// never the full archived-size L1 envelope or WSV-A12 acceptance.
func TestCaptureCapacityProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("explicit bounded capacity harness probe")
	}
	runL1Fleet(t, 16, 512)
}

func runL1Fleet(t *testing.T, archiveCount, payloadBytes int) {
	t.Helper()
	started := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Minute)
	defer cancel()
	dir := prepareEvictionFactory(t)
	runner := &l1Script{entered: make(chan *l1Stream, l1Active), release: make(chan struct{}), finish: make(chan struct{})}
	baseURL, store := startL1Host(t, ctx, dir, runner)
	t.Logf("environment OS=%s arch=%s Go=%s CPUs=%d profile=%s archives=%d archivePayloadBytes=%d diskBudget=%d", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), dir, archiveCount, payloadBytes, l1DiskBudget)
	seedL1Archives(t, ctx, store, archiveCount, payloadBytes)
	assertL1ArchiveMembership(t, ctx, baseURL, archiveCount)
	streams := admitL1Streams(t, ctx, baseURL, runner)
	for index := range streams {
		waitEvictionCapture(t, ctx, baseURL, fmt.Sprintf("l1-active-%03d", index), 2)
	}
	close(runner.release)
	measureL1Capture(t, ctx, baseURL, streams)
	close(runner.finish)
	for index, stream := range streams {
		stream.mu.Lock()
		position := int64(len(stream.emitted) + 3)
		stream.mu.Unlock()
		page := waitEvictionCapture(t, ctx, baseURL, fmt.Sprintf("l1-active-%03d", index), position)
		if page.Health != "COMPLETE" {
			t.Errorf("terminal capture health=%s", page.Health)
		}
	}
	var diskBytes int64
	err := filepath.WalkDir(filepath.Join(dir, ".you-agent-factory", "worker-recordings"), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err == nil {
			diskBytes += info.Size()
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("achieved active=%d archived=%d retainedBytes=%d measuredRun=%s", len(streams), archiveCount, diskBytes, time.Since(started))
	if diskBytes > l1DiskBudget {
		t.Errorf("retained bytes %d exceeds %d", diskBytes, l1DiskBudget)
	}
}

func seedL1Archives(t *testing.T, ctx context.Context, store recordings.WorkerRecordingStore, archiveCount, payloadBytes int) {
	t.Helper()
	// Exactly one large source-native message per archive. This exercises actual
	// persisted journals and indexed payloads, not sparse padding files.
	message, err := json.Marshal(workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: string(bytes.Repeat([]byte("x"), payloadBytes))}}})
	if err != nil {
		t.Fatal(err)
	}
	jobs := make(chan int)
	errors := make(chan error, l1SeedConcurrency)
	var group sync.WaitGroup
	for range l1SeedConcurrency {
		group.Go(func() {
			for index := range jobs {
				id := fmt.Sprintf("l1-archive-%05d", index)
				for position := uint64(1); position <= 3; position++ {
					record := l1ArchivedRecord(id, position, message)
					// A Factory recording contains many distinct Worker Sessions.
					// Model that shape so simultaneous admissions use the production
					// grouped sync path; each archive still has its own full payload,
					// identity, terminal capture and publicly discoverable history.
					record.RecordingID = fmt.Sprintf("l1-recording-%03d", index/l1SeedConcurrency)
					if err := store.PersistWorkerRecord(ctx, record); err != nil {
						errors <- err
						return
					}
				}
			}
		})
	}
	for index := range archiveCount {
		select {
		case jobs <- index:
		case err := <-errors:
			close(jobs)
			group.Wait()
			t.Fatal(err)
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			t.Fatal(ctx.Err())
		}
		if index%1000 == 0 {
			t.Logf("archive seed admitted=%d", index)
		}
	}
	close(jobs)
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	t.Logf("archive seed committed=%d sourceMessageBytes=%d", archiveCount, len(message))
}

func l1ArchivedRecord(id string, position uint64, message json.RawMessage) recordings.WorkerRecordingRecord {
	kind, phase := workers.KindSession, workers.PhaseStarted
	payload, _ := json.Marshal(workers.SessionPayload{Status: "STARTING", WorkerSessionID: id, AttemptID: id})
	if position == 2 {
		kind, phase, payload = workers.KindMessage, workers.PhaseCompleted, message
	}
	if position == 3 {
		phase = workers.PhaseCompleted
		payload = json.RawMessage(`{"status":"COMPLETED"}`)
	}
	draft, _ := json.Marshal(workers.Draft{Kind: kind, Phase: phase, DispatchID: id, Payload: payload,
		Provenance: workers.Provenance{Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly, NativeEventType: "l1_fixture", Representation: workers.RepresentationNotification}})
	sourceEventID := events.SourceEventID(fmt.Sprint(position))
	sourceSequence := events.SourceSequence(position)
	if position == 1 {
		sourceEventID = "started"
	}
	if position == 3 {
		sourceEventID = "terminal"
		sourceSequence = 2
	}
	return recordings.WorkerRecordingRecord{RecordingID: id, WorkerSessionID: id, Record: events.Record{
		ID:         events.RecordID{Topic: events.Topic("worker-session/" + id + "/events"), Position: events.AggregateSequence(position)},
		SourceType: "worker_session_lifecycle", SourceID: events.SourceID(id), SourceSequence: sourceSequence, SourceEventID: sourceEventID, SchemaID: "workers.draft.v1", Payload: draft}}
}

func assertL1ArchiveMembership(t *testing.T, ctx context.Context, baseURL string, archiveCount int) {
	t.Helper()
	endpoint := baseURL + "/worker-sessions?history=archived&maxResults=1000"
	seen := make(map[string]bool, archiveCount)
	for {
		body := fleetProfileHTTP(t, ctx, http.MethodGet, endpoint, nil)
		var page factoryapi.ListWorkerSessionsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Sessions {
			if seen[row.WorkerSessionId] {
				t.Fatalf("duplicate archive %s", row.WorkerSessionId)
			}
			seen[row.WorkerSessionId] = true
		}
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || *page.PaginationContext.NextToken == "" {
			break
		}
		endpoint = baseURL + "/worker-sessions?history=archived&maxResults=1000&nextToken=" + url.QueryEscape(*page.PaginationContext.NextToken)
	}
	if len(seen) != archiveCount {
		t.Fatalf("archives=%d want=%d", len(seen), archiveCount)
	}
}

func admitL1Streams(t *testing.T, ctx context.Context, baseURL string, runner *l1Script) []*l1Stream {
	t.Helper()
	streams := make([]*l1Stream, l1Active)
	worker := "worker"
	for index := range streams {
		id := fmt.Sprintf("l1-active-%03d", index)
		fleetProfileHTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", factoryapi.WorkerSessionStartRequest{
			RequestId: id, WorkerSessionId: id, Execution: factoryapi.WorkerSessionResolvedExecution{WorkstationName: "process", WorkerType: &worker,
				Dispatch: factoryapi.WorkerSessionResolvedDispatch{DispatchId: id, WorkstationName: "process", WorkerType: &worker}}})
		select {
		case streams[index] = <-runner.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	return streams
}

type l1Samples []time.Duration

func (samples l1Samples) report(t *testing.T, name string, ceiling time.Duration) {
	t.Helper()
	if len(samples) == 0 {
		t.Errorf("%s: no samples", name)
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p95 := samples[int(math.Ceil(float64(len(samples))*0.95))-1]
	t.Logf("%s n=%d min=%s median=%s max=%s p95=%s ceiling=%s", name, len(samples), samples[0], samples[len(samples)/2], samples[len(samples)-1], p95, ceiling)
	if p95 > ceiling {
		t.Errorf("%s p95=%s exceeds %s", name, p95, ceiling)
	}
}
