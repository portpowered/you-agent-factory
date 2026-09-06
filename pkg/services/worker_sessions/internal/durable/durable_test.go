package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type durableHistoryStub struct {
	listFn   func(context.Context, recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error)
	loadFn   func(context.Context, string) (recordings.WorkerRecordingSnapshot, error)
	listCall []recordings.WorkerRecordingListRequest
	loadCall []string
}

func (*durableHistoryStub) StartWorkerSessionRecording(
	context.Context,
	recordings.WorkerSessionRecordingRequest,
) (recordings.WorkerSessionRecording, error) {
	return nil, nil
}

func (stub *durableHistoryStub) ListWorkerRecordingProjections(
	ctx context.Context,
	request recordings.WorkerRecordingListRequest,
) (recordings.WorkerRecordingListResult, error) {
	stub.listCall = append(stub.listCall, request)
	if stub.listFn == nil {
		return recordings.WorkerRecordingListResult{}, nil
	}
	return stub.listFn(ctx, request)
}

func (stub *durableHistoryStub) LoadWorkerRecordingByWorkerSessionID(
	ctx context.Context,
	workerSessionID string,
) (recordings.WorkerRecordingSnapshot, error) {
	stub.loadCall = append(stub.loadCall, workerSessionID)
	if stub.loadFn == nil {
		return recordings.WorkerRecordingSnapshot{}, nil
	}
	return stub.loadFn(ctx, workerSessionID)
}

type recordingServiceOnly struct{}

func (recordingServiceOnly) StartWorkerSessionRecording(
	context.Context,
	recordings.WorkerSessionRecordingRequest,
) (recordings.WorkerSessionRecording, error) {
	return nil, nil
}

func TestDurableHistoryRoutingWithoutCapability(t *testing.T) {
	ctx := context.Background()
	serviceOnly := recordingServiceOnly{}
	if got := WorkerRecordingHistoryReader(serviceOnly); got != nil {
		t.Fatalf("WorkerRecordingHistoryReader(service without history) = %#v, want nil", got)
	}
	if got := WorkerRecordingHistoryReader(nil); got != nil {
		t.Fatalf("WorkerRecordingHistoryReader(nil) = %#v, want nil", got)
	}
	if _, found, err := WorkerProjection(serviceOnly, ctx, "worker-1"); found || err != nil {
		t.Fatalf("WorkerProjection(service without history) = found %v, error %v, want false/nil", found, err)
	}
	if _, err := ListWorkerRecordingProjections(serviceOnly, ctx, recordings.WorkerRecordingListRequest{}); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("ListWorkerRecordingProjections(service without history) error = %v", err)
	}
	if _, err := LoadWorkerRecordingByWorkerSessionID(serviceOnly, ctx, "worker-1"); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("LoadWorkerRecordingByWorkerSessionID(service without history) error = %v", err)
	}
	if got, err := WorkerProjections(serviceOnly, ctx, recordings.WorkerRecordingListRequest{}); got != nil || err != nil {
		t.Fatalf("WorkerProjections(service without history) = %#v, %v, want nil/nil", got, err)
	}
}

func TestDurableHistoryForwardingAndPaging(t *testing.T) {
	ctx := context.Background()
	projection := recordings.WorkerRecordingProjection{WorkerSessionID: "worker-1"}
	stub := newDurableHistoryPagingStub(t, projection)

	listRequest := recordings.WorkerRecordingListRequest{FactorySessionID: "factory-1", MaxResults: 2}
	listResult, err := ListWorkerRecordingProjections(stub, ctx, listRequest)
	if err != nil || len(listResult.Projections) != 1 || len(stub.listCall) != 1 || stub.listCall[0] != listRequest {
		t.Fatalf("ListWorkerRecordingProjections() = %#v, %v; calls = %#v", listResult, err, stub.listCall)
	}
	loaded, err := LoadWorkerRecordingByWorkerSessionID(stub, ctx, "worker-1")
	if err != nil || len(loaded.Sessions) != 1 || stub.loadCall[0] != "worker-1" {
		t.Fatalf("LoadWorkerRecordingByWorkerSessionID() = %#v, %v; calls = %#v", loaded, err, stub.loadCall)
	}
	all, err := WorkerProjections(stub, ctx, recordings.WorkerRecordingListRequest{NextToken: ""})
	if err != nil || len(all) != 2 || all[0].WorkerSessionID != "worker-1" || all[1].WorkerSessionID != "worker-2" {
		t.Fatalf("WorkerProjections() = %#v, %v, want two ordered projections", all, err)
	}
}

func TestDurableHistoryPagingErrors(t *testing.T) {
	ctx := context.Background()
	stub := &durableHistoryStub{}
	stub.listFn = func(_ context.Context, _ recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error) {
		return recordings.WorkerRecordingListResult{NextToken: "ignored"}, nil
	}
	if got, err := WorkerProjections(stub, ctx, recordings.WorkerRecordingListRequest{}); err != nil || len(got) != 0 {
		t.Fatalf("WorkerProjections(empty page) = %#v, %v, want empty result", got, err)
	}
	stub.listFn = func(_ context.Context, _ recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error) {
		return recordings.WorkerRecordingListResult{}, recordings.ErrMissingWorkerRecordingReader
	}
	if got, err := WorkerProjections(stub, ctx, recordings.WorkerRecordingListRequest{}); got != nil || err != nil {
		t.Fatalf("WorkerProjections(missing reader) = %#v, %v, want nil/nil", got, err)
	}
	stub.listFn = func(_ context.Context, _ recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error) {
		return recordings.WorkerRecordingListResult{}, errors.New("catalog unavailable")
	}
	if got, err := WorkerProjections(stub, ctx, recordings.WorkerRecordingListRequest{}); got != nil || !errors.Is(err, workersessions.ErrObservationRecordingUnavailable) {
		t.Fatalf("WorkerProjections(unavailable) = %#v, %v", got, err)
	}
}

func newDurableHistoryPagingStub(t *testing.T, projection recordings.WorkerRecordingProjection) *durableHistoryStub {
	t.Helper()
	stub := &durableHistoryStub{}
	stub.listFn = func(_ context.Context, request recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error) {
		if request.NextToken == "" {
			return recordings.WorkerRecordingListResult{
				Projections: []recordings.WorkerRecordingProjection{projection},
				NextToken:   "page-2",
			}, nil
		}
		return recordings.WorkerRecordingListResult{
			Projections: []recordings.WorkerRecordingProjection{{WorkerSessionID: "worker-2"}},
		}, nil
	}
	stub.loadFn = func(_ context.Context, workerSessionID string) (recordings.WorkerRecordingSnapshot, error) {
		if workerSessionID == "worker-1" {
			return durableReplaySnapshot(t, workerSessionID), nil
		}
		return recordings.WorkerRecordingSnapshot{}, errors.New("unexpected Worker Session")
	}
	return stub
}

func TestDurableWorkerProjectionErrorClassification(t *testing.T) {
	ctx := context.Background()
	errorCases := []struct {
		name      string
		loadErr   error
		wantFound bool
		wantErr   error
	}{
		{name: "replay error", loadErr: recordings.ErrWorkerRecordingReplay},
		{name: "missing reader", loadErr: recordings.ErrMissingWorkerRecordingReader},
		{name: "corrupt tail", loadErr: recordings.ErrWorkerRecordingCorruptTail, wantFound: true, wantErr: workersessions.ErrObservationRecordingCorrupt},
		{name: "unsupported schema", loadErr: recordings.ErrWorkerRecordingCompatibility, wantFound: true, wantErr: workersessions.ErrObservationRecordingCorrupt},
		{name: "unavailable", loadErr: errors.New("disk offline"), wantFound: true, wantErr: workersessions.ErrObservationRecordingUnavailable},
	}
	for _, test := range errorCases {
		t.Run(test.name, func(t *testing.T) {
			stub := &durableHistoryStub{loadFn: func(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
				return recordings.WorkerRecordingSnapshot{}, test.loadErr
			}}
			_, found, err := WorkerProjection(stub, ctx, " worker-1 ")
			if found != test.wantFound || !errors.Is(err, test.wantErr) {
				t.Fatalf("WorkerProjection() = found %v, error %v; want found %v, error %v", found, err, test.wantFound, test.wantErr)
			}
		})
	}

	stub := &durableHistoryStub{loadFn: func(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
		return durableReplaySnapshot(t, "worker-1"), nil
	}}
	projection, found, err := WorkerProjection(stub, ctx, " worker-1 ")
	if err != nil || !found || projection.Status != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("WorkerProjection(success) = %#v, found %v, error %v", projection, found, err)
	}
	stub.loadFn = func(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
		return recordings.WorkerRecordingSnapshot{
			RecordingID: "recording",
			Sessions: []recordings.WorkerSessionRecordingSnapshot{{
				WorkerSessionID: "worker-1",
				Records:         []events.Record{{Payload: []byte("bad")}},
			}},
		}, nil
	}
	if _, found, err := WorkerProjection(stub, ctx, "worker-1"); !found || !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) {
		t.Fatalf("WorkerProjection(codec failure) = found %v, error %v", found, err)
	}
}

func TestDurableObservationProjectsIdentityAndUsage(t *testing.T) {
	projection := durableFullProjection(t, workersessions.StateFailed)
	observation, err := Observation(projection)
	if err != nil {
		t.Fatalf("Observation() error = %v", err)
	}
	requireDurableObservationIdentity(t, observation)
	requireDurableObservationModel(t, observation)
	requireDurableObservationUsage(t, observation)
	requireDurableObservationFailure(t, observation)
}

func TestDurableObservationStateAndErrors(t *testing.T) {
	projection := durableFullProjection(t, workersessions.StateFailed)
	state, err := WorkerState(projection)
	if err != nil || state != workersessions.StateFailed {
		t.Fatalf("WorkerState() = %q, %v, want FAILED", state, err)
	}
	stateCases := []recordings.WorkerRecordingProjection{
		{},
		{Terminal: &recordings.WorkerRecordingTerminal{Status: "RUNNING"}},
		{Terminal: &recordings.WorkerRecordingTerminal{Status: "UNKNOWN"}},
		{ExecutionTerminal: &recordings.WorkerRecordingTerminal{Status: "COMPLETED"}},
	}
	if got, err := WorkerState(stateCases[0]); err != nil || got != workersessions.StateRunning {
		t.Fatalf("WorkerState(no terminal) = %q, %v", got, err)
	}
	for _, test := range stateCases[1:3] {
		if _, err := WorkerState(test); err == nil {
			t.Fatalf("WorkerState(%#v) error = nil, want invalid terminal status", test)
		}
	}
	if got, err := WorkerState(stateCases[3]); err != nil || got != workersessions.StateCompleted {
		t.Fatalf("WorkerState(execution terminal) = %q, %v", got, err)
	}

	badAttempt := projection
	badAttempt.AttemptID = ""
	badAttempt.Records = nil
	if _, err := Observation(badAttempt); !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) {
		t.Fatalf("Observation(missing attempt) error = %v", err)
	}
	badTerminal := projection
	badTerminal.Terminal = &recordings.WorkerRecordingTerminal{Status: "RUNNING"}
	if _, err := Observation(badTerminal); !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) {
		t.Fatalf("Observation(invalid terminal) error = %v", err)
	}
	if got := terminalFailure(nil, workersessions.StateFailed); got != nil {
		t.Fatalf("terminalFailure(empty records) = %#v, want nil", got)
	}
	if got := terminalFailure(projection.Records, workersessions.StateCompleted); got != nil {
		t.Fatalf("terminalFailure(non-failed state) = %#v, want nil", got)
	}
}

func TestDurableObservationHealthAndStartTime(t *testing.T) {
	projection := durableFullProjection(t, workersessions.StateFailed)
	if startedAt := ObservationStartedAt(projection); startedAt.IsZero() {
		t.Fatal("ObservationStartedAt() = zero, want opening timestamp")
	}

	if got := recordingReason(recordings.WorkerRecordingProjection{InterruptionReason: "interrupted"}); got != "interrupted" {
		t.Fatalf("recordingReason(interruption) = %q", got)
	}
	if got := recordingReason(recordings.WorkerRecordingProjection{Degradation: "degraded", InterruptionReason: "interrupted"}); got != "degraded" {
		t.Fatalf("recordingReason(degradation) = %q", got)
	}
	if got := ObservationStartedAt(recordings.WorkerRecordingProjection{Records: []events.Record{{Payload: []byte("not-json")}}}); !got.IsZero() {
		t.Fatalf("ObservationStartedAt(invalid opening) = %v, want zero", got)
	}
	if got := ObservationStartedAt(recordings.WorkerRecordingProjection{}); !got.IsZero() {
		t.Fatalf("ObservationStartedAt(empty) = %v, want zero", got)
	}
}

func requireDurableObservationIdentity(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.WorkerSessionID != "worker-full" || observation.FactorySessionID != "factory-full" || observation.Direct {
		t.Fatalf("observation identity = %#v", observation)
	}
	if observation.PredecessorWorkerSessionID != "worker-previous" || observation.SuccessorWorkerSessionID != "worker-successor" {
		t.Fatalf("observation lineage = %#v", observation)
	}
	if observation.TurnID != "turn-full" || observation.AttemptID != "attempt-full" || observation.State != workersessions.StateFailed {
		t.Fatalf("observation lifecycle = %#v", observation)
	}
	if observation.RecordingHealth != recordings.WorkerRecordingStatusComplete || observation.RecordingHealthReason != "persisted" {
		t.Fatalf("observation health = %#v", observation)
	}
}

func requireDurableObservationModel(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.Model == nil {
		t.Fatal("observation model = nil")
	}
	if *observation.Model != "model-full" {
		t.Fatalf("observation model = %q", *observation.Model)
	}
	if observation.ReasoningEffort == nil {
		t.Fatal("observation reasoning effort = nil")
	}
	if *observation.ReasoningEffort != "high" {
		t.Fatalf("observation reasoning effort = %q", *observation.ReasoningEffort)
	}
}

func requireDurableObservationUsage(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.TokenUsage == nil {
		t.Fatal("observation token usage = nil")
	}
	requireDurableInt(t, "input", observation.TokenUsage.InputTokens, 11)
	requireDurableInt(t, "cached input", observation.TokenUsage.CachedInputTokens, 3)
	requireDurableInt(t, "output", observation.TokenUsage.OutputTokens, 7)
	requireDurableInt(t, "reasoning output", observation.TokenUsage.ReasoningOutputTokens, 5)
	requireDurableInt(t, "total", observation.TokenUsage.TotalTokens, 26)
}

func requireDurableObservationFailure(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.Failure == nil {
		t.Fatal("observation failure = nil")
	}
	if observation.Failure.Kind != workersessions.FailureCauseWorkersExecutionFailure || observation.Failure.Detail != "execution failed" {
		t.Fatalf("observation failure = %#v", observation.Failure)
	}
}

func requireDurableInt(t *testing.T, name string, value *int, want int) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s token = nil", name)
	}
	if *value != want {
		t.Fatalf("%s token = %d, want %d", name, *value, want)
	}
}

func TestDurableTranscriptNormalizesSourceDrafts(t *testing.T) {
	projection := durableFullProjection(t, workersessions.StateCompleted)
	projection.Degradation = ""
	projection.InterruptionReason = ""
	transcript, err := Transcript(projection)
	if err != nil {
		t.Fatalf("Transcript() error = %v", err)
	}
	requireDurableTranscriptEnvelope(t, transcript, len(projection.Records))
	requireDurableTranscriptMessages(t, transcript.Entries)
	requireDurableTranscriptReasoning(t, transcript.Entries)
	requireDurableTranscriptTools(t, transcript.Entries)
	requireDurableTranscriptSystemEntry(t, transcript.Entries)
	requireDurableTranscriptOrder(t, transcript.Entries)
}

func TestDurableTranscriptMalformedPayloads(t *testing.T) {

	if got := transcriptEntries([]events.Record{{Payload: []byte(`{"kind":"MESSAGE","phase":"COMPLETED","payload":{"role":"assistant","contentBlocks":[]}}`)}}); len(got) != 1 || got[0].Type != workersessions.TranscriptAssistantMessage || got[0].Text != nil {
		t.Fatalf("transcriptEntries(empty message) = %#v", got)
	}
	if got := messageText(workers.MessagePayload{}); got != nil {
		t.Fatalf("messageText(empty) = %v, want nil", got)
	}
	if got := reasoningTranscriptEntry(workersessions.TranscriptEntry{}, []byte("not-json")); got.Type != workersessions.TranscriptReasoning || got.Summary != nil {
		t.Fatalf("reasoningTranscriptEntry(malformed) = %#v", got)
	}
	if got := toolTranscriptEntry(workersessions.TranscriptEntry{}, []byte("not-json")); got.Type != "" {
		t.Fatalf("toolTranscriptEntry(malformed) = %#v", got)
	}
	if got := errorTranscriptEntry(workersessions.TranscriptEntry{}, []byte("not-json")); got.Type != workersessions.TranscriptSystemEvent || got.Summary != nil {
		t.Fatalf("errorTranscriptEntry(malformed) = %#v", got)
	}
	if got := progressTranscriptEntry(workersessions.TranscriptEntry{}, []byte("not-json")); got.Type != workersessions.TranscriptSystemEvent || got.Summary != nil {
		t.Fatalf("progressTranscriptEntry(malformed) = %#v", got)
	}
}

func TestDurableTranscriptRejectsInvalidState(t *testing.T) {
	projection := durableFullProjection(t, workersessions.StateCompleted)
	badState := projection
	badState.Terminal = &recordings.WorkerRecordingTerminal{Status: "UNKNOWN"}
	if _, err := Transcript(badState); !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) {
		t.Fatalf("Transcript(invalid state) error = %v", err)
	}
}

func requireDurableTranscriptEnvelope(t *testing.T, transcript workersessions.ReadTranscriptResult, recordCount int) {
	t.Helper()
	if transcript.WorkerSessionID != "worker-full" || transcript.State != workersessions.StateCompleted {
		t.Fatalf("Transcript() identity/state = %#v", transcript)
	}
	if transcript.AttemptID != "attempt-full" || len(transcript.Entries) != recordCount {
		t.Fatalf("Transcript() attempt/entry count = %#v", transcript)
	}
	if transcript.Entries[1].Type != workersessions.TranscriptSystemEvent || transcript.Entries[2].Type != workersessions.TranscriptSystemEvent {
		t.Fatalf("malformed/empty usage entries = %#v", transcript.Entries[1:3])
	}
}

func findDurableTranscriptType(t *testing.T, entries []workersessions.TranscriptEntry, kind workersessions.TranscriptEntryType) workersessions.TranscriptEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Type == kind {
			return entry
		}
	}
	t.Fatalf("transcript type %q not found in %#v", kind, entries)
	return workersessions.TranscriptEntry{}
}

func requireDurableTranscriptMessages(t *testing.T, entries []workersessions.TranscriptEntry) {
	t.Helper()
	user := findDurableTranscriptType(t, entries, workersessions.TranscriptUserMessage)
	if user.Text == nil || *user.Text != "hello" {
		t.Fatalf("user transcript entry = %#v", user)
	}
	assistant := findDurableTranscriptType(t, entries, workersessions.TranscriptAssistantMessage)
	if assistant.Text == nil || *assistant.Text != "hello world" {
		t.Fatalf("assistant transcript entry = %#v", assistant)
	}
}

func requireDurableTranscriptReasoning(t *testing.T, entries []workersessions.TranscriptEntry) {
	t.Helper()
	reasoning := findDurableTranscriptType(t, entries, workersessions.TranscriptReasoning)
	if reasoning.Summary == nil || *reasoning.Summary != "summary" {
		t.Fatalf("reasoning transcript entry = %#v", reasoning)
	}
	foundDelta := false
	for _, entry := range entries {
		if entry.Type == workersessions.TranscriptReasoning && entry.Summary != nil && *entry.Summary == "delta" {
			foundDelta = true
		}
	}
	if !foundDelta {
		t.Fatal("transcript does not contain the SummaryDelta reasoning entry")
	}
}

func requireDurableTranscriptTools(t *testing.T, entries []workersessions.TranscriptEntry) {
	t.Helper()
	call := findDurableTranscriptType(t, entries, workersessions.TranscriptToolCall)
	if call.CallID == nil || *call.CallID != "call-1" || call.Name == nil || *call.Name != "grep" {
		t.Fatalf("tool call identity = %#v", call)
	}
	if call.Arguments == nil || *call.Arguments != `{"path":"."}` {
		t.Fatalf("tool call arguments = %#v", call)
	}
	output := findDurableTranscriptType(t, entries, workersessions.TranscriptToolOutput)
	if output.Output == nil || *output.Output != `{"ok":true}` {
		t.Fatalf("tool output transcript entry = %#v", output)
	}
}

func requireDurableTranscriptSystemEntry(t *testing.T, entries []workersessions.TranscriptEntry) {
	t.Helper()
	for _, entry := range entries {
		if entry.Type == workersessions.TranscriptSystemEvent && entry.Summary != nil {
			return
		}
	}
	t.Fatalf("system transcript entry with summary not found in %#v", entries)
}

func requireDurableTranscriptOrder(t *testing.T, entries []workersessions.TranscriptEntry) {
	t.Helper()
	for index, entry := range entries {
		if entry.Order != index || entry.LineNumber == nil || *entry.LineNumber != index+1 {
			t.Fatalf("transcript entry %d = %#v, want stable order/line number", index, entry)
		}
	}
}

func TestDurableObservationStreamRejectsInvalidCursors(t *testing.T) {
	projection, deliver := durableObservationStreamFixture(t)
	if _, err := ObservationStream(context.Background(), projection, 1, nil, nil); err == nil {
		t.Fatal("ObservationStream(nil delivery) error = nil, want error")
	}
	foreignCases := []struct {
		name   string
		cursor *workersessions.ObservationCursor
		want   error
	}{
		{name: "foreign Worker Session", cursor: &workersessions.ObservationCursor{WorkerSessionID: "other", Position: 1}, want: workersessions.ErrObservationCursorForeign},
		{name: "foreign generation", cursor: &workersessions.ObservationCursor{StreamGenerationID: WorkerStreamGenerationForIdentity("other"), Position: 1}, want: workersessions.ErrObservationCursorForeign},
		{name: "unavailable generation", cursor: &workersessions.ObservationCursor{StreamGenerationID: "unknown-generation", Position: 1}, want: workersessions.ErrObservationCursorUnavailable},
		{name: "future position", cursor: &workersessions.ObservationCursor{Position: 3}, want: workersessions.ErrObservationCursorFuture},
	}
	for _, test := range foreignCases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ObservationStream(context.Background(), projection, 1, test.cursor, deliver); !errors.Is(err, test.want) {
				t.Fatalf("ObservationStream() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDurableObservationStreamReplaysCompletion(t *testing.T) {
	projection, deliver := durableObservationStreamFixture(t)
	subscription, err := ObservationStream(context.Background(), projection, 0, &workersessions.ObservationCursor{Position: 1}, deliver)
	if err != nil {
		t.Fatalf("ObservationStream(valid cursor) error = %v", err)
	}
	if got := subscription.Next(nil); got.Kind != workersessions.ObservationDeliveryRecord || got.Event.Position != 2 || got.Event.Cursor.StreamGenerationID != WorkerStreamGenerationForIdentity("worker-full") {
		t.Fatalf("first replay delivery = %#v, want position 2 with stable generation", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryReplaySummary || got.Summary == nil || !got.Summary.Complete || got.Summary.Reason != "session-completed" || got.Summary.EventsEmitted != 1 {
		t.Fatalf("completion summary = %#v, want complete one-event summary", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after summary = %#v, want closed", got)
	}
	subscription.Close()
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after Close = %#v, want closed", got)
	}
}

func TestDurableObservationStreamCancelsDelivery(t *testing.T) {
	projection, deliver := durableObservationStreamFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uncanceled, err := ObservationStream(ctx, projection, 1, nil, deliver)
	if err != nil {
		t.Fatalf("ObservationStream(canceled context) error = %v", err)
	}
	if got := uncanceled.Next(ctx); got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled delivery = %#v, want typed cancellation", got)
	}
}

func TestDurableObservationStreamHealthSummariesAndHelpers(t *testing.T) {
	projection, deliver := durableObservationStreamFixture(t)
	degraded := projection
	degraded.Status = recordings.WorkerRecordingStatusDegraded
	degraded.Degradation = "capture lost"
	degradedSubscription, err := ObservationStream(context.Background(), degraded, 1, nil, deliver)
	if err != nil {
		t.Fatalf("ObservationStream(degraded) error = %v", err)
	}
	if got := degradedSubscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("degraded record = %#v", got)
	}
	_ = degradedSubscription.Next(context.Background())
	if got := degradedSubscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryReplaySummary || got.Summary == nil || got.Summary.Complete || got.Summary.Reason != "capture lost" {
		t.Fatalf("degraded summary = %#v", got)
	}

	incomplete := projection
	incomplete.Status = recordings.WorkerRecordingStatusIncomplete
	incomplete.Degradation = ""
	incomplete.InterruptionReason = "process stopped"
	incompleteSubscription, err := ObservationStream(context.Background(), incomplete, 1, nil, deliver)
	if err != nil {
		t.Fatalf("ObservationStream(incomplete) error = %v", err)
	}
	_ = incompleteSubscription.Next(context.Background())
	_ = incompleteSubscription.Next(context.Background())
	if got := incompleteSubscription.Next(context.Background()); got.Summary == nil || got.Summary.Reason != "process stopped" {
		t.Fatalf("incomplete summary = %#v", got)
	}
	var nilSubscription *observationSubscription
	nilSubscription.Close()
	if got := replayReason(recordings.WorkerRecordingProjection{Status: recordings.WorkerRecordingStatusIncomplete}); got != "recording-incomplete" {
		t.Fatalf("replayReason(incomplete) = %q", got)
	}
	if got := WorkerStreamGenerationForIdentity(" worker-full "); got != "worker-recording/worker-full" {
		t.Fatalf("WorkerStreamGenerationForIdentity() = %q", got)
	}
	if !isTerminalLifecycleRecord(events.Record{SourceType: lifecycleSourceType, SourceSequence: terminalSourceSequence, SourceEventID: terminalSourceEventID}) {
		t.Fatal("isTerminalLifecycleRecord(valid) = false")
	}
	if isTerminalLifecycleRecord(events.Record{SourceType: lifecycleSourceType, SourceSequence: 1, SourceEventID: terminalSourceEventID}) {
		t.Fatal("isTerminalLifecycleRecord(non-terminal sequence) = true")
	}
}

func durableObservationStreamFixture(t *testing.T) (recordings.WorkerRecordingProjection, func(events.Record, bool, string) workersessions.ObservationDelivery) {
	t.Helper()
	projection := durableFullProjection(t, workersessions.StateCompleted)
	projection.Records = projection.Records[:2]
	projection.LastPosition = 2
	projection.Terminal = &recordings.WorkerRecordingTerminal{Position: 2, Status: "COMPLETED"}
	projection.Degradation = ""
	projection.InterruptionReason = ""
	deliver := func(record events.Record, _ bool, workerSessionID string) workersessions.ObservationDelivery {
		return workersessions.ObservationDelivery{
			Kind: workersessions.ObservationDeliveryRecord,
			Event: workersessions.ObservationEvent{
				Position:   uint64(record.ID.Position),
				Cursor:     workersessions.ObservationCursor{WorkerSessionID: workerSessionID, Position: uint64(record.ID.Position)},
				SourceType: string(record.SourceType),
			},
		}
	}
	return projection, deliver
}

func durableFullProjection(t *testing.T, state workersessions.State) recordings.WorkerRecordingProjection {
	t.Helper()
	const sessionID = "worker-full"
	startedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	openingPayload := workers.SessionPayload{
		Status:           "STARTING",
		StartedAt:        &startedAt,
		WorkerSessionID:  sessionID,
		FactorySessionID: "factory-full",
		WorkIDs:          []string{"work-full"},
		AttemptID:        "attempt-full",
		TurnID:           "turn-full",
		DispatchID:       "dispatch-full",
		AttemptReason:    workers.AttemptReasonResume,
		Continuation:     &workers.SessionContinuation{Provider: "codex", Kind: "session_id", ID: "provider-full"},
		Lineage: &workers.SessionLineage{
			PredecessorWorkerSessionID: "worker-previous",
			SuccessorWorkerSessionID:   "worker-successor",
			PreviousDispatchID:         "dispatch-previous",
			PreviousAttemptID:          "attempt-previous",
		},
		Model:           "model-full",
		ReasoningEffort: "high",
	}
	records := []events.Record{
		durableSessionRecord(t, sessionID, 1, workers.PhaseStarted, openingPayload),
		{ID: events.RecordID{Topic: durableTopic(sessionID), Position: 2}, Payload: []byte("not-json")},
		durableDraftRecord(t, sessionID, 3, workers.KindUsage, workers.PhaseUpdated, workers.UsagePayload{}),
		durableDraftRecord(t, sessionID, 4, workers.KindUsage, workers.PhaseUpdated, workers.UsagePayload{
			InputTokens: 11, CachedInputTokens: 3, OutputTokens: 7, ReasoningOutputTokens: 5, TotalTokens: 26, Model: "usage-model",
		}),
		durableDraftRecord(t, sessionID, 5, workers.KindMessage, workers.PhaseCompleted, workers.MessagePayload{
			Role: "user", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}},
		}),
		durableDraftRecord(t, sessionID, 6, workers.KindMessage, workers.PhaseCompleted, workers.MessagePayload{
			Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}, {Kind: workers.ContentBlockText, Text: " world"}},
		}),
		durableDraftRecord(t, sessionID, 7, workers.KindReasoning, workers.PhaseUpdated, workers.ReasoningPayload{Summary: "summary"}),
		durableDraftRecord(t, sessionID, 8, workers.KindReasoning, workers.PhaseUpdated, workers.ReasoningPayload{SummaryDelta: "delta"}),
		durableDraftRecord(t, sessionID, 9, workers.KindTool, workers.PhaseStarted, workers.ToolPayload{ToolCallID: "call-1", ToolName: "grep", ArgumentsSummary: json.RawMessage(`{"path":"."}`)}),
		durableDraftRecord(t, sessionID, 10, workers.KindTool, workers.PhaseCompleted, workers.ToolPayload{ToolCallID: "call-1", ToolName: "grep", ResultSummary: json.RawMessage(`{"ok":true}`)}),
		durableDraftRecord(t, sessionID, 11, workers.KindError, workers.PhaseUpdated, workers.ErrorPayload{Message: "error detail"}),
		durableDraftRecord(t, sessionID, 12, workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{Label: "progress", Message: "progress detail"}),
		durableDraftRecord(t, sessionID, 13, workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{Label: "label fallback"}),
		durableDraftRecord(t, sessionID, 14, workers.KindRun, workers.PhaseUpdated, workers.RunPayload{Status: "RUNNING"}),
		durableDraftRecord(t, sessionID, 15, workers.KindTurn, workers.PhaseUpdated, workers.TurnPayload{TurnIndex: 1}),
		durableDraftRecord(t, sessionID, 16, workers.KindPlan, workers.PhaseUpdated, workers.PlanPayload{Summary: "plan"}),
		durableDraftRecord(t, sessionID, 17, workers.KindFileChange, workers.PhaseUpdated, workers.FileChangePayload{Path: "file.txt", Operation: "write"}),
		durableDraftRecord(t, sessionID, 18, workers.KindStreamGap, workers.PhaseUpdated, workers.StreamGapPayload{Reason: "gap"}),
		{ID: events.RecordID{Topic: durableTopic(sessionID), Position: 19}, Payload: []byte("not-a-draft")},
	}
	terminalPayload := map[string]string{
		"status":               string(state),
		"failureCause":         string(workersessions.FailureCauseWorkersExecutionFailure),
		"failureDetail":        "execution failed",
		"agentRunFailureClass": "",
	}
	terminal := durableSessionRecord(t, sessionID, 20, terminalPhase(state), terminalPayload)
	terminal.SourceSequence = terminalSourceSequence
	terminal.SourceEventID = terminalSourceEventID
	records = append(records, terminal)
	return recordings.WorkerRecordingProjection{
		RecordingID:        "recording-full",
		WorkerSessionID:    sessionID,
		FactorySessionID:   "factory-full",
		WorkIDs:            []string{"work-full"},
		AttemptID:          "attempt-full",
		Status:             recordings.WorkerRecordingStatusComplete,
		Complete:           true,
		LastPosition:       20,
		Terminal:           &recordings.WorkerRecordingTerminal{Position: 20, Phase: terminalPhase(state), Status: string(state)},
		Degradation:        "persisted",
		InterruptionReason: "interrupted",
		Records:            records,
	}
}

func durableReplaySnapshot(t *testing.T, sessionID string) recordings.WorkerRecordingSnapshot {
	t.Helper()
	openingPayload := workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID, AttemptID: "attempt-replay"}
	return recordings.WorkerRecordingSnapshot{
		RecordingID: "recording-replay",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID,
			Status:          recordings.WorkerRecordingStatusComplete,
			LastPosition:    2,
			Records:         durableReplayRecords(t, sessionID, openingPayload),
		}},
	}
}

func durableReplayRecords(t *testing.T, sessionID string, openingPayload workers.SessionPayload) []events.Record {
	t.Helper()
	opening := durableSessionRecord(t, sessionID, 1, workers.PhaseStarted, openingPayload)
	opening.SourceSequence = 1
	opening.SourceEventID = "started"
	terminal := durableSessionRecord(t, sessionID, 2, workers.PhaseCompleted, map[string]string{"status": "COMPLETED"})
	terminal.SourceSequence = 2
	terminal.SourceEventID = "terminal"
	return []events.Record{opening, terminal}
}

func durableTopic(sessionID string) events.Topic {
	return events.Topic("worker-session/" + sessionID + "/events")
}

func durableSessionRecord(t *testing.T, sessionID string, position events.AggregateSequence, phase workers.Phase, payload any) events.Record {
	t.Helper()
	return durableDraftRecordWithPayload(t, sessionID, position, workers.KindSession, phase, payload)
}

func durableDraftRecord(t *testing.T, sessionID string, position events.AggregateSequence, kind workers.Kind, phase workers.Phase, payload any) events.Record {
	t.Helper()
	return durableDraftRecordWithPayload(t, sessionID, position, kind, phase, payload)
}

func durableDraftRecordWithPayload(t *testing.T, sessionID string, position events.AggregateSequence, kind workers.Kind, phase workers.Phase, payload any) events.Record {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal durable payload: %v", err)
	}
	draft, err := json.Marshal(workers.Draft{
		Kind:  kind,
		Phase: phase,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliverySynthesized,
			Fidelity:        workers.FidelityLifecycleOnly,
			NativeEventType: "worker_session_lifecycle",
			Representation:  workers.RepresentationNotification,
		},
		Payload: payloadBytes,
	})
	if err != nil {
		t.Fatalf("marshal durable draft: %v", err)
	}
	return events.Record{
		ID:             events.RecordID{Topic: durableTopic(sessionID), Position: position},
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(sessionID),
		SourceSequence: events.SourceSequence(position),
		SourceEventID:  events.SourceEventID(fmt.Sprintf("event-%d", position)),
		SchemaID:       "workers.draft.v1",
		Payload:        draft,
	}
}

func terminalPhase(state workersessions.State) workers.Phase {
	switch state {
	case workersessions.StateCompleted:
		return workers.PhaseCompleted
	case workersessions.StateFailed:
		return workers.PhaseFailed
	case workersessions.StateCanceled:
		return workers.PhaseCanceled
	default:
		return workers.PhaseCompleted
	}
}
