package workersessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishCapturedUsagePreservesNativeTokenClasses(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := newCapturedPublisher(spy, func(workers.ProgressFragment) {})
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "usage.updated",
		Payload: `{"input_tokens":0,"cached_input_tokens":5,"output_tokens":7,"reasoning_output_tokens":3}`,
	})
	if len(spy.published) != 1 || spy.published[0].Draft.Kind != workers.KindUsage || spy.published[0].Draft.Phase != workers.PhaseUpdated {
		t.Fatalf("native usage did not become a capturable observation: %+v", spy.published)
	}
	var usage map[string]int64
	if err := json.Unmarshal(spy.published[0].Draft.Payload, &usage); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{"inputTokens": 0, "cachedInputTokens": 5, "outputTokens": 7, "reasoningOutputTokens": 3} {
		if got, exists := usage[key]; !exists || got != want {
			t.Fatalf("captured %s = %d, present=%t, want %d", key, got, exists, want)
		}
	}
	if _, exists := usage["totalTokens"]; exists {
		t.Fatal("capture invented an unreported total token count")
	}
	for _, detail := range []string{`{"total_tokens":0}`, `{"totalTokens":0}`} {
		publisher.Publish(workers.ProgressFragment{
			DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "usage.updated", Payload: detail,
		})
		var reported map[string]int64
		if err := json.Unmarshal(spy.published[len(spy.published)-1].Draft.Payload, &reported); err != nil {
			t.Fatal(err)
		}
		if total, exists := reported["totalTokens"]; !exists || total != 0 {
			t.Fatalf("capture lost explicit zero total: %s", detail)
		}
	}
}

// TestPublishRecordRequest_Validate_AcceptsWellFormedRequest proves a request
// whose SessionID, complete Events identity, SchemaID, and Draft are each
// individually well-formed passes Validate unchanged.
func TestPublishRecordRequest_Validate_AcceptsWellFormedRequest(t *testing.T) {
	if err := validPublishRequest().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsBlankSessionID proves Validate
// checks SessionID before ever inspecting the Events identity, SchemaID, or
// Draft.
func TestPublishRecordRequest_Validate_RejectsBlankSessionID(t *testing.T) {
	req := validPublishRequest()
	req.SessionID = "   "
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Validate() error = %v, want ErrInvalidSessionID", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsMalformedEventsIdentity proves
// Validate delegates the four-part Events idempotency identity to
// events.AppendIdentity.Validate rather than reimplementing its rules.
func TestPublishRecordRequest_Validate_RejectsMalformedEventsIdentity(t *testing.T) {
	req := validPublishRequest()
	req.SourceType = ""
	if err := req.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a non-nil error for an empty SourceType")
	}
}

// TestPublishRecordRequest_Validate_RejectsMalformedSchemaID proves Validate
// checks SchemaID even once SessionID and the Events identity are both
// well-formed.
func TestPublishRecordRequest_Validate_RejectsMalformedSchemaID(t *testing.T) {
	req := validPublishRequest()
	req.SchemaID = ""
	if err := req.Validate(); !errors.Is(err, events.ErrEmptySchemaID) {
		t.Fatalf("Validate() error = %v, want ErrEmptySchemaID", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsInvalidDraft proves Validate's
// final check is the existing workers.ValidateDraft rules applied to
// req.Draft.
func TestPublishRecordRequest_Validate_RejectsInvalidDraft(t *testing.T) {
	req := validPublishRequest()
	req.Draft = workers.Draft{}
	if err := req.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a non-nil error for a zero-value Draft")
	}
}

func TestProviderSessionObservationRequest_Validate(t *testing.T) {
	valid := workersessions.ProviderSessionObservationRequest{
		DispatchID: "dispatch-1",
		Reference:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid observation request error = %v, want nil", err)
	}

	blankDispatch := valid
	blankDispatch.DispatchID = " "
	if err := blankDispatch.Validate(); !errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation) {
		t.Fatalf("blank dispatch error = %v, want ErrInvalidProviderSessionAssociation", err)
	}

	invalidReference := valid
	invalidReference.Reference.ID = ""
	if err := invalidReference.Validate(); !errors.Is(err, providers.ErrInvalidSessionRef) {
		t.Fatalf("invalid reference error = %v, want Providers ErrInvalidSessionRef", err)
	}
}

// workerRecordSpy captures binding order and committed Worker records.
type workerRecordSpy struct {
	workersessions.Service
	published []workersessions.PublishRecordRequest
	bindings  []workersessions.ProviderBindingRequest
}

func (s *workerRecordSpy) ObserveProviderSession(
	context.Context, workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *workerRecordSpy) PublishRecord(
	_ context.Context, req workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	if req.Draft.Provenance.Provider != "" && len(s.bindings) == 0 {
		return workersessions.PublishRecordResult{}, errors.New("provider output arrived before binding")
	}
	s.published = append(s.published, req)
	return workersessions.PublishRecordResult{}, nil
}

func (s *workerRecordSpy) EnsureProviderBinding(
	_ context.Context,
	req workersessions.ProviderBindingRequest,
) (workersessions.ProviderBindingResult, error) {
	s.bindings = append(s.bindings, req)
	return workersessions.ProviderBindingResult{
		WorkerSessionID: "worker-1",
		DispatchID:      req.DispatchID,
		Provider:        req.Provider,
		Outcome:         workersessions.ProviderBindingOutcomeAccepted,
	}, nil
}

func (s *workerRecordSpy) WorkerSessionIDForDispatch(
	_ context.Context,
	dispatchID string,
) (string, error) {
	return dispatchID, nil
}

// TestPublish_CanonicalDraftBindsBeforeWorkerOutput proves canonical provider
// drafts bind the explicitly selected Worker before its record is committed.
func TestPublish_CanonicalDraftBindsBeforeWorkerOutput(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := &workersessions.ProviderSessionObservationPublisher{}
	fragment := workers.CanonicalDraftFragment("attempt-1", workers.Draft{
		Kind:       workers.KindMessage,
		Phase:      workers.PhaseCompleted,
		DispatchID: "attempt-1",
		Provenance: workers.Provenance{Provider: "codex", NativeEventType: "message.completed", Delivery: workers.DeliveryNativeFinal, Representation: workers.RepresentationSnapshot, Fidelity: workers.FidelityFinalOnly},
		Payload:    []byte(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"done"}]}`),
	})
	fragment.Correlation = workers.ExecutionCorrelation{DispatchID: "dispatch-1", AttemptID: "attempt-1"}
	if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", fragment); err != nil {
		t.Fatal(err)
	}

	if len(spy.bindings) != 1 || spy.bindings[0].Provider != "codex" || spy.bindings[0].DispatchID != "attempt-1" || spy.bindings[0].WorkerSessionID != "selected-worker" {
		t.Fatalf("provider bindings = %#v, want one codex binding before output", spy.bindings)
	}
	if len(spy.published) != 1 || spy.published[0].Draft.Kind != workers.KindMessage || spy.published[0].Draft.Provenance.Provider != "codex" {
		t.Fatalf("published canonical records = %#v, want one codex MESSAGE record", spy.published)
	}
	if spy.published[0].SessionID != "selected-worker" || spy.published[0].Draft.DispatchID != "attempt-1" {
		t.Fatalf("record = %#v, want selected Worker and physical attempt", spy.published[0])
	}
}

// TestPublish_NoProviderSessionReferenceStillBindsAndPreservesProvenance
// proves provider identity is sufficient to attribute a raw provider output
// even when the provider has no resumable native session reference to share.
func TestPublish_NoProviderSessionReferenceStillBindsAndPreservesProvenance(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := &workersessions.ProviderSessionObservationPublisher{}
	if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", workers.ProgressFragment{
		DispatchID:  "dispatch-1",
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1", AttemptID: "attempt-1"},
		Kind:        workers.ProgressFragmentKind,
		Type:        "message.completed",
		Payload:     "final-only output",
		Provider:    "antigravity",
		Metadata:    map[string]string{"item_id": "message-1"},
	}); err != nil {
		t.Fatal(err)
	}

	if len(spy.bindings) != 1 || spy.bindings[0].Provider != "antigravity" {
		t.Fatalf("provider bindings = %#v, want one antigravity binding", spy.bindings)
	}
	if len(spy.published) != 1 {
		t.Fatalf("published records = %#v, want one output record", spy.published)
	}
	output := spy.published[0].Draft
	if output.Provenance.Provider != "antigravity" || output.Kind != workers.KindMessage || output.Phase != workers.PhaseCompleted {
		t.Fatalf("output draft = %#v, want antigravity MESSAGE/COMPLETED provenance", output)
	}
	if spy.published[0].SessionID != "selected-worker" || output.DispatchID != "attempt-1" {
		t.Fatalf("record = %#v, want selected Worker and physical attempt", spy.published[0])
	}
}

// Worker output is committed to the selected Worker's topic. Runtime's bound
// progress operation owns downstream forwarding, which its own tests protect.
//
// Every committed Draft must satisfy workers.ValidateDraft. That is the point:
// an invalid Draft is rejected by PublishRecord and the observation is lost
// with no diagnostic, which is the failure this routing exists to remove.
// workerOutputCase is one provider fact and the record it must commit as.
type workerOutputCase struct {
	name      string
	fragment  workers.ProgressFragment
	wantKind  workers.Kind
	wantPhase workers.Phase
	wantNone  bool
}

// workerOutputCases covers both provider vocabularies a Factory can dispatch:
// ACP execution, which names its fact kind in metadata, and the native
// adapters, which put a dotted "noun.phase" in the fragment type.
func workerOutputCases() []workerOutputCase {
	return []workerOutputCase{
		{
			name: "acp message delta",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "hello",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
			wantKind: workers.KindMessage, wantPhase: workers.PhaseDelta,
		},
		{
			name: "acp reasoning delta",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "thinking",
				Metadata: map[string]string{"kind": "reasoning", "item_id": "r1"},
			},
			wantKind: workers.KindReasoning, wantPhase: workers.PhaseDelta,
		},
		{
			// A tool_call_update arrives with a bare "updated" phase, which is
			// not legal for TOOL. Status is what carries the transition.
			name: "acp tool update resolves its phase from status",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "Inspect Factory",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "completed"},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseCompleted,
		},
		{
			name: "acp file change keeps its owning tool call",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "a.txt",
				Metadata: map[string]string{
					"kind": "file_change", "item_id": "file:a.txt",
					"path": "a.txt", "operation": "created", "tool_call_id": "t1",
				},
			},
			wantKind: workers.KindFileChange, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "native codex reasoning delta",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "reasoning.delta", Payload: "thinking",
				Metadata: map[string]string{"item_id": "r1"},
			},
			wantKind: workers.KindReasoning, wantPhase: workers.PhaseDelta,
		},
		{
			// A provider's own session metadata is not this Worker Session's
			// lifecycle and must not collide with its opening/terminal records.
			name: "acp session metadata degrades to progress",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "New title",
				Metadata: map[string]string{"kind": "session", "item_id": "session", "native_type": "session_info_update"},
			},
			wantKind: workers.KindProgress, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "a status-only tool update carries nothing new",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "in_progress"},
			},
			wantNone: true,
		},
	}
}

func TestPublishWorkerSessionProgress_CommitsWorkerOutputAsValidRecords(t *testing.T) {
	t.Parallel()
	for _, tc := range workerOutputCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			tc.fragment.Correlation = workers.ExecutionCorrelation{DispatchID: tc.fragment.DispatchID, AttemptID: "attempt-1"}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", tc.fragment); err != nil {
				t.Fatal(err)
			}
			if tc.wantNone {
				if len(spy.published) != 0 {
					t.Fatalf("published %d record(s), want none: %+v", len(spy.published), spy.published)
				}
				return
			}
			if len(spy.published) != 1 {
				t.Fatalf("published %d record(s), want exactly 1", len(spy.published))
			}
			req := spy.published[0]
			if req.SessionID != "selected-worker" || req.Draft.DispatchID != "attempt-1" {
				t.Fatalf("record = %#v, want selected Worker and physical attempt", req)
			}
			if req.Draft.Kind != tc.wantKind || req.Draft.Phase != tc.wantPhase {
				t.Fatalf("draft = %q/%q, want %q/%q",
					req.Draft.Kind, req.Draft.Phase, tc.wantKind, tc.wantPhase)
			}
			if err := req.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil -- an invalid record is a lost observation", err)
			}
		})
	}
}

// Equal dispatch and physical attempt IDs do not combine distinct Workers'
// source sequences. The caller supplies the already-resolved Worker identity.
func TestPublish_KeepsEachWorkerSessionSequenceIndependent(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := &workersessions.ProviderSessionObservationPublisher{}

	for _, dispatch := range []string{"d1", "d2", "d1", "d2", "d1"} {
		if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, dispatch, workers.ProgressFragment{
			DispatchID: "shared-dispatch", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "chunk",
			Correlation: workers.ExecutionCorrelation{DispatchID: "shared-dispatch", AttemptID: "shared-attempt"},
			Metadata:    map[string]string{"kind": "message", "item_id": "m1"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	bySession := map[string][]uint64{}
	for _, req := range spy.published {
		bySession[req.SessionID] = append(bySession[req.SessionID], uint64(req.SourceSequence))
	}
	for session, sequences := range bySession {
		for index, sequence := range sequences {
			if sequence != uint64(index+1) {
				t.Fatalf("%s sequences = %v, want 1..n independent of every other Worker", session, sequences)
			}
		}
	}
	if len(bySession["d1"]) != 3 || len(bySession["d2"]) != 2 {
		t.Fatalf("records per session = %v, want d1:3 d2:2", bySession)
	}
}

func TestPublishWorkerSessionProgress_RejectsCanonicalAttemptMismatchBeforeBinding(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := &workersessions.ProviderSessionObservationPublisher{}
	fragment := workers.CanonicalDraftFragment("logical-dispatch", workers.Draft{
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		DispatchID: "stale-attempt", Provenance: workers.Provenance{Provider: "codex"},
		Payload: []byte(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"late"}]}`),
	})
	fragment.Correlation = workers.ExecutionCorrelation{DispatchID: "logical-dispatch", AttemptID: "current-attempt"}
	if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", fragment); !errors.Is(err, workersessions.ErrProviderBindingAttemptMismatch) {
		t.Fatalf("publication error = %v, want physical attempt mismatch", err)
	}
	if len(spy.bindings) != 0 || len(spy.published) != 0 {
		t.Fatalf("stale output mutated binding/records: %#v %#v", spy.bindings, spy.published)
	}
}

func TestPublishWorkerSessionProgress_ReturnsRecordRejection(t *testing.T) {
	t.Parallel()
	for _, want := range []error{workersessions.ErrPublicationNotOpen, workersessions.ErrOutOfOrderPublication, workersessions.ErrSessionNotFound} {
		t.Run(want.Error(), func(t *testing.T) {
			t.Parallel()
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			fragment := workers.ProgressFragment{
				DispatchID: "logical-dispatch", Kind: workers.ProgressFragmentKind,
				Type: "message.delta", Payload: "hello",
				Correlation: workers.ExecutionCorrelation{DispatchID: "logical-dispatch", AttemptID: "physical-attempt"},
			}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), &rejectingWorkerRecordSpy{err: want}, "selected-worker", fragment); !errors.Is(err, want) {
				t.Fatalf("publication error = %v, want %v", err, want)
			}
		})
	}
}

// rejectingWorkerRecordSpy fails every publication so the drop path is
// exercised rather than assumed.
type rejectingWorkerRecordSpy struct {
	workersessions.Service
	err error
}

func (s *rejectingWorkerRecordSpy) ObserveProviderSession(
	context.Context, workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *rejectingWorkerRecordSpy) PublishRecord(
	context.Context, workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, s.err
}

func (s *rejectingWorkerRecordSpy) WorkerSessionIDForDispatch(
	_ context.Context,
	dispatchID string,
) (string, error) {
	return dispatchID, nil
}

// TestPublish_IgnoresUnrecognizedOrIncompleteFacts covers the guards that
// keep a malformed or unroutable observation from reaching PublishRecord.
func TestPublish_IgnoresUnrecognizedOrIncompleteFacts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fragment workers.ProgressFragment
	}{
		{
			name: "an unrecognized phase word",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "sideways", Payload: "hi",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
		{
			name: "a native type carrying no phase at all",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "undotted", Payload: "hi",
			},
		},
		{
			name: "a file change with no path",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "file_change", "item_id": "f1", "operation": "created"},
			},
		},
		{
			name: "usage with no token count",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "usage", "item_id": "usage"},
			},
		},
		{
			name: "a tool fact with no tool call id",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "t",
				Metadata: map[string]string{"kind": "tool", "status": "completed"},
			},
		},
		{
			name: "generic progress with nothing to label it",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind,
				Metadata: map[string]string{"kind": "mystery"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &workerRecordSpy{}
			t.Parallel()
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			fragment := tc.fragment
			fragment.Correlation = workers.ExecutionCorrelation{DispatchID: "logical", AttemptID: "physical"}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", fragment); err != nil {
				t.Fatal(err)
			}
			if len(spy.published) != 0 {
				t.Fatalf("published %+v, want nothing committed", spy.published)
			}
		})
	}
}

// TestPublish_CommitsRemainingWorkerVocabulary covers the fact kinds the
// primary table does not, so every branch that can reach a Worker topic is
// exercised at least once.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestPublish_CommitsRemainingWorkerVocabulary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		fragment  workers.ProgressFragment
		wantKind  workers.Kind
		wantPhase workers.Phase
	}{
		{
			name: "acp plan with structured entries",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{
					"kind": "plan", "item_id": "plan",
					"entries": `[{"content":"Finish the turn","priority":"high","status":"in_progress"},{"content":""}]`,
				},
			},
			wantKind: workers.KindPlan, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "a plan with no structure falls back to its summary",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "do the thing",
				Metadata: map[string]string{"kind": "plan", "item_id": "plan"},
			},
			wantKind: workers.KindPlan, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "acp usage",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "usage", "item_id": "usage", "used_tokens": "17"},
			},
			wantKind: workers.KindUsage, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "a provider error",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "failed",
				Metadata: map[string]string{"kind": "error", "item_id": "e1"},
			},
			wantKind: workers.KindError, wantPhase: workers.PhaseUpdated,
		},
		{
			name: "a run start",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "started",
				Metadata: map[string]string{"kind": "run", "item_id": "run"},
			},
			wantKind: workers.KindRun, wantPhase: workers.PhaseStarted,
		},
		{
			name: "a tool call opening",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "started", Payload: "Inspect",
				Metadata: map[string]string{
					"kind": "tool", "item_id": "t1", "status": "pending", "raw_input": `{"scope":"factory"}`,
				},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseStarted,
		},
		{
			name: "a cancelled tool call",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "Inspect",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "cancelled"},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseCanceled,
		},
		{
			name: "a failed tool call",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "Inspect",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "failed"},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseFailed,
		},
		{
			name: "a tool output increment with no title",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{
					"kind": "tool", "item_id": "t1", "status": "in_progress", "raw_output": `{"ok":true}`,
				},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseDelta,
		},
		{
			name: "an unnamed tool call still commits",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "started",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "pending"},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseStarted,
		},
		{
			name: "a completed native message snapshot",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "message.completed", Payload: "done",
				Metadata: map[string]string{"item_id": "m1", "partial": "true"},
			},
			wantKind: workers.KindMessage, wantPhase: workers.PhaseCompleted,
		},
		{
			name: "a completed native reasoning snapshot",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "reasoning.completed", Payload: "thought",
				Metadata: map[string]string{"item_id": "r1"},
			},
			wantKind: workers.KindReasoning, wantPhase: workers.PhaseCompleted,
		},
		{
			name: "a cancelled run",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "canceled",
				Metadata: map[string]string{"kind": "turn", "item_id": "turn"},
			},
			wantKind: workers.KindRun, wantPhase: workers.PhaseCanceled,
		},
		{
			name: "an unrecognized fact kind becomes labelled progress",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Payload: "detail",
				Metadata: map[string]string{"kind": "mystery", "native_type": "vendor/thing"},
			},
			wantKind: workers.KindProgress, wantPhase: workers.PhaseUpdated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			tc.fragment.Correlation = workers.ExecutionCorrelation{DispatchID: tc.fragment.DispatchID, AttemptID: "attempt-1"}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", tc.fragment); err != nil {
				t.Fatal(err)
			}

			if len(spy.published) != 1 {
				t.Fatalf("published %d record(s), want exactly 1", len(spy.published))
			}
			req := spy.published[0]
			if req.Draft.Kind != tc.wantKind || req.Draft.Phase != tc.wantPhase {
				t.Fatalf("draft = %q/%q, want %q/%q",
					req.Draft.Kind, req.Draft.Phase, tc.wantKind, tc.wantPhase)
			}
			if err := req.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

// TestPublish_ResponseFragmentsAlsoReachTheWorkerTopic covers the second
// Worker-authored fragment kind: the runner's own terminal content.
func TestPublish_ResponseFragmentsAlsoReachTheWorkerTopic(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := &workersessions.ProviderSessionObservationPublisher{}
	if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", workers.ProgressFragment{
		DispatchID: "d1", Kind: workers.ResponseFragmentKind, Type: "delta", Payload: "final",
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1", AttemptID: "attempt-1"},
		Metadata:    map[string]string{"kind": "message", "item_id": "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(spy.published) != 1 {
		t.Fatalf("published %d record(s), want the response fragment committed too", len(spy.published))
	}
}

// TestPublish_DropsFactsThatCannotBecomeALegalRecord covers the conversions
// that resolve to a Kind/Phase pair or payload the Workers vocabulary refuses.
// Dropping them here is what keeps PublishRecord from rejecting a record and
// losing the observation with no explanation.
func TestPublish_DropsFactsThatCannotBecomeALegalRecord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fragment workers.ProgressFragment
	}{
		{
			// MESSAGE has no CANCELED phase. The pair is resolved here but
			// refused by workers.ValidateDraft.
			name: "a message phase the vocabulary does not declare",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "canceled", Payload: "hi",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
		{
			name: "a message delta with no text",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			tc.fragment.Correlation = workers.ExecutionCorrelation{DispatchID: tc.fragment.DispatchID, AttemptID: "attempt-1"}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", tc.fragment); err != nil {
				t.Fatal(err)
			}
			if len(spy.published) != 0 {
				t.Fatalf("published %+v, want nothing committed", spy.published)
			}
		})
	}
}

// TestPublish_CoversTheRemainingPhaseVocabulary exercises the phase words and
// tool statuses the other tables do not reach.
func TestPublish_CoversTheRemainingPhaseVocabulary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		fragment  workers.ProgressFragment
		wantKind  workers.Kind
		wantPhase workers.Phase
	}{
		{
			// A provider reporting an ongoing message change means DELTA;
			// no content kind declares UPDATED.
			name: "a message reported as updated is an increment",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "more",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
			wantKind: workers.KindMessage, wantPhase: workers.PhaseDelta,
		},
		{
			name: "an unrecognized tool status is treated as an increment",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "partial output",
				Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "vendor-specific"},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseDelta,
		},
		{
			name: "a completed tool call carries its raw output",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "Inspect",
				Metadata: map[string]string{
					"kind": "tool", "item_id": "t1", "status": "completed", "raw_output": `{"ok":true}`,
				},
			},
			wantKind: workers.KindTool, wantPhase: workers.PhaseCompleted,
		},
		{
			name: "a native message start",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "message.started", Payload: "hi",
				Metadata: map[string]string{"item_id": "m1"},
			},
			wantKind: workers.KindMessage, wantPhase: workers.PhaseStarted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			publisher := &workersessions.ProviderSessionObservationPublisher{}
			tc.fragment.Correlation = workers.ExecutionCorrelation{DispatchID: tc.fragment.DispatchID, AttemptID: "attempt-1"}
			if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", tc.fragment); err != nil {
				t.Fatal(err)
			}
			if len(spy.published) != 1 {
				t.Fatalf("published %d record(s), want exactly 1", len(spy.published))
			}
			req := spy.published[0]
			if req.Draft.Kind != tc.wantKind || req.Draft.Phase != tc.wantPhase {
				t.Fatalf("draft = %q/%q, want %q/%q", req.Draft.Kind, req.Draft.Phase, tc.wantKind, tc.wantPhase)
			}
			if err := req.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}
