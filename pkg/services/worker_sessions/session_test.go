package workersessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRuntimeAttempt_CompleteDelegatesAndRejectsUnavailable(t *testing.T) {
	if err := (workersessions.RuntimeAttempt(nil)).Complete(context.Background(), workers.WorkstationDispatchResult{}, nil); err == nil {
		t.Fatal("nil RuntimeAttempt.Complete() error = nil, want unavailable error")
	}

	type contextKey string
	const key contextKey = "worker-session-test"
	wantContext := context.WithValue(context.Background(), key, "present")
	wantDispatchErr := errors.New("dispatch failed")
	var gotResult workers.WorkstationDispatchResult
	var gotDispatchErr error
	called := false
	attempt := workersessions.RuntimeAttempt(func(ctx context.Context, result workers.WorkstationDispatchResult, dispatchErr error) error {
		called = true
		if ctx.Value(key) != "present" {
			t.Errorf("callback context value = %v, want present", ctx.Value(key))
		}
		gotResult = result
		gotDispatchErr = dispatchErr
		return wantDispatchErr
	})

	wantResult := workers.WorkstationDispatchResult{}
	if err := attempt.Complete(wantContext, wantResult, wantDispatchErr); !errors.Is(err, wantDispatchErr) {
		t.Fatalf("RuntimeAttempt.Complete() error = %v, want %v", err, wantDispatchErr)
	}
	if !called || !reflect.DeepEqual(gotResult, wantResult) || !errors.Is(gotDispatchErr, wantDispatchErr) {
		t.Fatalf("RuntimeAttempt callback called=%v result=%#v dispatchErr=%v, want called with %#v and %v", called, gotResult, gotDispatchErr, wantResult, wantDispatchErr)
	}
}

func TestSession_Validate_AcceptsNonEmptyIDAndAcceptedState(t *testing.T) {
	session := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestPublish_CanonicalMockUsagePreservesExplicitZeroesAndModel(t *testing.T) {
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "worker-1",
		Kind:       workers.ProgressFragmentKind,
		Type:       "usage.updated",
		Provider:   "codex",
		Payload:    `{"inputTokens":0,"outputTokens":5,"reasoningOutputTokens":0,"totalTokens":5,"model":"gpt-5-codex"}`,
	})

	if len(spy.published) != 1 {
		t.Fatalf("published records = %d, want exactly one usage record", len(spy.published))
	}
	draft := spy.published[0].Draft
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated || draft.Provenance.Provider != "codex" {
		t.Fatalf("draft = %#v, want codex USAGE/UPDATED", draft)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("draft payload is not valid JSON: %v", err)
	}
	for _, field := range []string{"inputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens", "model"} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("draft payload = %s, missing %q", draft.Payload, field)
		}
	}
	if _, ok := payload["cachedInputTokens"]; ok {
		t.Fatalf("draft payload = %s, cachedInputTokens should remain omitted", draft.Payload)
	}
}

func TestSession_Validate_RejectsEmptyAndWhitespaceIdentity(t *testing.T) {
	for _, id := range []string{"", "   ", "\t"} {
		session := workersessions.Session{ID: id, State: workersessions.StateRunning}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
			t.Errorf("Validate() with ID %q = %v, want ErrInvalidSessionID", id, err)
		}
	}
}

func TestSession_Validate_RejectsUnknownAndInterruptedState(t *testing.T) {
	for _, state := range []workersessions.State{"", "INTERRUPTED", "unknown"} {
		session := workersessions.Session{ID: "worker-1", State: state}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
			t.Errorf("Validate() with state %q = %v, want ErrInvalidState", state, err)
		}
	}
}

func TestSession_Validate_IsDeterministicAndDoesNotMutate(t *testing.T) {
	session := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}
	original := session

	firstErr := session.Validate()
	secondErr := session.Validate()

	if firstErr != nil || secondErr != nil {
		t.Fatalf("Validate() = (%v, %v), want (nil, nil)", firstErr, secondErr)
	}
	if session != original {
		t.Fatalf("Validate() mutated the session: got %+v, want %+v", session, original)
	}
}

func TestSession_Validate_CompletedRequiresMatchingCompletedResult(t *testing.T) {
	session := workersessions.Session{
		ID:     "worker-1",
		State:  workersessions.StateCompleted,
		Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	missing := workersessions.Session{ID: "worker-1", State: workersessions.StateCompleted}
	if err := missing.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with missing Result = %v, want ErrInvalidTerminalResult", err)
	}

	mismatched := workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateCompleted,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "execution failed"},
		},
	}
	if err := mismatched.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with mismatched Result = %v, want ErrInvalidTerminalResult", err)
	}
}

func TestSession_Validate_FailedRequiresMatchingFailedResultWithCause(t *testing.T) {
	session := workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateFailed,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "executor failed"},
		},
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	noCause := workersessions.Session{
		ID:     "worker-1",
		State:  workersessions.StateFailed,
		Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed},
	}
	if err := noCause.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with FAILED and nil Cause = %v, want ErrInvalidTerminalResult", err)
	}
}

func TestSession_Validate_NonTerminalStateRejectsTerminalResult(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved, workersessions.StateStarting, workersessions.StateRunning, workersessions.StatePaused,
	} {
		session := workersessions.Session{
			ID:     "worker-1",
			State:  state,
			Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
		}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
			t.Errorf("Validate() with state %q and a Result = %v, want ErrInvalidTerminalResult", state, err)
		}
	}
}

func TestSession_Terminal_MatchesStateTerminal(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved, workersessions.StateStarting, workersessions.StateRunning, workersessions.StatePaused,
		workersessions.StateCompleted, workersessions.StateFailed, workersessions.StateCanceled, workersessions.StateTerminated,
	} {
		session := workersessions.Session{ID: "worker-1", State: state}
		if got, want := session.Terminal(), state.Terminal(); got != want {
			t.Errorf("Session.Terminal() with state %q = %v, want %v", state, got, want)
		}
	}
}

func TestReserveRequest_Validate_RejectsEmptyIdentity(t *testing.T) {
	if err := (workersessions.ReserveRequest{ID: ""}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Errorf("Validate() = %v, want ErrInvalidSessionID", err)
	}
	if err := (workersessions.ReserveRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestGetRequest_Validate_RejectsEmptyIdentity(t *testing.T) {
	if err := (workersessions.GetRequest{ID: ""}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Errorf("Validate() = %v, want ErrInvalidSessionID", err)
	}
	if err := (workersessions.GetRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestFilter_Validate_RejectsUnknownState(t *testing.T) {
	filter := workersessions.Filter{States: []workersessions.State{workersessions.StateRunning, "INTERRUPTED"}}
	if err := filter.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
		t.Errorf("Validate() = %v, want ErrInvalidState", err)
	}
}

func TestFilter_Validate_AcceptsEmptyAndAllValidStates(t *testing.T) {
	if err := (workersessions.Filter{}).Validate(); err != nil {
		t.Errorf("Validate() on empty filter = %v, want nil", err)
	}
	filter := workersessions.Filter{States: []workersessions.State{workersessions.StateRunning, workersessions.StateCompleted}}
	if err := filter.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestListRequest_Validate_DelegatesToFilter proves ListRequest.Validate is
// exactly req.Filter.Validate(): it accepts a well-formed Filter and rejects
// the same malformed Filter Filter.Validate itself rejects.
func TestListRequest_Validate_DelegatesToFilter(t *testing.T) {
	if err := (workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateRunning}}}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
	req := workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}}
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
		t.Errorf("Validate() = %v, want ErrInvalidState", err)
	}
}

func TestProviderSessionAssociation_ValidateAndClone(t *testing.T) {
	valid := workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker-1",
		TurnID:          "turn-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid association Validate() = %v, want nil", err)
	}
	if got := valid.Clone(); got != valid {
		t.Fatalf("Clone() = %#v, want equal detached value %#v", got, valid)
	}

	for _, test := range []struct {
		name        string
		association workersessions.ProviderSessionAssociation
		wantErr     error
	}{
		{name: "missing worker session", association: workersessions.ProviderSessionAssociation{DispatchID: "dispatch-1", AttemptID: "dispatch-1", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidSessionID},
		{name: "missing dispatch", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", AttemptID: "dispatch-1", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "mismatched attempt", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", DispatchID: "dispatch-1", AttemptID: "dispatch-2", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "invalid provider reference", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", DispatchID: "dispatch-1", AttemptID: "dispatch-1", Reference: providers.SessionRef{Kind: providers.SessionIDKind, ID: "provider-session-1"}}, wantErr: providers.ErrInvalidID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.association.Validate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestProviderSessionAssociationRequest_Validate(t *testing.T) {
	valid := workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid association request Validate() = %v, want nil", err)
	}
	for _, test := range []struct {
		name    string
		req     workersessions.ProviderSessionAssociationRequest
		wantErr error
	}{
		{name: "missing worker session", req: workersessions.ProviderSessionAssociationRequest{DispatchID: valid.DispatchID, Reference: valid.Reference}, wantErr: workersessions.ErrInvalidSessionID},
		{name: "blank dispatch", req: workersessions.ProviderSessionAssociationRequest{WorkerSessionID: valid.WorkerSessionID, DispatchID: " ", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "invalid provider reference", req: workersessions.ProviderSessionAssociationRequest{WorkerSessionID: valid.WorkerSessionID, DispatchID: valid.DispatchID, Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind}}, wantErr: providers.ErrInvalidSessionRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.req.Validate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestControlRequest_Validate(t *testing.T) {
	if err := (workersessions.ControlRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Fatalf("valid control request Validate() = %v, want nil", err)
	}
	if err := (workersessions.ControlRequest{ID: " \t"}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("blank control request Validate() = %v, want ErrInvalidSessionID", err)
	}
}

func TestControlRecordPayload_ValidateCoversRequestAndOutcomeContract(t *testing.T) {
	validRequest := workersessions.ControlRecordPayload{
		RecordType:      workersessions.ControlRecordTypeRequest,
		Action:          workersessions.ControlActionResume,
		RequestID:       "request-1",
		CorrelationID:   "worker-1/request-1",
		WorkerSessionID: "worker-1",
	}
	validOutcome := validRequest
	validOutcome.RecordType = workersessions.ControlRecordTypeOutcome
	validOutcome.Outcome = workersessions.ControlOutcomeApplied

	for name, payload := range map[string]workersessions.ControlRecordPayload{
		"valid request": validRequest,
		"valid outcome": validOutcome,
	} {
		t.Run(name, func(t *testing.T) {
			if err := payload.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}

	for name, payload := range map[string]workersessions.ControlRecordPayload{
		"invalid record type": {
			RecordType: workersessions.ControlRecordType("other"), Action: workersessions.ControlActionPause,
			RequestID: "request-1", CorrelationID: "correlation-1", WorkerSessionID: "worker-1",
		},
		"invalid action": {
			RecordType: workersessions.ControlRecordTypeRequest, Action: workersessions.ControlAction("other"),
			RequestID: "request-1", CorrelationID: "correlation-1", WorkerSessionID: "worker-1",
		},
		"missing identity": {
			RecordType: workersessions.ControlRecordTypeRequest, Action: workersessions.ControlActionPause,
		},
		"request with outcome": {
			RecordType: workersessions.ControlRecordTypeRequest, Action: workersessions.ControlActionPause,
			Outcome: workersessions.ControlOutcomeApplied, RequestID: "request-1", CorrelationID: "correlation-1", WorkerSessionID: "worker-1",
		},
		"outcome without stable outcome": {
			RecordType: workersessions.ControlRecordTypeOutcome, Action: workersessions.ControlActionPause,
			RequestID: "request-1", CorrelationID: "correlation-1", WorkerSessionID: "worker-1",
		},
		"outcome with unknown outcome": {
			RecordType: workersessions.ControlRecordTypeOutcome, Action: workersessions.ControlActionPause,
			Outcome: workersessions.ControlOutcome("other"), RequestID: "request-1", CorrelationID: "correlation-1", WorkerSessionID: "worker-1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := payload.Validate(); err == nil {
				t.Fatal("Validate() = nil, want validation error")
			}
		})
	}
}

func TestContinueRequest_ValidateAndNormalize(t *testing.T) {
	valid := workersessions.ContinueRequest{
		RequestID:                " request-1 ",
		SourceWorkerSessionID:    " source-1 ",
		SuccessorWorkerSessionID: " successor-1 ",
		FollowUpInput:            "  follow-up  ",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ContinueRequest.Validate() = %v, want nil", err)
	}
	normalized := valid.Normalize()
	if normalized.RequestID != "request-1" || normalized.SourceWorkerSessionID != "source-1" || normalized.SuccessorWorkerSessionID != "successor-1" {
		t.Fatalf("Normalize() identities = %#v, want trimmed identities", normalized)
	}
	if normalized.FollowUpInput != valid.FollowUpInput {
		t.Fatalf("Normalize() changed follow-up input from %q to %q", valid.FollowUpInput, normalized.FollowUpInput)
	}

	for _, test := range []struct {
		name string
		req  workersessions.ContinueRequest
		want error
	}{
		{name: "missing request ID", req: workersessions.ContinueRequest{SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", FollowUpInput: "input"}, want: workersessions.ErrInvalidContinuationRequestID},
		{name: "same lineage identity", req: workersessions.ContinueRequest{RequestID: "request", SourceWorkerSessionID: "same", SuccessorWorkerSessionID: "same", FollowUpInput: "input"}, want: workersessions.ErrInvalidContinuationLineage},
		{name: "missing input", req: workersessions.ContinueRequest{RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor"}, want: workersessions.ErrInvalidContinuationInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.req.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSession_Validate_RequiresAssociationToBelongToSession(t *testing.T) {
	association := &workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	valid := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning, ProviderSessionAssociation: association}
	if err := valid.Validate(); err != nil {
		t.Fatalf("associated session Validate() = %v, want nil", err)
	}

	mismatched := valid
	associationCopy := association.Clone()
	mismatched.ProviderSessionAssociation = &associationCopy
	mismatched.ProviderSessionAssociation.WorkerSessionID = "worker-2"
	if err := mismatched.Validate(); !errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation) {
		t.Fatalf("mismatched association Validate() = %v, want ErrInvalidProviderSessionAssociation", err)
	}

	malformed := valid
	malformedAssociation := association.Clone()
	malformedAssociation.Reference.ID = ""
	malformed.ProviderSessionAssociation = &malformedAssociation
	if err := malformed.Validate(); !errors.Is(err, providers.ErrInvalidSessionRef) {
		t.Fatalf("malformed association Validate() = %v, want Providers ErrInvalidSessionRef", err)
	}
}

func TestSession_CloneAndContinueResultCloneDetachNestedState(t *testing.T) {
	association := &workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	model, reasoningEffort := "gpt-5.6-luna", "high"
	original := workersessions.Session{
		ID:                         "worker-1",
		State:                      workersessions.StateFailed,
		Model:                      &model,
		ReasoningEffort:            &reasoningEffort,
		Result:                     &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed, Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "failed"}},
		ProviderSessionAssociation: association,
		PredecessorWorkerSessionID: "previous-worker",
	}
	clone := original.Clone()
	result := workersessions.ContinueResult{
		RequestID:                "request-1",
		SourceWorkerSessionID:    "worker-1",
		SuccessorWorkerSessionID: "worker-2",
		Session:                  original,
	}.Clone()

	clone.Result.Cause.Detail = "mutated clone"
	clone.ProviderSessionAssociation.Reference.ID = "mutated-provider"
	*clone.Model = "mutated-model"
	*clone.ReasoningEffort = "mutated-effort"
	if original.Result.Cause.Detail != "failed" || original.ProviderSessionAssociation.Reference.ID != "provider-session-1" ||
		*original.Model != "gpt-5.6-luna" || *original.ReasoningEffort != "high" {
		t.Fatalf("Session.Clone() shared nested state: original = %#v", original)
	}
	result.Session.Result.Cause.Detail = "mutated result clone"
	result.Session.ProviderSessionAssociation.Reference.ID = "mutated-result-provider"
	*result.Session.Model = "mutated-result-model"
	*result.Session.ReasoningEffort = "mutated-result-effort"
	if original.Result.Cause.Detail != "failed" || original.ProviderSessionAssociation.Reference.ID != "provider-session-1" ||
		*original.Model != "gpt-5.6-luna" || *original.ReasoningEffort != "high" {
		t.Fatalf("ContinueResult.Clone() shared nested state: original = %#v", original)
	}
}

func TestSession_Validate_RejectsMalformedLineage(t *testing.T) {
	for _, test := range []struct {
		name        string
		predecessor string
		successor   string
	}{
		{name: "blank predecessor", predecessor: "   "},
		{name: "self predecessor", predecessor: "worker-1"},
		{name: "blank successor", successor: "\t"},
		{name: "self successor", successor: "worker-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := workersessions.Session{
				ID:                         "worker-1",
				State:                      workersessions.StateRunning,
				PredecessorWorkerSessionID: test.predecessor,
				SuccessorWorkerSessionID:   test.successor,
			}
			if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidContinuationLineage) {
				t.Fatalf("Validate() = %v, want ErrInvalidContinuationLineage", err)
			}
		})
	}
}

func TestInterruptRequestNormalizeValidateAndClonePreserveObservableContract(t *testing.T) {
	request := workersessions.InterruptRequest{
		RequestID:                " request-1 ",
		SourceWorkerSessionID:    " source-1 ",
		SuccessorWorkerSessionID: " successor-1 ",
		ReplacementMessage:       "  preserve surrounding whitespace  ",
	}
	normalized := request.Normalize()
	if normalized.RequestID != "request-1" || normalized.SourceWorkerSessionID != "source-1" || normalized.SuccessorWorkerSessionID != "successor-1" {
		t.Fatalf("Normalize() = %#v, want trimmed identities", normalized)
	}
	if normalized.ReplacementMessage != request.ReplacementMessage {
		t.Fatalf("Normalize() replacement message = %q, want byte-preserved %q", normalized.ReplacementMessage, request.ReplacementMessage)
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("valid InterruptRequest.Validate() = %v", err)
	}

	for name, invalid := range map[string]workersessions.InterruptRequest{
		"missing request ID":        {SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: "message"},
		"invalid lineage":           {RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "source", ReplacementMessage: "message"},
		"blank replacement message": {RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: " \t"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invalid.Validate(); err == nil {
				t.Fatal("Validate() = nil, want validation error")
			}
		})
	}

	result := workersessions.InterruptResult{
		RequestID: request.RequestID,
		Source:    workersessions.Session{ID: "source-1", State: workersessions.StateCanceled},
		Successor: workersessions.Session{ID: "successor-1", State: workersessions.StateRunning},
	}
	clone := result.Clone()
	if clone.Source.ID != result.Source.ID || clone.Successor.ID != result.Successor.ID || clone.Source.State != workersessions.StateCanceled || clone.Successor.State != workersessions.StateRunning {
		t.Fatalf("InterruptResult.Clone() = %#v, want detached lifecycle snapshots", clone)
	}
}

func TestInterruptErrorPreservesPhaseCauseAndErrorsIsContract(t *testing.T) {
	cause := errors.New("boundary unavailable")
	interruptErr := &workersessions.InterruptError{Phase: workersessions.InterruptPhaseSourceCancellation, Cause: cause}
	if !strings.Contains(interruptErr.Error(), string(workersessions.InterruptPhaseSourceCancellation)) || !strings.Contains(interruptErr.Error(), cause.Error()) {
		t.Fatalf("InterruptError.Error() = %q, want phase and cause", interruptErr.Error())
	}
	if !errors.Is(interruptErr, cause) || !errors.Is(interruptErr, workersessions.ErrInterruptSourceCancellation) || errors.Is(interruptErr, workersessions.ErrInterruptSuccessorAdmission) {
		t.Fatalf("errors.Is() phase/cause matching is incorrect")
	}
	if workersessions.ErrInterruptValidation.Error() != string(workersessions.InterruptPhaseValidation) {
		t.Fatalf("interrupt validation sentinel = %q, want phase name", workersessions.ErrInterruptValidation.Error())
	}
	if !errors.Is(interruptErr, interruptErr) {
		t.Fatal("errors.Is() should match the same InterruptError through Cause traversal")
	}
	if interruptErr.Unwrap() != cause {
		t.Fatalf("InterruptError.Unwrap() = %v, want cause", interruptErr.Unwrap())
	}

	withoutCause := (&workersessions.InterruptError{Phase: workersessions.InterruptPhaseValidation}).Error()
	if !strings.Contains(withoutCause, string(workersessions.InterruptPhaseValidation)) {
		t.Fatalf("InterruptError without cause = %q, want validation phase", withoutCause)
	}
	var nilError *workersessions.InterruptError
	if nilError.Error() != "worker session: interrupt failed" || nilError.Unwrap() != nil || nilError.Is(workersessions.ErrInterruptValidation) {
		t.Fatal("nil InterruptError methods did not preserve safe zero behavior")
	}
}

type providerSessionObservationSpy struct {
	workersessions.Service
	requests []workersessions.ProviderSessionObservationRequest
	err      error
}

func continuationFor(reference providers.SessionRef) *providers.ContinuationRef {
	continuation := reference.ContinuationRef()
	return &continuation
}

func (s *providerSessionObservationSpy) ObserveProviderSession(
	_ context.Context,
	req workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	req.Reference = req.Reference.Clone()
	s.requests = append(s.requests, req)
	return workersessions.ProviderSessionAssociationResult{}, s.err
}

func validPublishRequest() workersessions.PublishRecordRequest {
	return workersessions.PublishRecordRequest{
		SessionID:      "worker-1",
		Draft:          workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: []byte(`{"label":"working"}`)},
		SourceType:     "worker_provider",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "workers.draft.v1",
	}
}

func TestProviderSessionObservationPublisher_FallbackNilReceiverIsSafe(t *testing.T) {
	var publisher *workersessions.ProviderSessionObservationPublisher
	if got := publisher.WithUnassociatedProgressFallback(); got != nil {
		t.Fatalf("nil fallback publisher = %v, want nil", got)
	}
}

func TestProviderSessionObservationPublisher_SuppressesProviderIdentityDisagreement(t *testing.T) {
	observer := &providerSessionObservationSpy{}
	forwarded := 0
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {
		forwarded++
	})
	publisher.Bind(observer)

	publisher.Publish(workers.ProgressFragment{
		DispatchID:   "dispatch-1",
		Provider:     "claude",
		Continuation: continuationFor(providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}),
	})

	if len(observer.requests) != 0 || forwarded != 0 {
		t.Fatalf("contradictory provider identity requests=%#v forwarded=%d, want both suppressed", observer.requests, forwarded)
	}
}

func TestPublish_MalformedCanonicalUsageFallsBackToUsedTokens(t *testing.T) {
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "worker-usage",
		Kind:       workers.ProgressFragmentKind,
		Type:       "usage.updated",
		Payload:    "{malformed",
		Metadata:   map[string]string{"used_tokens": "7"},
	})

	if len(spy.published) != 1 {
		t.Fatalf("published records = %d, want one usage record", len(spy.published))
	}
	if got := spy.published[0].Draft.Kind; got != workers.KindUsage {
		t.Fatalf("draft kind = %q, want %q", got, workers.KindUsage)
	}
	var payload workers.UsagePayload
	if err := json.Unmarshal(spy.published[0].Draft.Payload, &payload); err != nil {
		t.Fatalf("usage payload is not valid JSON: %v", err)
	}
	if payload.TotalTokens != 7 {
		t.Fatalf("usage payload total tokens = %d, want 7", payload.TotalTokens)
	}

	publisher.Publish(workers.ProgressFragment{
		DispatchID: "worker-usage-empty-object",
		Kind:       workers.ProgressFragmentKind,
		Type:       "usage.updated",
		Payload:    `{}`,
		Metadata:   map[string]string{"used_tokens": "8"},
	})
	if len(spy.published) != 2 {
		t.Fatalf("published records after empty canonical object = %d, want two usage records", len(spy.published))
	}
	var emptyObjectFallback workers.UsagePayload
	if err := json.Unmarshal(spy.published[1].Draft.Payload, &emptyObjectFallback); err != nil {
		t.Fatalf("empty canonical object fallback is not valid JSON: %v", err)
	}
	if emptyObjectFallback.TotalTokens != 8 {
		t.Fatalf("empty canonical object fallback total tokens = %d, want 8", emptyObjectFallback.TotalTokens)
	}
}
