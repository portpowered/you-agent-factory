package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type catalogScanGate struct {
	platformreplay.Local
	once    sync.Once
	entered chan struct{}
	release chan struct{}
	scans   atomic.Int32
}

func (gate *catalogScanGate) ScanDirectory(path string, size int, visit func([]os.DirEntry) error) error {
	gate.scans.Add(1)
	gate.once.Do(func() { close(gate.entered); <-gate.release })
	return gate.Local.ScanDirectory(path, size, visit)
}

// Observe evaluation of the waiter's cancellation arm, rather than sleeping
// or inspecting the writer's gate. The context remains owned by this caller.
type catalogWaitContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
}

func (ctx *catalogWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.entered) })
	return ctx.Context.Done()
}

func TestFileWriterCatalogScanCancellation(t *testing.T) {
	t.Parallel()
	for _, recovery := range []bool{false, true} {
		for _, cancelLeader := range []bool{false, true} {
			t.Run(fmt.Sprintf("recovery=%t/cancelLeader=%t", recovery, cancelLeader), func(t *testing.T) {
				t.Parallel()
				assertCatalogScanCancellation(t, recovery, cancelLeader)
			})
		}
	}
}

func assertCatalogScanCancellation(t *testing.T, recovery, cancelLeader bool) {
	t.Helper()
	ctx, finish := context.WithTimeout(t.Context(), 10*time.Second)
	defer finish()
	local := platformreplay.NewLocal(runtime.GOOS)
	seed := journalWriter(t, local)
	persistCatalogSummaryFixture(t, seed, journalRecord(t, "scan-cancellation", "summary-worker"))
	gate := &catalogScanGate{Local: local, entered: make(chan struct{}), release: make(chan struct{})}
	store, err := NewFileWriter(gate, gate, gate, &captureTimeProbe{}, seed.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	writer := store.(*FileWriter)
	release := sync.OnceFunc(func() { close(gate.release) })
	var calls sync.WaitGroup
	t.Cleanup(func() { release(); calls.Wait() })
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leader := make(chan error, 1)
	calls.Add(1)
	go func() {
		defer calls.Done()
		if recovery {
			leader <- writer.RecoverWorkerOwners(leaderCtx)
		} else {
			_, err := writer.ListWorkerSessionCaptures(leaderCtx, recordings.WorkerCapturedCatalogRequest{})
			leader <- err
		}
	}()
	waitCatalogSignal(t, ctx, gate.entered)
	waitCtx, cancelWaiter := context.WithCancel(ctx)
	defer cancelWaiter()
	waiterCtx := &catalogWaitContext{Context: waitCtx, entered: make(chan struct{})}
	waiter := make(chan error, 1)
	calls.Add(1)
	go func() {
		defer calls.Done()
		_, err := writer.ListWorkerSessionCaptures(waiterCtx, recordings.WorkerCapturedCatalogRequest{})
		waiter <- err
	}()
	waitCatalogSignal(t, ctx, waiterCtx.entered)
	if cancelLeader {
		cancel()
		release()
		assertCatalogCallResult(t, ctx, leader, context.Canceled)
		assertCatalogCallResult(t, ctx, waiter, nil)
	} else {
		cancelWaiter()
		assertCatalogCallResult(t, ctx, waiter, context.Canceled)
		// The canceled waiter leaves while the leader is still blocked in IO.
		release()
		assertCatalogCallResult(t, ctx, leader, nil)
	}
	assertCatalogSummaryFacts(t, mustCatalogSummary(t, writer))
	wantScans := int32(1)
	if cancelLeader {
		wantScans++ // A canceled scan cannot establish complete membership.
	}
	if scans := gate.scans.Load(); scans != wantScans {
		t.Fatalf("directory scans = %d, want %d", scans, wantScans)
	}
}

func waitCatalogSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("catalog scenario signal: %v", ctx.Err())
	}
}

func assertCatalogCallResult(t *testing.T, ctx context.Context, result <-chan error, want error) {
	t.Helper()
	select {
	case err := <-result:
		if !errors.Is(err, want) {
			t.Fatalf("catalog call error = %v, want %v", err, want)
		}
	case <-ctx.Done():
		t.Fatalf("catalog caller remained blocked: %v", ctx.Err())
	}
}

type groupedAppendProbe struct {
	platformreplay.Local
	fault      string
	calls      int
	beforeSync func()
}

func (probe *groupedAppendProbe) AppendFile(path string, data []byte) error {
	probe.calls++
	if probe.beforeSync != nil {
		probe.beforeSync()
	}
	if probe.fault == "before-sync" {
		return errors.New("injected append failure")
	}
	if err := probe.Local.AppendFile(path, data); err != nil {
		return err
	}
	if probe.fault == "after-sync" {
		return errors.New("injected uncertain close")
	}
	return nil
}

func TestFileWriterCapturedGroupAdmissionAndUncertainRetry(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"", "before-sync", "after-sync"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			probe := &groupedAppendProbe{Local: local}
			writer := journalWriter(t, probe)
			opening := journalRecord(t, "grouped", "grouped-worker")
			if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
				t.Fatal(err)
			}
			requests := groupedCaptureRequests(t, opening)
			probe.fault = fault
			probe.beforeSync = func() {
				catalog, err := writer.LookupWorkerSessionCapture(t.Context(), opening.WorkerSessionID)
				if err != nil || catalog.CommittedPosition != 1 {
					t.Errorf("advertised unsynced group: %+v %v", catalog, err)
				}
			}
			entry := writer.entry(opening.RecordingID)
			entry.pending = append([]*pendingWorkerRecord(nil), requests...)
			entry.mu.Lock()
			writer.persistPendingRecords(t.Context(), entry, opening.RecordingID)
			entry.mu.Unlock()
			assertCapturedGroupResults(t, probe.calls, requests, fault)
			probe.fault, probe.beforeSync = "", nil
			for _, index := range []int{0, 4} {
				if err := writer.PersistWorkerRecord(t.Context(), requests[index].record); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := writer.LoadWorkerRecording(t.Context(), opening.RecordingID)
			if err != nil || len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Records) != 3 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete {
				t.Fatalf("retry lost prefix or duplicated group: %+v %v", snapshot, err)
			}
		})
	}
}

func assertCapturedGroupResults(t *testing.T, calls int, requests []*pendingWorkerRecord, fault string) {
	t.Helper()
	if calls != 2 {
		t.Fatalf("appends=%d want opening plus one grouped sync", calls)
	}
	if !errors.Is(requests[2].err, recordings.ErrWorkerRecordingDuplicate) || !errors.Is(requests[3].err, recordings.ErrWorkerRecordingOrder) {
		t.Fatal("invalid siblings admitted")
	}
	if requests[5].err != nil || !errors.Is(requests[6].err, context.Canceled) {
		t.Fatal("committed duplicate or canceled sibling changed grouped outcome")
	}
	for _, index := range []int{0, 1, 4} {
		if !requests[index].complete || (requests[index].err != nil) != (fault != "") {
			t.Fatalf("request %d: %+v", index, requests[index])
		}
	}
}

func groupedCaptureRequests(t *testing.T, opening recordings.WorkerRecordingRecord) []*pendingWorkerRecord {
	t.Helper()
	output := opening
	output.Record = mustRecord(t, workerOutputAppend(opening.Record.ID.Topic, opening.WorkerSessionID, 1, "output"), 2)
	conflict := output
	conflict.Record = output.Record.Detached()
	conflict.Record.Payload = []byte(`{"changed":true}`)
	skipped := opening
	skipped.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, opening.WorkerSessionID), 4)
	terminal := opening
	terminal.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, opening.WorkerSessionID), 3)
	requests := make([]*pendingWorkerRecord, 0, 7)
	for _, record := range []recordings.WorkerRecordingRecord{output, output, conflict, skipped, terminal, opening} {
		requests = append(requests, &pendingWorkerRecord{ctx: t.Context(), record: record})
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	requests = append(requests, &pendingWorkerRecord{ctx: canceled, record: terminal})
	return requests
}

func TestFileWriterCapturedSnapshotsPreserveReducedHealthAndDetachedHistory(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	persistSharedCapturedHealth(t, writer)
	first, err := writer.LoadWorkerRecording(t.Context(), "shared-health")
	if err != nil || len(first.Sessions) != 3 {
		t.Fatalf("shared snapshot=%+v error=%v", first, err)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, reader := range []recordings.WorkerRecordingReader{writer, reopened} {
		assertSharedCapturedHealth(t, reader, first)
	}
}

func persistSharedCapturedHealth(t *testing.T, writer *FileWriter) {
	t.Helper()
	for _, id := range []string{"incomplete", "complete", "degraded"} {
		record := journalRecord(t, "shared-health", id)
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
		record.Record = mustRecord(t, workerOutputAppend(record.Record.ID.Topic, id, 1, "captured-content"), 2)
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
		switch id {
		case "complete":
			record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, id), 3)
			if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
				t.Fatal(err)
			}
		case "degraded":
			if err := writer.PersistWorkerRecordingFailure(t.Context(), recordings.WorkerRecordingFailure{
				RecordingID: record.RecordingID, WorkerSessionID: id, Topic: record.Record.ID.Topic, Code: "PERSISTENCE_FAILED",
				ExecutionTerminal: &recordings.WorkerRecordingTerminal{Position: 3, Phase: workers.PhaseCompleted, Status: "COMPLETED"},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func assertSharedCapturedHealth(t *testing.T, reader recordings.WorkerRecordingReader, first recordings.WorkerRecordingSnapshot) {
	t.Helper()
	got, err := reader.LoadWorkerRecording(t.Context(), "shared-health")
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("recovered snapshot differs: %+v error=%v", got, err)
	}
	for index := range got.Sessions {
		session := &got.Sessions[index]
		projection, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: got, WorkerSessionID: session.WorkerSessionID})
		if err != nil || projection.Projection.Status != session.Status || projection.Projection.LastPosition != session.LastPosition || !reflect.DeepEqual(projection.Projection.Records, session.Records) {
			t.Fatalf("snapshot/replay disagree for %s: %+v error=%v", session.WorkerSessionID, projection, err)
		}
	}
	for index := range got.Sessions {
		session := &got.Sessions[index]
		session.Records[0].Payload[0] = '!'
		session.CapturedAt["1"] = time.Time{}
		if session.ExecutionTerminal != nil {
			session.ExecutionTerminal.Status = "mutated"
		}
	}
	again, err := reader.LoadWorkerRecording(t.Context(), "shared-health")
	if err != nil || !reflect.DeepEqual(again, first) {
		t.Fatal("caller mutation changed committed snapshot")
	}
}

func TestFileWriterCapturedUsageKeepsFrozenHeadAcrossRestart(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	record := journalRecord(t, "usage-head", "summary-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID, Limit: 1}
	initial, err := writer.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil || initial.TokenUsage != nil {
		t.Fatalf("opening invented usage: %+v, %v", initial, err)
	}
	persistUsageHeadRecords(t, writer, record, 2, []string{
		`{"totalTokens":12,"model":"first"}`, `{"label":"working"}`,
	})
	frozen, err := writer.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil || frozen.NextToken == "" {
		t.Fatalf("freeze prefix: %+v, %v", frozen, err)
	}
	persistUsageHeadRecords(t, writer, record, 4, []string{
		`{"origin":"SYNTHETIC","inputTokens":0,"totalTokens":0}`, `{"model":"latest"}`, `{"label":"working"}`,
	})
	record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, record.WorkerSessionID), 7)
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []recordings.WorkerRecordingStore{writer, reopened} {
		assertCapturedUsageHead(t, store, request, frozen.NextToken)
	}
}

func persistUsageHeadRecords(t *testing.T, writer *FileWriter, record recordings.WorkerRecordingRecord, start uint64, payloads []string) {
	t.Helper()
	for index, payload := range payloads {
		kind := workers.KindUsage
		if strings.Contains(payload, "label") {
			kind = workers.KindProgress
		}
		record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, kind, payload, start+uint64(index))
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
		// Replayed admission must not add a second usage entry or event.
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
}

func assertCapturedUsageHead(t *testing.T, writer recordings.WorkerRecordingStore, request recordings.WorkerCapturedActivityRequest, token string) {
	t.Helper()
	request.NextToken = token
	for position := uint64(2); position <= 3; position++ {
		page, err := writer.ReadWorkerCapturedActivity(t.Context(), request)
		if err != nil || page.Catalog.CommittedPosition != 3 || len(page.Records) != 1 || uint64(page.Records[0].Record.ID.Position) != position || page.TokenUsage == nil {
			t.Fatalf("frozen page %d: %+v, %v", position, page, err)
		}
		if page.TokenUsage.TotalTokens != 12 || page.TokenUsage.Model != "first" || page.TokenUsage.Origin != "" || page.Terminal != nil {
			t.Fatalf("later commit leaked into frozen usage: %+v", page)
		}
		page.TokenUsage.TotalTokens = 999
		request.NextToken = page.NextToken
	}
	page, err := writer.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil || page.Catalog.CommittedPosition != 7 || page.TokenUsage == nil || page.TokenUsage.Origin != "SYNTHETIC" || page.TokenUsage.Model != "latest" || page.TokenUsage.TotalTokens != 0 {
		t.Fatalf("refreshed usage lost model-only/zero update: %+v, %v", page, err)
	}
}

func TestFileWriterCapturedActiveLossPreservesIncompletePrefix(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	record := journalRecord(t, "active-loss-recording", "active-loss-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID}
	prefix, err := writer.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.PersistWorkerRecordingFailure(t.Context(), recordings.WorkerRecordingFailure{
		RecordingID: record.RecordingID, WorkerSessionID: record.WorkerSessionID, Topic: record.Record.ID.Topic, Code: "PERSISTENCE_FAILED",
	}); err != nil {
		t.Fatal(err)
	}
	reader, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	for index, current := range []recordings.WorkerCapturedActivityReader{writer, reader} {
		page, err := current.ReadWorkerCapturedActivity(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if page.Health != recordings.WorkerRecordingStatusIncomplete || page.HealthReason != "PERSISTENCE_FAILED" || page.Terminal != nil || page.NextToken == "" {
			t.Fatalf("capture loss hid its health or fabricated terminal: %+v", page)
		}
		if page.OwnerLost != (index == 1) {
			t.Fatalf("owner loss requires a changed known host epoch: %+v", page)
		}
		if page.Catalog.CommittedPosition != prefix.Catalog.CommittedPosition || !reflect.DeepEqual(page.Records, prefix.Records) {
			t.Fatal("capture loss changed committed records/time/watermark")
		}
	}
}

type captureTimeProbe struct {
	now   time.Time
	calls int
}

func TestFileWriterCapturedSuccessorSurvivesRestartWithoutSourceRewrite(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	source := lineageOpeningRecord(t, "source", "", "", "")
	if err := writer.PersistWorkerRecord(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	source.Record = mustRecord(t, terminalAppend(source.Record.ID.Topic, "source"), 2)
	if err := writer.PersistWorkerRecord(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	before, err := local.ReadFile(writer.path(source.RecordingID) + "l")
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedSuccessor(t, writer, "")
	// A foreign Factory scope and an unrelated attempt cannot enrich source.
	for _, record := range []recordings.WorkerRecordingRecord{
		lineageOpeningRecord(t, "foreign", "factory-other", "source", ""),
		lineageOpeningRecord(t, "other-attempt", "", "source", "unrelated-attempt"),
	} {
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	assertCapturedSuccessor(t, writer, "")
	successor := lineageOpeningRecord(t, "successor", "", "source", "source-attempt")
	if err := writer.PersistWorkerRecord(t.Context(), successor); err != nil {
		t.Fatal(err)
	}
	assertCapturedSuccessor(t, writer, "successor")
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedSuccessor(t, reopened, "successor")
	after, err := local.ReadFile(writer.path(source.RecordingID) + "l")
	if err != nil || string(before) != string(after) {
		t.Fatal("successor admission or recovery rewrote the terminal source journal")
	}
}

func TestFileWriterCapturedSuccessorRejectsAmbiguousAdmission(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	for _, record := range []recordings.WorkerRecordingRecord{
		lineageOpeningRecord(t, "source", "", "", ""),
		lineageOpeningRecord(t, "one", "", "source", "source-attempt"),
		lineageOpeningRecord(t, "two", "", "source", "source-attempt"),
	} {
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	_, err := writer.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "source"})
	if !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("ambiguous successor selected an arbitrary link: %v", err)
	}
	if _, err := writer.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "one"}); err != nil {
		t.Fatalf("ambiguous source hid healthy selected sibling: %v", err)
	}
}

func TestFileWriterCapturedSuccessorWaitsForCommittedOpening(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS), entered: make(chan struct{}), release: make(chan struct{})}
	writer := journalWriter(t, probe)
	source := lineageOpeningRecord(t, "source", "", "", "")
	if err := writer.PersistWorkerRecord(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	// Complete reconstruction before admission so source reads use the index
	// while the successor's independent append barrier is held.
	assertCapturedSuccessor(t, writer, "")
	successor := lineageOpeningRecord(t, "successor", "", "source", "source-attempt")
	probe.gatePath = writer.path(successor.RecordingID) + "l"
	var release sync.Once
	unblock := func() { release.Do(func() { close(probe.release) }) }
	t.Cleanup(unblock)
	done := make(chan error, 1)
	go func() { done <- writer.PersistWorkerRecord(t.Context(), successor) }()
	select {
	case <-probe.entered:
	case <-t.Context().Done():
		t.Fatal("successor did not reach the append boundary")
	}
	assertCapturedSuccessor(t, writer, "")
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	assertCapturedSuccessor(t, writer, "successor")
}

func lineageOpeningRecord(t *testing.T, id, factory, predecessor, previousAttempt string) recordings.WorkerRecordingRecord {
	t.Helper()
	record := journalRecord(t, "recording-"+id, id)
	var draft workers.Draft
	if err := json.Unmarshal(record.Record.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	opening := workers.SessionPayload{WorkerSessionID: id, FactorySessionID: factory, DispatchID: id + "-attempt", AttemptID: id + "-attempt", Status: "STARTING"}
	if predecessor != "" {
		if previousAttempt == "" {
			previousAttempt = predecessor + "-attempt"
		}
		opening.AttemptReason = workers.AttemptReasonResume
		opening.Continuation = &workers.SessionContinuation{Provider: "codex", Kind: "session_id", ID: "controlled"}
		opening.Lineage = &workers.SessionLineage{PredecessorWorkerSessionID: predecessor, PreviousDispatchID: previousAttempt, PreviousAttemptID: previousAttempt}
	}
	var err error
	draft.Payload, err = json.Marshal(opening)
	if err != nil {
		t.Fatal(err)
	}
	record.Record.Payload, err = json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func assertCapturedSuccessor(t *testing.T, reader recordings.WorkerCapturedActivityReader, want string) {
	t.Helper()
	page, err := reader.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "source"})
	if err != nil || page.SuccessorWorkerSessionID != want {
		t.Fatalf("selected captured successor=%q want=%q error=%v", page.SuccessorWorkerSessionID, want, err)
	}
	items, err := reader.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Items {
		if item.Catalog.WorkerSessionID == "source" && item.SuccessorWorkerSessionID != want {
			t.Fatalf("catalog successor=%q want=%q", item.SuccessorWorkerSessionID, want)
		}
	}
}

func TestFileWriterCatalogSummaryKeepsLatestFactsAcrossRestart(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	record := journalRecord(t, "summary-recording", "summary-worker")
	persistCatalogSummaryFixture(t, writer, record)
	want := mustCatalogSummary(t, writer)
	assertCatalogSummaryFacts(t, want)
	probe.mu.Lock()
	reads := probe.reads
	probe.mu.Unlock()
	// Returned payloads, stamps and terminal facts are caller-owned snapshots.
	want.Opening.Payload[0] = '!'
	want.MetadataRecords[0].Payload[0] = '!'
	want.CapturedAt["1"] = time.Time{}
	want.Terminal.Status = "FAILED"
	again := mustCatalogSummary(t, writer)
	assertCatalogSummaryFacts(t, again)
	probe.mu.Lock()
	if probe.reads != reads {
		t.Errorf("cached summary reloaded logs: reads %d -> %d", reads, probe.reads)
	}
	probe.mu.Unlock()
	reopened, err := NewFileWriter(probe, probe, probe, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustCatalogSummary(t, reopened); !reflect.DeepEqual(got, again) {
		t.Fatalf("restarted summary changed committed facts: got=%+v want=%+v", got, again)
	}
}

func persistCatalogSummaryFixture(t *testing.T, writer recordings.WorkerRecordingStore, record recordings.WorkerRecordingRecord) {
	t.Helper()
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	for i, cell := range []struct {
		kind    workers.Kind
		payload string
	}{
		{workers.KindSession, `{"providerSelection":{"runnerId":"codex"},"model":"initial","reasoningEffort":"high"}`},
		{workers.KindSession, `{"workerSessionId":"summary-worker","dispatchId":"attempt","attemptId":"attempt","attemptReason":"RESUME","continuation":{"provider":"codex","kind":"session_id","id":"opaque"},"lineage":{"predecessorWorkerSessionId":"prior","previousDispatchId":"prior-attempt","previousAttemptId":"prior-attempt"}}`},
		{workers.KindSession, `{"continuation":{"provider":"codex","kind":"session_id","id":"opaque"}}`},
		{workers.KindUsage, `{"model":"observed","totalTokens":12}`},
		{workers.KindUsage, `{"inputTokens":0,"totalTokens":0}`},
		{workers.KindSession, `{"title":"latest display title"}`},
		{workers.KindProgress, `{"label":"working"}`},
		{workers.KindSession, `{"status":"RUNNING"}`},
	} {
		record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, cell.kind, cell.payload, uint64(i+2))
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	record.Record = mustRecord(t, workerOutputAppend(record.Record.ID.Topic, record.WorkerSessionID, 1, "output"), 10)
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, record.WorkerSessionID), 11)
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
}

func catalogMetadataRecord(t *testing.T, topic events.Topic, kind workers.Kind, payload string, position uint64) events.Record {
	t.Helper()
	request := openingAppend(topic, "summary-worker")
	var draft workers.Draft
	if err := json.Unmarshal(request.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Kind, draft.Phase, draft.Payload = kind, workers.PhaseUpdated, json.RawMessage(payload)
	var err error
	request.Payload, err = json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceID = "summary-facts"
	request.SourceSequence = events.SourceSequence(position)
	request.SourceEventID = events.SourceEventID(strconv.FormatUint(position, 10))
	return mustRecord(t, request, events.AggregateSequence(position))
}

func TestFileWriterCatalogSummaryModelOnlyUsageKeepsCounters(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	record := journalRecord(t, "model-updates", "summary-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	for index, payload := range []string{`{"inputTokens":0,"model":"initial"}`, `{"model":"later","inputTokens":null}`} {
		record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindUsage, payload, uint64(index+2))
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	want := mustCatalogSummary(t, writer)
	if len(want.MetadataRecords) != 2 || want.MetadataRecords[0].ID.Position != 2 || want.MetadataRecords[1].ID.Position != 3 {
		t.Fatalf("model-only update erased captured counters: %+v", want.MetadataRecords)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustCatalogSummary(t, reopened); !reflect.DeepEqual(got.MetadataRecords, want.MetadataRecords) {
		t.Fatalf("recovery lost usage/model facts: %+v", got.MetadataRecords)
	}
}

func mustCatalogSummary(t *testing.T, reader recordings.WorkerCapturedActivityReader) recordings.WorkerCapturedCatalogItem {
	t.Helper()
	page, err := reader.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("summary page = %+v, %v", page, err)
	}
	return page.Items[0]
}

func TestFileWriterRepeatedCatalogSummariesRemainDetachedAcrossCommits(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	record := journalRecord(t, "summary", "worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindUsage, `{"totalTokens":10}`, 2)
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	frozen := mustCatalogSummary(t, writer)
	assertConcurrentSummaryDetachment(t, writer, frozen)
	if got := mustCatalogSummary(t, writer); !reflect.DeepEqual(got, frozen) {
		t.Fatalf("reader mutation escaped into a later page: %+v", got)
	}
	record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindUsage, `{"totalTokens":20}`, 3)
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	updated := mustCatalogSummary(t, writer)
	if updated.Catalog.CommittedPosition != 3 || len(updated.MetadataRecords) != 1 || updated.MetadataRecords[0].ID.Position != 3 || frozen.MetadataRecords[0].ID.Position != 2 {
		t.Fatalf("commit failed to refresh selection or changed a frozen page: %+v %+v", updated, frozen)
	}
	if err := writer.PersistWorkerRecordingFailure(t.Context(), recordings.WorkerRecordingFailure{
		RecordingID: record.RecordingID, WorkerSessionID: record.WorkerSessionID, Topic: record.Record.ID.Topic, Code: "PERSISTENCE_FAILED",
		ExecutionTerminal: &recordings.WorkerRecordingTerminal{Position: 4, Phase: workers.PhaseCompleted, Status: "COMPLETED"},
	}); err != nil {
		t.Fatal(err)
	}
	failed := mustCatalogSummary(t, writer)
	if failed.Terminal == nil || failed.Terminal.Position != 4 || failed.Health != recordings.WorkerRecordingStatusDegraded || updated.Terminal != nil {
		t.Fatalf("failure commit left a stale summary: %+v", failed)
	}
	failed.Terminal.Status = "caller mutation"
	if got := mustCatalogSummary(t, writer); got.Terminal.Status != "COMPLETED" {
		t.Fatalf("terminal pointer escaped into later pages: %+v", got)
	}
}

func assertConcurrentSummaryDetachment(t *testing.T, writer recordings.WorkerCapturedActivityReader, frozen recordings.WorkerCapturedCatalogItem) {
	t.Helper()
	// Concurrent readers own their returned payloads and timestamp maps. Their
	// mutations must not alter the retained selection or each other's pages.
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			page, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
			if err != nil || len(page.Items) != 1 {
				t.Errorf("concurrent summary: %+v %v", page, err)
				return
			}
			item := page.Items[0]
			if !reflect.DeepEqual(item, frozen) {
				t.Errorf("reader received changed summary: %+v", item)
			}
			item.Opening.Payload[0] = '!'
			item.MetadataRecords[0].Payload[0] = '!'
			item.CapturedAt["1"] = time.Time{}
		})
	}
	readers.Wait()
}

func assertCatalogSummaryFacts(t *testing.T, item recordings.WorkerCapturedCatalogItem) {
	t.Helper()
	var positions []uint64
	for _, record := range item.MetadataRecords {
		positions = append(positions, uint64(record.ID.Position))
		if item.CapturedAt[strconv.FormatUint(uint64(record.ID.Position), 10)].IsZero() {
			t.Fatalf("selected fact lacks its recorded stamp: %+v", record)
		}
	}
	if !reflect.DeepEqual(positions, []uint64{2, 3, 4, 5, 6, 9, 11}) || len(item.CapturedAt) != 8 || item.CapturedAt["1"].IsZero() {
		t.Fatalf("summary lost partial facts or included output/history stamps: positions=%v stamps=%v", positions, item.CapturedAt)
	}
	if item.Catalog.CommittedPosition != 11 || item.Terminal == nil || item.Terminal.Status != "COMPLETED" || item.Health != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("summary lost terminal truth: %+v", item)
	}
	var draft workers.Draft
	if err := json.Unmarshal(item.MetadataRecords[4].Payload, &draft); err != nil {
		t.Fatal(err)
	}
	var usage map[string]int
	if err := json.Unmarshal(draft.Payload, &usage); err != nil {
		t.Fatal(err)
	}
	if value, present := usage["inputTokens"]; !present || value != 0 || len(usage) != 2 {
		t.Fatalf("latest usage erased explicit zero or invented absent fields: %s", draft.Payload)
	}
}

func TestFileWriterCatalogSummaryDistinguishesPrefixAndUncapturedTerminal(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name             string
		legacy, terminal bool
	}{
		{name: "incomplete"},
		{name: "legacy", legacy: true},
		{name: "uncaptured-terminal", terminal: true},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			got := restartedPrefixSummary(t, cell.legacy, cell.terminal)
			if got.OwnerLost != (!cell.legacy && !cell.terminal) {
				t.Fatalf("owner loss must require a known prior epoch and unfinished capture: %+v", got.Catalog)
			}
			wantStamps := 1
			if cell.legacy {
				wantStamps = 0
			}
			if len(got.MetadataRecords) != 0 || got.Catalog.CommittedPosition != 1 || len(got.CapturedAt) != wantStamps {
				t.Fatalf("prefix invented metadata, stamps or watermark: %+v", got)
			}
			if cell.terminal {
				if got.Terminal == nil || got.Terminal.Position != 2 || got.Health != recordings.WorkerRecordingStatusDegraded || got.HealthReason != "PERSISTENCE_FAILED" {
					t.Fatalf("uncaptured authoritative terminal lost its degraded health: %+v", got)
				}
			} else if got.Terminal != nil || got.Health != recordings.WorkerRecordingStatusIncomplete {
				t.Fatalf("prefix masquerades as complete or ended: %+v", got)
			}
		})
	}
}

func restartedPrefixSummary(t *testing.T, legacy, terminal bool) recordings.WorkerCapturedCatalogItem {
	t.Helper()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	opening := journalRecord(t, "legacy", "legacy-session")
	if legacy {
		if err := local.WriteFile(writer.path("legacy"), []byte(legacyWorkerOpeningFixture)); err != nil {
			t.Fatal(err)
		}
	} else if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
		t.Fatal(err)
	}
	if terminal {
		if err := writer.PersistWorkerRecordingFailure(t.Context(), recordings.WorkerRecordingFailure{
			RecordingID: "legacy", WorkerSessionID: "legacy-session", Topic: opening.Record.ID.Topic, Code: "PERSISTENCE_FAILED",
			ExecutionTerminal: &recordings.WorkerRecordingTerminal{Position: 2, Phase: workers.PhaseCompleted, Status: "COMPLETED"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
	if err != nil {
		t.Fatal(err)
	}
	return mustCatalogSummary(t, reopened)
}

type catalogAppendProbe struct {
	platformreplay.Local
	fault error
}

func (probe *catalogAppendProbe) AppendFile(path string, data []byte) error {
	if probe.fault != nil {
		return probe.fault
	}
	return probe.Local.AppendFile(path, data)
}

func TestFileWriterCatalogSummaryExcludesRejectedAppend(t *testing.T) {
	t.Parallel()
	probe := &catalogAppendProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	record := journalRecord(t, "rejected", "summary-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	want := mustCatalogSummary(t, writer)
	record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindUsage, `{"totalTokens":10}`, 2)
	probe.fault = errors.New("selected append unavailable")
	if err := writer.PersistWorkerRecord(t.Context(), record); !errors.Is(err, probe.fault) {
		t.Fatalf("rejected append = %v", err)
	}
	if got := mustCatalogSummary(t, writer); !reflect.DeepEqual(got, want) {
		t.Fatalf("summary advertised uncommitted metadata: got=%+v want=%+v", got, want)
	}
	assertCapturedUsageTokens(t, writer, record.WorkerSessionID, nil)
	probe.fault = nil
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	got := mustCatalogSummary(t, writer)
	if got.Catalog.CommittedPosition != 2 || len(got.MetadataRecords) != 1 || len(got.CapturedAt) != 2 {
		t.Fatalf("accepted retry failed to publish metadata: %+v", got)
	}
	assertCapturedUsageTokens(t, writer, record.WorkerSessionID, &workers.UsagePayload{TotalTokens: 10})
}

func TestFileWriterCatalogSummaryRehydratesUncertainCommit(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	record := journalRecord(t, "uncertain", "summary-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	_ = mustCatalogSummary(t, writer)
	probe.failAfterSync = true
	record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindUsage, `{"totalTokens":10}`, 2)
	if err := writer.PersistWorkerRecord(t.Context(), record); err == nil {
		t.Fatal("uncertain close did not return its error")
	}
	got := mustCatalogSummary(t, writer)
	if got.Catalog.CommittedPosition != 2 || len(got.MetadataRecords) != 1 || len(got.CapturedAt) != 2 {
		t.Fatalf("summary failed to recover synchronized bytes after uncertain close: %+v", got)
	}
	assertCapturedUsageTokens(t, writer, record.WorkerSessionID, &workers.UsagePayload{TotalTokens: 10})
}

func TestFileWriterCatalogSummaryKeepsPartialSelectionAndRetryFacts(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	record := journalRecord(t, "partial-selection", "summary-worker")
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	for i, payload := range []string{
		`{"providerSelection":{"runnerId":"codex","modelProvider":"openai"}}`,
		`{"providerSelection":{"executorProvider":"codex"}}`,
		`{"providerSelection":{"runnerId":"codex"}}`,
		`{"workerSessionId":"summary-worker","dispatchId":"attempt","attemptId":"attempt","attemptReason":"RETRY","lineage":{"previousDispatchId":"prior-attempt","previousAttemptId":"prior-attempt"}}`,
		`{"status":"RUNNING"}`,
	} {
		record.Record = catalogMetadataRecord(t, record.Record.ID.Topic, workers.KindSession, payload, uint64(i+2))
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	got := mustCatalogSummary(t, writer)
	var positions []uint64
	for _, metadata := range got.MetadataRecords {
		positions = append(positions, uint64(metadata.ID.Position))
	}
	if !reflect.DeepEqual(positions, []uint64{2, 3, 4, 5, 6}) || got.Catalog.CommittedPosition != 6 || got.Terminal != nil {
		t.Fatalf("partial updates erased known selection or retry facts: %+v", got)
	}
}

func TestFileWriterCatalogSummaryRebuildsLegacyFactsWithoutStamps(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	source := journalWriter(t, local)
	record := journalRecord(t, "summary-recording", "summary-worker")
	persistCatalogSummaryFixture(t, source, record)
	want := mustCatalogSummary(t, source)
	snapshot, err := source.LoadWorkerRecording(t.Context(), record.RecordingID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Sessions[0].CapturedAt = nil
	snapshot.Sessions[0].RecordingGenerationID = ""
	snapshot.Sessions[0].OwnerEpoch = ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	legacy := journalWriter(t, local)
	if err := local.WriteFile(legacy.path(record.RecordingID), data); err != nil {
		t.Fatal(err)
	}
	got := mustCatalogSummary(t, legacy)
	if len(got.CapturedAt) != 0 || got.Catalog.OwnerEpoch != "historical" || got.Catalog.RecordingGenerationID == "" {
		t.Fatalf("legacy summary fabricated commit metadata: %+v", got)
	}
	if !reflect.DeepEqual(got.Opening, want.Opening) || !reflect.DeepEqual(got.MetadataRecords, want.MetadataRecords) ||
		!reflect.DeepEqual(got.Terminal, want.Terminal) || got.Health != want.Health {
		t.Fatalf("legacy summary lost source facts: got=%+v want=%+v", got, want)
	}
	assertCapturedUsageTokens(t, legacy, record.WorkerSessionID, &workers.UsagePayload{})
}

func assertCapturedUsageTokens(t *testing.T, store recordings.WorkerRecordingStore, id string, want *workers.UsagePayload) {
	t.Helper()
	page, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: id, Limit: 1})
	if err != nil || !reflect.DeepEqual(page.TokenUsage, want) {
		t.Fatalf("captured usage = %+v, want %+v; error %v", page.TokenUsage, want, err)
	}
}

func TestFileWriterCatalogEnumerationSurvivesRestart(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	for _, id := range []string{"z", "a", "m"} {
		if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "recording-"+id, id)); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	token := ""
	for {
		page, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{Limit: 1, NextToken: token})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			entry := item.Catalog
			ids = append(ids, entry.WorkerSessionID)
			if entry.OwnerEpoch == "reopened" || entry.CommittedPosition != 1 {
				t.Fatalf("invented restarted ownership or watermark: %+v", entry)
			}
		}
		token = page.NextToken
		if token == "" {
			break
		}
	}
	if !reflect.DeepEqual(ids, []string{"a", "m", "z"}) {
		t.Fatalf("catalog IDs = %v", ids)
	}
}

func TestFileWriterCatalogPagesObserveCommitsWithoutChangingMembership(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	for _, id := range []string{"a", "b"} {
		if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, id, id)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{Limit: 1})
	if err != nil || first.NextToken == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	terminal := recordings.WorkerRecordingRecord{
		RecordingID: "b", WorkerSessionID: "b",
		Record: mustRecord(t, terminalAppend(events.Topic("worker-session/b/events"), "b"), 2),
	}
	if err := writer.PersistWorkerRecord(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	for range 16 {
		readers.Go(func() {
			assertCatalogCommittedContinuation(t, writer, first)
		})
	}
	readers.Wait()
	if first.Items[0].Catalog.CommittedPosition != 1 || first.Items[0].Terminal != nil {
		t.Fatalf("first page mutated: %+v", first)
	}
}

func assertCatalogCommittedContinuation(t *testing.T, writer *FileWriter, first recordings.WorkerCapturedCatalogPage) {
	t.Helper()
	page, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{Limit: 1, NextToken: first.NextToken})
	if err != nil || len(page.Items) != 1 {
		t.Errorf("continued page = %+v, %v", page, err)
		return
	}
	item := page.Items[0]
	if page.GenerationID != first.GenerationID || page.NextToken != "" || item.Catalog.WorkerSessionID != "b" ||
		item.Catalog.CommittedPosition != 2 || item.Terminal == nil || item.Terminal.Status != "COMPLETED" {
		t.Errorf("continued page lost committed terminal: %+v", page)
	}
	// Returned facts belong to this reader; mutating them cannot affect peers.
	if item.Terminal != nil {
		item.Terminal.Status = "changed by caller"
	}
}

func TestFileWriterCatalogEnumerationFencesMembershipAndProfile(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	for _, id := range []string{"a", "b"} {
		if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, id, id)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{Limit: 1})
	if err != nil || first.NextToken == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	other := journalWriter(t, local)
	for _, id := range []string{"a", "b"} {
		if err := other.PersistWorkerRecord(t.Context(), journalRecord(t, id, id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := other.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{NextToken: first.NextToken}); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
		t.Fatalf("cross-profile cursor: %v", err)
	}
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "c", "c")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{NextToken: first.NextToken}); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
		t.Fatalf("changed membership cursor: %v", err)
	}
	for _, req := range []recordings.WorkerCapturedCatalogRequest{{Limit: -1}, {Limit: 1001}, {NextToken: "bad"}} {
		if _, err := writer.ListWorkerSessionCaptures(t.Context(), req); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
			t.Fatalf("invalid request %+v: %v", req, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := writer.ListWorkerSessionCaptures(ctx, recordings.WorkerCapturedCatalogRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enumeration: %v", err)
	}
}

func TestFileWriterCatalogCollisionNeverSelectsOneCapture(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	for _, id := range []string{"first", "second"} {
		if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, id, "collision")); err != nil {
			t.Fatal(err)
		}
		if id == "first" {
			page, err := writer.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("initial catalog = %+v, %v", page, err)
			}
		}
	}
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "healthy", "healthy")); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []recordings.WorkerCapturedActivityReader{writer, reopened} {
		if err := store.(*FileWriter).RecoverWorkerOwners(t.Context()); err != nil {
			t.Fatal(err)
		}
		page, err := store.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(page.Items) != 0 {
			t.Fatalf("ambiguous enumeration = %+v, %v", page, err)
		}
		if _, err := store.LookupWorkerSessionCapture(t.Context(), "collision"); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
			t.Fatalf("ambiguous lookup = %v", err)
		}
		if _, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "collision"}); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
			t.Fatalf("ambiguous logs = %v", err)
		}
		if page, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "healthy"}); err != nil || len(page.Records) != 1 {
			t.Fatalf("collision erased healthy history: %+v, %v", page, err)
		}
	}
}

func TestFileWriterCatalogKeepsDamagedCaptureUnavailable(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	for _, id := range []string{"healthy", "malformed", "torn"} {
		record := journalRecord(t, id, id+"-worker")
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	for id, tail := range map[string]string{"malformed": "not json\n", "torn": "{\"uncommitted\":"} {
		if err := local.AppendFile(writer.path(id)+"l", []byte(tail)); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	assertDamagedCapturedCatalog(t, reopened)
}

func TestFileWriterTornCatalogCannotProveAssociations(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "torn", "torn-worker")); err != nil {
		t.Fatal(err)
	}
	if err := local.AppendFile(writer.path("torn")+"l", []byte(`{"uncommitted":`)); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		catalog, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(catalog.Items) != 0 {
			t.Fatalf("torn membership presented as complete: %+v, %v", catalog, err)
		}
	}
	page, err := reopened.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "torn-worker"})
	if err != nil || len(page.Records) != 1 || page.HealthReason != "PERSISTENCE_FAILED" {
		t.Fatalf("catalog failure erased readable Worker prefix: %+v, %v", page, err)
	}
}

func assertDamagedCapturedCatalog(t *testing.T, reopened recordings.WorkerRecordingStore) {
	t.Helper()
	if err := reopened.(*FileWriter).RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("default enumeration lost healthy histories beside damage: %+v, %v", page, err)
	}
	for _, item := range page.Items {
		if item.Catalog.WorkerSessionID != "healthy-worker" && item.Catalog.WorkerSessionID != "torn-worker" {
			t.Fatalf("default enumeration exposed damaged identity: %+v", item)
		}
	}
	catalog, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
	if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(catalog.Items) != 0 {
		t.Fatalf("damaged enumeration claimed complete membership: %+v %v", catalog, err)
	}
	for range 2 {
		page, err := reopened.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "malformed-worker"})
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(page.Records) != 0 || page.Catalog.CommittedPosition != 0 {
			t.Fatalf("damaged capture fabricated absence or accepted state: page=%+v error=%v", page, err)
		}
	}
	for _, id := range []string{"healthy-worker", "torn-worker"} {
		page, err := reopened.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: id})
		if err != nil || len(page.Records) != 1 || page.Catalog.CommittedPosition != 1 || page.Health != recordings.WorkerRecordingStatusIncomplete {
			t.Fatalf("committed prefix lost or claimed complete: page=%+v error=%v", page, err)
		}
		if id == "torn-worker" && page.HealthReason != "PERSISTENCE_FAILED" {
			t.Fatalf("torn prefix hid capture failure: %+v", page)
		}
	}
	if _, err := reopened.LookupWorkerSessionCapture(t.Context(), "unknown-worker"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown identity error=%v", err)
	}
}

type catalogScanProbe struct {
	local     platformreplay.Local
	fault     error
	calls     int
	afterScan func()
}

func (scan *catalogScanProbe) ScanDirectory(path string, batchSize int, visit func([]os.DirEntry) error) error {
	scan.calls++
	if scan.fault != nil {
		return scan.fault
	}
	err := scan.local.ScanDirectory(path, batchSize, visit)
	if scan.afterScan != nil {
		scan.afterScan()
	}
	return err
}

type catalogReadProbe struct {
	platformreplay.Local
	fault error
}

func (probe *catalogReadProbe) ReadFile(path string) ([]byte, error) {
	if probe.fault != nil {
		return nil, probe.fault
	}
	return probe.Local.ReadFile(path)
}

func TestFileWriterCatalogReadFailureDoesNotCacheAbsence(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	original := journalWriter(t, local)
	record := journalRecord(t, "unreadable-recording", "unreadable-worker")
	if err := original.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	storage := &catalogReadProbe{Local: local, fault: errors.New("private-path sentinel-secret")}
	reader, err := NewFileWriter(storage, local, local, &captureTimeProbe{}, original.root, "retry-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID}
	for range 2 {
		catalog, err := reader.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || strings.Contains(err.Error(), "sentinel-secret") || len(catalog.Items) != 0 {
			t.Fatalf("unreadable catalog claimed absence or disclosed error: page=%+v error=%v", catalog, err)
		}
		page, err := reader.ReadWorkerCapturedActivity(t.Context(), request)
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || strings.Contains(err.Error(), "sentinel-secret") || len(page.Records) != 0 {
			t.Fatalf("unreadable capture became absent or disclosed error: page=%+v error=%v", page, err)
		}
	}
	storage.fault = nil
	page, err := reader.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil || len(page.Records) != 1 || page.Catalog.CommittedPosition != 1 {
		t.Fatalf("read recovery did not retry catalog: page=%+v error=%v", page, err)
	}
}

func TestFileWriterCatalogCanceledScanDoesNotCacheAbsence(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scan := &catalogScanProbe{local: local, afterScan: cancel}
	reader, err := NewFileWriter(local, local, scan, &captureTimeProbe{}, root, "observer-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.LookupWorkerSessionCapture(ctx, "later-worker"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan returned %v", err)
	}
	// A different store writes only after the canceled read has returned. The
	// next observer must reconstruct, rather than inherit permanent absence.
	writer, err := NewFileWriter(local, local, local, &captureTimeProbe{}, root, "writer-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "later-recording", "later-worker")); err != nil {
		t.Fatal(err)
	}
	scan.afterScan = nil
	page, err := reader.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "later-worker"})
	if err != nil || len(page.Records) != 1 || scan.calls != 2 {
		t.Fatalf("canceled observation poisoned retry: page=%+v scans=%d error=%v", page, scan.calls, err)
	}
}

func TestFileWriterCatalogScanRetriesFailureAndCachesLookup(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	clock := &captureTimeProbe{now: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)}
	original, err := NewFileWriter(local, local, local, clock, root, "original", nil)
	if err != nil {
		t.Fatal(err)
	}
	record := journalRecord(t, "scan-recording", "scan-worker")
	if err := original.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fault := errors.New("selected filesystem unavailable")
	scan := &catalogScanProbe{local: local, fault: fault}
	reopened, err := NewFileWriter(local, local, scan, clock, root, "reopened", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID}
	if _, err := reopened.ReadWorkerCapturedActivity(t.Context(), request); !errors.Is(err, fault) {
		t.Fatalf("selected scanner failure = %v, want %v", err, fault)
	}
	scan.fault = nil
	page, err := reopened.ReadWorkerCapturedActivity(t.Context(), request)
	if err != nil || len(page.Records) != 1 || scan.calls != 2 {
		t.Fatalf("retry page=%+v scans=%d error=%v", page, scan.calls, err)
	}
	scan.fault = fault
	if _, err := reopened.ReadWorkerCapturedActivity(t.Context(), request); err != nil || scan.calls != 2 {
		t.Fatalf("cached lookup rescanned fleet: scans=%d error=%v", scan.calls, err)
	}
}

func (clock *captureTimeProbe) Now() time.Time {
	clock.calls++
	return clock.now.Add(time.Duration(clock.calls) * time.Second)
}

func TestFileWriterCapturedAtAndCatalogSurviveReopening(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	clock := &captureTimeProbe{now: time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)}
	store, err := NewFileWriter(local, local, local, clock, root, "owner-one", nil)
	if err != nil {
		t.Fatal(err)
	}
	record := journalRecord(t, "capture-time", "capture-worker")
	if err := store.PersistWorkerRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistWorkerRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	if clock.calls != 1 {
		t.Fatalf("duplicate sampled %d commit times, want 1", clock.calls)
	}
	snapshot, err := store.LoadWorkerRecording(ctx, record.RecordingID)
	if err != nil {
		t.Fatal(err)
	}
	first := snapshot.Sessions[0]
	want := clock.now.Add(time.Second)
	assertCapturedMetadata(t, first, want)
	first.CapturedAt["1"] = time.Time{}
	// The cache is disposable and cannot manufacture committed records.
	if err := os.RemoveAll(filepath.Join(root, "catalog")); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileWriter(local, local, local, clock, root, "owner-two", nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := reopened.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].CapturedAt == nil || !page.Records[0].CapturedAt.Equal(want) || page.Catalog.RecordingGenerationID != snapshot.Sessions[0].RecordingGenerationID || page.Health != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("reopened page does not preserve detached capture metadata and historical health: %+v", page.Catalog)
	}
}

func assertCapturedMetadata(t *testing.T, snapshot recordings.WorkerSessionRecordingSnapshot, want time.Time) {
	t.Helper()
	if !snapshot.CapturedAt["1"].Equal(want) || snapshot.RecordingGenerationID == "" || snapshot.OwnerEpoch != "owner-one" {
		t.Fatalf("capture metadata not retained: generation=%s epoch=%s time=%v", snapshot.RecordingGenerationID, snapshot.OwnerEpoch, snapshot.CapturedAt)
	}
}

func TestFileWriterCapturedPagesPinHeadAndFenceCursors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	record := journalRecord(t, "page", "page-worker")
	output := mustRecord(t, workerOutputAppend(record.Record.ID.Topic, record.WorkerSessionID, 1, "captured"), 2)
	persistWorkerRecoveryPrefix(t, writer, record.RecordingID, record.WorkerSessionID, record.Record, output)
	page, err := writer.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Catalog.CommittedPosition != 2 || len(page.Records) != 1 || page.NextToken == "" {
		t.Fatalf("first page: %+v", page.Catalog)
	}
	terminal := mustRecord(t, terminalAppend(record.Record.ID.Topic, record.WorkerSessionID), 3)
	persistWorkerRecoveryPrefix(t, writer, record.RecordingID, record.WorkerSessionID, terminal)
	continued, err := writer.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID, Limit: 1, NextToken: page.NextToken})
	if err != nil {
		t.Fatal(err)
	}
	if continued.Catalog.CommittedPosition != 2 || len(continued.Records) != 1 || continued.Records[0].Record.ID.Position != 2 || continued.NextToken == "" {
		t.Fatal("continuation advanced beyond the original committed head")
	}
	assertCapturedResumeTerminal(t, writer, record.WorkerSessionID, continued.NextToken)
	other := journalRecord(t, "other-page", "other-worker")
	if err := writer.PersistWorkerRecord(ctx, other); err != nil {
		t.Fatal(err)
	}
	for _, req := range []recordings.WorkerCapturedActivityRequest{
		{WorkerSessionID: record.WorkerSessionID, Limit: -1},
		{WorkerSessionID: record.WorkerSessionID, Limit: 1001},
		{WorkerSessionID: record.WorkerSessionID, NextToken: "invalid"},
		{WorkerSessionID: other.WorkerSessionID, NextToken: page.NextToken},
	} {
		if _, err := writer.ReadWorkerCapturedActivity(ctx, req); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
			t.Fatalf("invalid cursor/limit accepted: %v", err)
		}
	}
	otherProfile := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	if err := otherProfile.PersistWorkerRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := otherProfile.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID, NextToken: page.NextToken}); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
		t.Fatalf("cross-profile token: %v", err)
	}
}

func assertCapturedResumeTerminal(t *testing.T, writer *FileWriter, id, token string) {
	t.Helper()
	resumed, err := writer.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: id, NextToken: token})
	if err != nil || resumed.Catalog.CommittedPosition != 3 || len(resumed.Records) != 1 || resumed.Records[0].Record.ID.Position != 3 || resumed.NextToken != "" {
		t.Fatalf("at-head continuation lost later terminal: %+v error=%v", resumed, err)
	}
}

func TestFileWriterCatalogDiscoversLegacyAndPreservesHealthySibling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	record := journalRecord(t, "legacy-index", "legacy-index-worker")
	legacy := recordings.WorkerRecordingSnapshot{RecordingID: record.RecordingID, Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: record.WorkerSessionID, Records: []events.Record{record.Record}}}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.WriteFile(writer.path(record.RecordingID), data); err != nil {
		t.Fatal(err)
	}
	if err := local.AppendFile(writer.path("corrupt-index")+"l", []byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	page, err := writer.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID})
	if err != nil {
		t.Fatal(err)
	}
	if page.Catalog.OwnerEpoch != "historical" || page.Catalog.RecordingGenerationID == "" || len(page.Records) != 1 || page.Records[0].CapturedAt != nil {
		t.Fatal("legacy capture lost or assigned invented commit time/ownership")
	}
	reopened, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := reopened.LookupWorkerSessionCapture(ctx, record.WorkerSessionID)
	if err != nil || entry.RecordingGenerationID != page.Catalog.RecordingGenerationID {
		t.Fatalf("legacy generation changed on reconstruction: %v", err)
	}
	for range 2 {
		catalog, err := reopened.ListWorkerSessionCaptures(ctx, recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
		if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(catalog.Items) != 0 {
			t.Fatalf("unidentified damage claimed complete catalog: %+v, %v", catalog, err)
		}
	}
}
