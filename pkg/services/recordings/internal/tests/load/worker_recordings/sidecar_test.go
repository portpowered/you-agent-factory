package worker_recordings_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// BenchmarkWorkerSidecarAppend uses the same public constructor on the base
// and changed checkout. A fixed legacy baseline excludes setup and hydration
// from the measured steady-state record N+1 append with real local fsync.
func BenchmarkWorkerSidecarAppend(b *testing.B) {
	for _, count := range []int{1, 40, 400} {
		b.Run(fmt.Sprintf("sessions=%d", count), func(b *testing.B) {
			root := b.TempDir()
			local := &benchmarkStorage{Local: platformreplay.NewLocal(runtime.GOOS)}
			snapshot := benchmarkBaseline(count)
			data, err := json.Marshal(snapshot)
			if err != nil {
				b.Fatal(err)
			}
			hash := sha256.Sum256([]byte(snapshot.RecordingID))
			path := filepath.Join(root, hex.EncodeToString(hash[:])+".worker.json")
			if err := local.WriteFile(path, data); err != nil {
				b.Fatal(err)
			}
			writer, err := recordingswire.NewWorkerRecordingFileWriter(local, local, platformclock.Real{}, root, uuid.NewString())
			if err != nil {
				b.Fatal(err)
			}
			started := time.Now()
			if _, err := writer.(recordings.WorkerRecordingReader).LoadWorkerRecording(context.Background(), snapshot.RecordingID); err != nil {
				b.Fatal(err)
			}
			hydration := time.Since(started)
			local.written = 0
			samples := make([]int64, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				record := recordings.WorkerRecordingRecord{RecordingID: snapshot.RecordingID, WorkerSessionID: "worker-0", Record: benchmarkOutput("worker-0", 17+i)}
				started := time.Now()
				if err := writer.PersistWorkerRecord(context.Background(), record); err != nil {
					b.Fatal(err)
				}
				samples[i] = time.Since(started).Nanoseconds()
			}
			b.StopTimer()
			b.ReportMetric(float64(local.written)/float64(b.N), "written-bytes/op")
			b.ReportMetric(float64(len(data)), "baseline-bytes")
			b.ReportMetric(float64(hydration.Nanoseconds()), "hydrate-ns")
			b.Logf("OS=%s real-fsync baseline-bytes=%d hydrate=%s append samples ns=%v", runtime.GOOS, len(data), hydration, samples)
		})
	}
}
func benchmarkBaseline(count int) recordings.WorkerRecordingSnapshot {
	snapshot := recordings.WorkerRecordingSnapshot{RecordingID: "performance-recording"}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("worker-%d", i)
		payload, _ := json.Marshal(workers.SessionPayload{Status: "STARTING", WorkerSessionID: id})
		draft, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted,
			Provenance: workers.Provenance{Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly, NativeEventType: "worker_session_lifecycle", Representation: workers.RepresentationNotification}, Payload: payload})
		session := recordings.WorkerSessionRecordingSnapshot{WorkerSessionID: id, Records: []events.Record{{
			ID:         events.RecordID{Topic: events.Topic("worker-session/" + id + "/events"), Position: 1},
			SourceType: "worker_session_lifecycle", SourceID: events.SourceID(id), SourceSequence: 1, SourceEventID: "started", SchemaID: "workers.draft.v1", Payload: draft}}}
		for j := 1; j <= 16; j++ {
			session.Records = append(session.Records, benchmarkOutput(id, j))
		}
		snapshot.Sessions = append(snapshot.Sessions, session)
	}
	return snapshot
}
func benchmarkOutput(id string, sequence int) events.Record {
	payload, _ := json.Marshal(workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: strings.Repeat("x", 1800)}}})
	draft, _ := json.Marshal(workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Provenance: workers.Provenance{Delivery: workers.DeliveryNativeFinal, Fidelity: workers.FidelityFinalOnly, NativeEventType: "message.completed", Provider: "codex", Representation: workers.RepresentationSnapshot}, Payload: payload, DispatchID: id})
	return events.Record{ID: events.RecordID{Topic: events.Topic("worker-session/" + id + "/events"), Position: events.AggregateSequence(sequence + 1)}, SourceType: "worker_provider", SourceID: events.SourceID(id + "/provider"), SourceSequence: events.SourceSequence(sequence), SourceEventID: events.SourceEventID(fmt.Sprintf("message-%d", sequence)), SchemaID: "workers.draft.v1", Payload: draft}
}

// The storage boundary counts actual bytes submitted to durable I/O, for
// both the original whole-file writer and the append-only implementation.
type benchmarkStorage struct {
	platformreplay.Local
	written int64
}

func (storage *benchmarkStorage) WriteFile(path string, data []byte) error {
	storage.written += int64(len(data))
	return storage.Local.WriteFile(path, data)
}
func (storage *benchmarkStorage) AppendFile(path string, data []byte) error {
	storage.written += int64(len(data))
	return storage.Local.AppendFile(path, data)
}
