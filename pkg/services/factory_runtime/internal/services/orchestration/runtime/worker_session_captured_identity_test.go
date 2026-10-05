package runtime

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type forbiddenCapturedProviderProjection struct{ providersessions.Service }

func (forbiddenCapturedProviderProjection) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	panic("canonical Worker ID read consulted provider files")
}

func TestCapturedFactoryIdentityUsesOnlyCommittedUsage(t *testing.T) {
	t.Parallel()
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	service.providerSessions = forbiddenCapturedProviderProjection{}
	service.recordingID = "captured-factory"
	reader := &scriptedWorkerRecordingReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: service.recordingID,
		Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "sibling", Status: recordings.WorkerRecordingStatusComplete,
				Records: []events.Record{{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"sibling"}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":999}}`)}}},
			{WorkerSessionID: fixture.workerSessionID, Status: recordings.WorkerRecordingStatusComplete,
				Records: []events.Record{{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker-recorded-exact"}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"outputTokens":4,"cachedInputTokens":2,"reasoningOutputTokens":1}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"STARTED","payload":{"inputTokens":999}}`)}}},
		},
	}}
	service.recordingReader = reader
	req := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID}
	got, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkerSessionID != fixture.workerSessionID || got.State != workersessions.StateCompleted || !got.ProviderSessionAvailable || got.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		t.Fatalf("captured identity lost lifecycle/association or invented transcript: %+v", got)
	}
	usage := got.TokenUsage
	assertCapturedFactoryUsage(t, usage)
	*usage.OutputTokens = 999
	again, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || *again.TokenUsage.OutputTokens != 4 {
		t.Fatalf("captured usage aliases previous read: %+v %v", again, err)
	}
	reader.snapshot.Sessions = nil
	unknown, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || unknown.TokenUsage != nil {
		t.Fatalf("missing capture invented usage: %+v %v", unknown, err)
	}
	reader.err = recordings.ErrWorkerRecordingIncomplete
	prefix, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || prefix.WorkerSessionID != fixture.workerSessionID || prefix.TokenUsage != nil {
		t.Fatalf("incomplete capture hid Factory identity: %+v %v", prefix, err)
	}
}

func assertCapturedFactoryUsage(t *testing.T, usage *workersessions.TokenUsage) {
	t.Helper()
	if usage == nil {
		t.Fatal("captured usage is absent")
	}
	for _, field := range []struct {
		name string
		got  *int
		want int
	}{
		{"input", usage.InputTokens, 0},
		{"output", usage.OutputTokens, 4},
		{"cached input", usage.CachedInputTokens, 2},
		{"reasoning output", usage.ReasoningOutputTokens, 1},
	} {
		if field.got == nil || *field.got != field.want {
			t.Fatalf("captured %s usage = %v, want %d", field.name, field.got, field.want)
		}
	}
	if usage.TotalTokens != nil {
		t.Fatal("capture invented an absent total")
	}
}
