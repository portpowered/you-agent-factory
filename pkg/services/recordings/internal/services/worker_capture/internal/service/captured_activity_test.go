package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	reader, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened-owner")
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
	reopened, err := NewFileWriter(probe, probe, probe, &captureTimeProbe{}, writer.root, "restarted")
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
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted")
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
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted")
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
	probe.fault = nil
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	got := mustCatalogSummary(t, writer)
	if got.Catalog.CommittedPosition != 2 || len(got.MetadataRecords) != 1 || len(got.CapturedAt) != 2 {
		t.Fatalf("accepted retry failed to publish metadata: %+v", got)
	}
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
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened")
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
	}
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, "healthy", "healthy")); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "reopened")
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []recordings.WorkerCapturedActivityReader{writer, reopened} {
		page, err := store.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
		if err != nil || len(page.Items) != 1 || page.Items[0].Catalog.WorkerSessionID != "healthy" {
			t.Fatalf("ambiguous enumeration = %+v, %v", page, err)
		}
		if _, err := store.LookupWorkerSessionCapture(t.Context(), "collision"); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
			t.Fatalf("ambiguous lookup = %v", err)
		}
		if _, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "collision"}); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
			t.Fatalf("ambiguous logs = %v", err)
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

func assertDamagedCapturedCatalog(t *testing.T, reopened recordings.WorkerRecordingStore) {
	t.Helper()
	catalog, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
	if err != nil || len(catalog.Items) != 2 {
		t.Fatalf("catalog hid a healthy sibling or invented unreadable metadata: %+v %v", catalog, err)
	}
	for _, item := range catalog.Items {
		if item.Health != recordings.WorkerRecordingStatusIncomplete || item.Terminal != nil || item.Catalog.CommittedPosition != 1 {
			t.Fatalf("damaged-prefix summary claimed terminal or uncommitted bytes: %+v", item)
		}
		if item.Catalog.WorkerSessionID == "torn-worker" && item.HealthReason != "PERSISTENCE_FAILED" {
			t.Fatalf("torn prefix hid capture failure: %+v", item)
		}
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
	reader, err := NewFileWriter(storage, local, local, &captureTimeProbe{}, original.root, "retry-owner")
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID}
	for range 2 {
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
	reader, err := NewFileWriter(local, local, scan, &captureTimeProbe{}, root, "observer-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.LookupWorkerSessionCapture(ctx, "later-worker"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan returned %v", err)
	}
	// A different store writes only after the canceled read has returned. The
	// next observer must reconstruct, rather than inherit permanent absence.
	writer, err := NewFileWriter(local, local, local, &captureTimeProbe{}, root, "writer-owner")
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
	original, err := NewFileWriter(local, local, local, clock, root, "original")
	if err != nil {
		t.Fatal(err)
	}
	record := journalRecord(t, "scan-recording", "scan-worker")
	if err := original.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fault := errors.New("selected filesystem unavailable")
	scan := &catalogScanProbe{local: local, fault: fault}
	reopened, err := NewFileWriter(local, local, scan, clock, root, "reopened")
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
	store, err := NewFileWriter(local, local, local, clock, root, "owner-one")
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
	reopened, err := NewFileWriter(local, local, local, clock, root, "owner-two")
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
}
