package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestControlHistory_OrdersPauseResumeBracketsBeforeTerminalAndDeduplicatesReplay(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-control-history",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "pause-1"})
	if err != nil || paused.Outcome != workersessions.ControlOutcomeApplied || paused.Session.State != workersessions.StatePaused {
		t.Fatalf("Pause() = %#v, %v, want applied PAUSED", paused, err)
	}
	if repeated, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "pause-1"}); err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("replayed Pause() = %#v, %v, want NOOP without a second history bracket", repeated, err)
	}

	resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "resume-1"})
	if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied || resumed.Session.State != workersessions.StateRunning {
		t.Fatalf("Resume() = %#v, %v, want applied RUNNING", resumed, err)
	}
	resumedResult := completedDispatchResult(resumed.DispatchID)
	resumedResult.Result.ProviderSession = &workers.ProviderSessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
	boundary.complete(resumedResult, nil)
	if result := <-started; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("InvokeSession() result = %#v, want COMPLETED", result)
	}
	if repeated, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "resume-1"}); err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("replayed terminal Resume() = %#v, %v, want NOOP without a second history bracket", repeated, err)
	}

	topic := workersessions.Topic("worker-1")
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(read.Records) != 6 {
		t.Fatalf("control history record count = %d, want opening + two request/outcome pairs + terminal", len(read.Records))
	}

	var records [4]workersessions.ControlRecordPayload
	for index, recordIndex := range []int{1, 2, 3, 4} {
		var draft workers.Draft
		if err := json.Unmarshal(read.Records[recordIndex].Payload, &draft); err != nil {
			t.Fatalf("control draft %d decode error = %v", recordIndex, err)
		}
		if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseUpdated {
			t.Fatalf("control draft %d = %#v, want SESSION/UPDATED", recordIndex, draft)
		}
		if err := json.Unmarshal(draft.Payload, &records[index]); err != nil {
			t.Fatalf("control payload %d decode error = %v", recordIndex, err)
		}
		if err := records[index].Validate(); err != nil {
			t.Fatalf("control payload %d validation error = %v", recordIndex, err)
		}
		if read.Records[recordIndex].ID.Position <= read.Records[recordIndex-1].ID.Position {
			t.Fatalf("control record %d position = %d, want after record %d position %d", recordIndex, read.Records[recordIndex].ID.Position, recordIndex-1, read.Records[recordIndex-1].ID.Position)
		}
	}
	if records[0].RecordType != workersessions.ControlRecordTypeRequest || records[0].Action != workersessions.ControlActionPause || records[0].RequestID != "pause-1" {
		t.Fatalf("pause request payload = %#v", records[0])
	}
	if records[1].RecordType != workersessions.ControlRecordTypeOutcome || records[1].Outcome != workersessions.ControlOutcomeApplied || records[1].CorrelationID != records[0].CorrelationID {
		t.Fatalf("pause outcome payload = %#v, want applied matching correlation", records[1])
	}
	if records[2].RecordType != workersessions.ControlRecordTypeRequest || records[2].Action != workersessions.ControlActionResume || records[2].RequestID != "resume-1" {
		t.Fatalf("resume request payload = %#v", records[2])
	}
	if records[3].RecordType != workersessions.ControlRecordTypeOutcome || records[3].Outcome != workersessions.ControlOutcomeApplied || records[3].CorrelationID != records[2].CorrelationID {
		t.Fatalf("resume outcome payload = %#v, want applied matching correlation", records[3])
	}
	if records[1].DispatchID != "dispatch-1" || records[3].DispatchID != resumed.DispatchID {
		t.Fatalf("control dispatch identities = pause %q, resume %q; want exact attempts", records[1].DispatchID, records[3].DispatchID)
	}

	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: "recording-control-history",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: "worker-1",
			Topic:           topic,
			Status:          recordings.WorkerRecordingStatusCompleted,
			LastPosition:    read.Records[len(read.Records)-1].ID.Position,
			Records:         read.Records,
		}},
	}
	codec := recordings.WorkerRecordingCodec{}
	portable, err := codec.BuildWorkerPortableRecording(snapshot)
	if err != nil {
		t.Fatalf("BuildWorkerPortableRecording() error = %v", err)
	}
	encoded, err := codec.EncodeWorkerPortableRecording(portable)
	if err != nil {
		t.Fatalf("EncodeWorkerPortableRecording() error = %v", err)
	}
	decoded, err := codec.DecodeWorkerPortableRecording(encoded)
	if err != nil {
		t.Fatalf("DecodeWorkerPortableRecording() error = %v", err)
	}
	if len(decoded.Records) != len(portable.Records) || decoded.Records[1].Payload == nil ||
		decoded.Records[1].SourceType != events.SourceType("worker_session_control") ||
		decoded.Records[2].SourceType != events.SourceType("worker_session_control") {
		t.Fatalf("portable control records = %#v, want exact ordered control records", decoded.Records)
	}
	replayed, err := codec.ReplayWorkerPortableRecording(decoded)
	if err != nil || replayed.Projection.Status != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("ReplayWorkerPortableRecording() = %#v, %v, want complete preserved history", replayed, err)
	}
}

func TestControlHistory_NaturalCompletionWinsAfterControlRequestAndBeforeTerminalRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	started := startControlledSession(t, registry, boundary, "worker-natural", "dispatch-natural")
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-natural",
		DispatchID:      "dispatch-natural",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-natural-race",
		},
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	cancelEntered := make(chan struct{})
	releaseCancel := make(chan struct{})
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		close(cancelEntered)
		<-releaseCancel
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID}, nil
	})

	controlResult := make(chan struct {
		result workersessions.ControlResult
		err    error
	}, 1)
	go func() {
		result, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-natural", RequestID: "natural-race-control"})
		controlResult <- struct {
			result workersessions.ControlResult
			err    error
		}{result: result, err: err}
	}()
	<-cancelEntered

	boundary.complete(completedDispatchResult("dispatch-natural"), nil)
	close(releaseCancel)
	control := <-controlResult
	if control.err != nil || control.result.Outcome != workersessions.ControlOutcomeNoop || control.result.Session.State != workersessions.StateCompleted {
		t.Fatalf("racing Pause() = %#v, %v, want natural COMPLETED NOOP", control.result, control.err)
	}
	if result := <-started; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("natural completion result = %#v, want COMPLETED", result)
	}

	topic := workersessions.Topic("worker-natural")
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil || len(read.Records) != 4 {
		t.Fatalf("natural control history = %+v, %v, want opening/request/outcome/terminal", read, err)
	}
	var requestDraft, outcomeDraft workers.Draft
	if err := json.Unmarshal(read.Records[1].Payload, &requestDraft); err != nil {
		t.Fatalf("natural request draft decode error = %v", err)
	}
	if err := json.Unmarshal(read.Records[2].Payload, &outcomeDraft); err != nil {
		t.Fatalf("natural outcome draft decode error = %v", err)
	}
	var request, outcome workersessions.ControlRecordPayload
	if err := json.Unmarshal(requestDraft.Payload, &request); err != nil {
		t.Fatalf("natural request payload decode error = %v", err)
	}
	if err := json.Unmarshal(outcomeDraft.Payload, &outcome); err != nil {
		t.Fatalf("natural outcome payload decode error = %v", err)
	}
	if request.RecordType != workersessions.ControlRecordTypeRequest || outcome.RecordType != workersessions.ControlRecordTypeOutcome ||
		outcome.Outcome != workersessions.ControlOutcomeNoop || request.CorrelationID != outcome.CorrelationID {
		t.Fatalf("natural control bracket = request %#v outcome %#v, want one matching NOOP bracket", request, outcome)
	}
}

func TestControlHistory_RecordsInterruptBracketBeforeSourceTerminal(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	sourceResult := startControlledSession(t, registry, boundary, "source-interrupt", "dispatch-source-interrupt")
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-interrupt-history",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-interrupt",
		DispatchID:      "dispatch-source-interrupt",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	interrupted, err := registry.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID:                "interrupt-history-1",
		SourceWorkerSessionID:    "source-interrupt",
		SuccessorWorkerSessionID: "successor-interrupt",
		ReplacementMessage:       "replacement",
	})
	if err != nil || !interrupted.Accepted {
		t.Fatalf("Interrupt() = %#v, %v, want accepted", interrupted, err)
	}
	if result := <-sourceResult; result.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", result.Session)
	}

	topic := workersessions.Topic("source-interrupt")
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil || len(read.Records) != 4 {
		t.Fatalf("interrupt source history = %+v, %v, want opening/request/outcome/terminal", read, err)
	}
	var requestDraft, outcomeDraft workers.Draft
	if err := json.Unmarshal(read.Records[1].Payload, &requestDraft); err != nil {
		t.Fatalf("interrupt request draft decode error = %v", err)
	}
	if err := json.Unmarshal(read.Records[2].Payload, &outcomeDraft); err != nil {
		t.Fatalf("interrupt outcome draft decode error = %v", err)
	}
	var request, outcome workersessions.ControlRecordPayload
	if err := json.Unmarshal(requestDraft.Payload, &request); err != nil {
		t.Fatalf("interrupt request payload decode error = %v", err)
	}
	if err := json.Unmarshal(outcomeDraft.Payload, &outcome); err != nil {
		t.Fatalf("interrupt outcome payload decode error = %v", err)
	}
	if request.RecordType != workersessions.ControlRecordTypeRequest ||
		request.Action != workersessions.ControlActionInterrupt ||
		request.RequestID != "interrupt-history-1" ||
		outcome.RecordType != workersessions.ControlRecordTypeOutcome ||
		outcome.Action != workersessions.ControlActionInterrupt ||
		outcome.Outcome != workersessions.ControlOutcomeApplied ||
		request.CorrelationID != outcome.CorrelationID {
		t.Fatalf("interrupt control bracket = request %#v outcome %#v, want applied matching bracket", request, outcome)
	}
	if outcome.DispatchID != "dispatch-source-interrupt" || read.Records[3].ID.Position <= read.Records[2].ID.Position {
		t.Fatalf("interrupt ordering/dispatch = outcome %#v terminal position %d, want exact dispatch before terminal", outcome, read.Records[3].ID.Position)
	}

	boundary.complete(completedDispatchResult(interrupted.Successor.ProviderSessionAssociation.DispatchID), nil)
}
