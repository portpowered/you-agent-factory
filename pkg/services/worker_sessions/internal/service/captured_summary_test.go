package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type capturedSummaryFake struct {
	capturedActivityFake
	snapshot recordings.WorkerRecordingSnapshot
}

func (f *capturedSummaryFake) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return f.snapshot, nil
}

func TestCapturedArchivedSummaryPreservesKnownFacts(t *testing.T) {
	t.Parallel()
	fake := &capturedSummaryFake{capturedActivityFake: capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
		Catalog:  recordings.WorkerSessionCatalogEntry{WorkerSessionID: "worker", RecordingID: "recording", CommittedPosition: 3},
		Opening:  events.Record{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker","attemptId":"attempt","workIds":["work"],"model":"model","reasoningEffort":"high","startedAt":"2026-10-05T00:00:00Z"}}`)},
		Terminal: &recordings.WorkerRecordingTerminal{Position: 3, Status: "COMPLETED"},
		Health:   recordings.WorkerRecordingStatusComplete, TokenUsage: &workers.UsagePayload{TotalTokens: 12},
	}}, snapshot: recordings.WorkerRecordingSnapshot{RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{
		{WorkerSessionID: "sibling", Records: []events.Record{{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"totalTokens":999}}`)}}},
		{WorkerSessionID: "worker", Records: []events.Record{
			{ID: events.RecordID{Position: 2}, Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"totalTokens":12}}`)},
			{ID: events.RecordID{Position: 4}, Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"totalTokens":777}}`)},
		}},
	}}}
	reader := &LogReader{reader: fake}
	req := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"}
	got, err := reader.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || got.Validate() != nil || !got.Direct || got.State != workersessions.StateCompleted || got.StartedAt == nil {
		t.Fatalf("archived summary lost identity: %+v %v", got, err)
	}
	assertArchivedUnknownFacts(t, got)
	if got.TokenUsage == nil || *got.TokenUsage.TotalTokens != 12 || got.TokenUsage.InputTokens == nil || *got.TokenUsage.InputTokens != 0 {
		t.Fatalf("archived summary lost known facts or invented unknown ones: %+v %v", got, err)
	}
	*got.Model = "mutated"
	got.WorkIDs[0] = "mutated"
	*got.TokenUsage.TotalTokens = 321
	again, err := reader.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || *again.Model != "model" || again.WorkIDs[0] != "work" || *again.TokenUsage.TotalTokens != 12 || fake.request.Limit != 1 {
		t.Fatalf("archived summary aliases data or uses an unbounded page: %+v %v", again, err)
	}
}

func assertArchivedUnknownFacts(t *testing.T, got workersessions.Observation) {
	t.Helper()
	if got.EndedAt != nil || got.Duration != nil || got.ProviderSessionAvailable || got.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		t.Fatalf("archived summary invented unknown facts: %+v", got)
	}
}

func TestCapturedArchivedSummaryDoesNotInferLiveState(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name         string
		page         recordings.WorkerCapturedActivityPage
		source, want error
	}{
		{name: "prefix", page: recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: "worker"}}, want: workersessions.ErrObservationProjectionUnavailable},
		{name: "missing", source: os.ErrNotExist, want: workersessions.ErrObservationSessionNotFound},
		{name: "storage", source: errors.New("secret path"), want: workersessions.ErrObservationProjectionUnavailable},
		{name: "canceled", source: context.Canceled, want: context.Canceled},
		{name: "sibling", page: recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: "sibling"}, Terminal: &recordings.WorkerRecordingTerminal{Status: "COMPLETED"}}, want: workersessions.ErrObservationProjectionUnavailable},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			reader := &LogReader{reader: &capturedActivityFake{page: cell.page, err: cell.source}}
			got, err := reader.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker"})
			if !errors.Is(err, cell.want) || got.WorkerSessionID != "" {
				t.Fatalf("unsafe archived result: %+v %v, want %v", got, err, cell.want)
			}
		})
	}
}

func TestCapturedArchivedSummaryMatchesIncompleteHistory(t *testing.T) {
	t.Parallel()
	item := historyCapture(t, "worker", "factory", "attempt", false)
	reader := &LogReader{reader: &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
		Catalog: item.Catalog, Opening: item.Opening, Health: item.Health, OwnerLost: item.OwnerLost,
	}}}
	got, err := reader.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "factory"})
	want, wantErr := capturedHistoryIdentity(item, nil)
	if err != nil || wantErr != nil || got.State != want.State || got.ConfirmationState != want.ConfirmationState || got.Failure == nil || got.Failure.Kind != workersessions.FailureCauseProcessGone || got.RecordingHealth != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("selected incomplete capture differs from history: %+v %v", got, err)
	}
	assertArchivedUnknownFacts(t, got)
	_, err = reader.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "other"})
	if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("foreign Factory scope: %v", err)
	}
}

func TestCapturedArchivedSummaryPersistsOwnerLossCause(t *testing.T) {
	t.Parallel()
	item := historyCapture(t, "worker", "factory", "attempt", false)
	item.Terminal = &recordings.WorkerRecordingTerminal{Phase: workers.PhaseFailed, Status: "FAILED"}
	item.HealthReason = "OWNER_LOST"
	reader := &LogReader{reader: &capturedSummaryFake{snapshot: recordings.WorkerRecordingSnapshot{RecordingID: item.Catalog.RecordingID}, capturedActivityFake: capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
		Catalog: item.Catalog, Opening: item.Opening, Terminal: item.Terminal, Health: item.Health, HealthReason: item.HealthReason,
	}}}}
	got, err := reader.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: "factory"})
	if err != nil || got.TerminalCause == nil || *got.TerminalCause != "OWNER_LOST" || got.Failure == nil || got.Failure.Kind != workersessions.FailureCauseProcessGone {
		t.Fatalf("owner loss observation=%+v error=%v", got, err)
	}
}
