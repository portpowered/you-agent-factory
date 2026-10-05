package service

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestCapturedUsageSummaryNeverReadsProviderFiles(t *testing.T) {
	t.Parallel()
	projector := &trackingObservationProjector{}
	r := newObservationRegistry(projector, nil)
	r.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	r.observations["worker-1"] = observationMetadata()
	liveTokens := 999
	r.observations["worker-1"].tokenUsage = &workersessions.TokenUsage{TotalTokens: &liveTokens}
	r.publications["worker-1"] = &publication{recordingID: "recording"}
	r.recording = replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: "recording",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "sibling", Records: []events.Record{{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"totalTokens":888}}`)}}},
			{WorkerSessionID: "worker-1", Records: []events.Record{
				{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"totalTokens":12}}`)},
				{Payload: []byte(`{"kind":"USAGE","phase":"COMPLETED","payload":{"totalTokens":777}}`)},
			}},
		},
	}}
	req := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}
	got, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || got.TokenUsage == nil || got.TokenUsage.TotalTokens == nil || *got.TokenUsage.TotalTokens != 12 || got.TokenUsage.InputTokens == nil || *got.TokenUsage.InputTokens != 0 {
		t.Fatalf("summary lost committed usage or explicit zero: %+v, %v", got.TokenUsage, err)
	}
	*got.TokenUsage.TotalTokens = 321
	again, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || *again.TokenUsage.TotalTokens != 12 || projector.calls != 0 {
		t.Fatalf("summary aliases usage or reads native provider: %+v calls=%d err=%v", again.TokenUsage, projector.calls, err)
	}
	r.recording = replayCaptureReader{}
	unknown, err := r.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || unknown.WorkerSessionID != "worker-1" || unknown.TokenUsage != nil || projector.calls != 0 {
		t.Fatalf("missing capture invented usage or hid identity: %+v calls=%d err=%v", unknown, projector.calls, err)
	}
}
