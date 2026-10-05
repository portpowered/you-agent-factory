package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type captureTimeProbe struct {
	now   time.Time
	calls int
}

type catalogScanProbe struct {
	local platformreplay.Local
	fault error
	calls int
}

func (scan *catalogScanProbe) ScanDirectory(path string, batchSize int, visit func([]os.DirEntry) error) error {
	scan.calls++
	if scan.fault != nil {
		return scan.fault
	}
	return scan.local.ScanDirectory(path, batchSize, visit)
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
	if continued.Catalog.CommittedPosition != 2 || len(continued.Records) != 1 || continued.Records[0].Record.ID.Position != 2 || continued.NextToken != "" {
		t.Fatal("continuation advanced beyond the original committed head")
	}
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
