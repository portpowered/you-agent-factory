package workersessions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type providerSessionObservationSpy struct {
	workersessions.Service
	requests []workersessions.ProviderSessionObservationRequest
	err      error
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

func TestProviderSessionObservationPublisher_AssociatesBeforeForwardingExactProgress(t *testing.T) {
	var forwarded []workers.ProgressFragment
	publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
		forwarded = append(forwarded, workers.ProgressFragment{
			DispatchID:               fragment.DispatchID,
			ProviderSessionReference: workers.CloneProviderSessionReference(fragment.ProviderSessionReference),
			ProviderSessionRef:       workers.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		})
	})

	// Plain output remains available even before the Worker Sessions service is
	// bound because it does not claim a Provider Session association.
	publisher.Publish(workers.ProgressFragment{DispatchID: "dispatch-plain"})
	if len(forwarded) != 1 {
		t.Fatalf("plain forwarded fragments = %#v, want one", forwarded)
	}

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	fragment := workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: reference.ID,
		},
	}
	// Reference-bearing output cannot leak before its Worker Session observer
	// exists.
	publisher.Publish(fragment)
	if len(forwarded) != 1 {
		t.Fatalf("unbound reference fragments = %#v, want no forward", forwarded)
	}

	first := &providerSessionObservationSpy{}
	publisher.Bind(nil)
	publisher.Bind(first)
	publisher.Publish(fragment)
	if len(first.requests) != 1 || first.requests[0].DispatchID != "dispatch-1" || first.requests[0].Reference != reference {
		t.Fatalf("observed requests = %#v, want exact dispatch/reference", first.requests)
	}
	if len(forwarded) != 2 || forwarded[1].ProviderSessionReference == nil || *forwarded[1].ProviderSessionReference != reference {
		t.Fatalf("forwarded fragments = %#v, want exact reference after observation", forwarded)
	}

	// The live reference hand-off commits the association but is internal
	// bookkeeping, so it cannot add a response-stream observation ahead of
	// provider-authored output.
	publisher.Publish(workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		Kind:                     workers.ProviderSessionObservedFragmentKind,
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: reference.ID,
		},
	})
	if len(first.requests) != 2 || len(forwarded) != 2 {
		t.Fatalf("live reference hand-off requests=%#v forwarded=%#v, want observed without forwarding", first.requests, forwarded)
	}

	fragment.ProviderSessionReference.ID = "caller-mutated"
	if first.requests[0].Reference.ID != reference.ID || forwarded[1].ProviderSessionReference.ID != reference.ID {
		t.Fatalf("association or forwarded reference retained caller mutation: %#v %#v", first.requests[0], forwarded[1])
	}

	// A second Bind cannot reroute a live runtime to another Worker Sessions
	// registry, and mismatched public metadata is not forwarded.
	second := &providerSessionObservationSpy{}
	publisher.Bind(second)
	mismatch := workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: "different-session",
		},
	}
	publisher.Publish(mismatch)
	if len(first.requests) != 2 || len(second.requests) != 0 || len(forwarded) != 2 {
		t.Fatalf("mismatched fragment rerouted or forwarded: first=%#v second=%#v forwarded=%#v", first.requests, second.requests, forwarded)
	}

	// Metadata-only compatibility output is not a provider-native SessionRef.
	// It must remain visible to the response stream, but cannot be reconstructed
	// into a resumable association.
	legacy := workers.ProgressFragment{
		DispatchID: "dispatch-legacy",
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: "legacy-session",
		},
	}
	publisher.Publish(legacy)
	if len(first.requests) != 2 || len(forwarded) != 3 ||
		forwarded[2].ProviderSessionReference != nil ||
		forwarded[2].ProviderSessionRef == nil ||
		forwarded[2].ProviderSessionRef.ID != "legacy-session" {
		t.Fatalf("metadata-only output association or forwarding = requests:%#v forwarded:%#v", first.requests, forwarded)
	}
	noDownstream := workersessions.NewProviderSessionObservationPublisher(nil)
	noDownstream.Bind(first)
	noDownstream.Publish(legacy)
	if len(first.requests) != 2 {
		t.Fatalf("nil-downstream metadata-only output was observed: %#v", first.requests)
	}
	first.err = errors.New("association rejected")
	publisher.Publish(workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef:       workers.CloneProviderSessionMetadata(forwarded[1].ProviderSessionRef),
	})
	if len(first.requests) != 3 || len(forwarded) != 3 {
		t.Fatalf("rejected observation forwarded output: requests:%#v forwarded:%#v", first.requests, forwarded)
	}

	var nilPublisher *workersessions.ProviderSessionObservationPublisher
	nilPublisher.Bind(first)
	nilPublisher.Publish(fragment)
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

// workerRecordSpy captures what Publish commits to a Worker Session topic.
type workerRecordSpy struct {
	workersessions.Service
	published []workersessions.PublishRecordRequest
}

func (s *workerRecordSpy) ObserveProviderSession(
	context.Context, workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *workerRecordSpy) PublishRecord(
	_ context.Context, req workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	s.published = append(s.published, req)
	return workersessions.PublishRecordResult{}, nil
}

// TestPublish_CommitsWorkerOutputAsValidRecordsAndStillForwards pins both
// halves of the routing.
//
// A Worker is a tool call, so its output is committed to that Worker's own
// topic, where the Chat Session sequences it as content inside the call. It
// also continues downstream, because the Factory Session response-event stream
// is what the CLI, the dashboard, and the HTTP SSE feed read.
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

func TestPublish_CommitsWorkerOutputAsValidRecordsAndStillForwards(t *testing.T) {
	for _, tc := range workerOutputCases() {
		t.Run(tc.name, func(t *testing.T) {
			spy := &workerRecordSpy{}
			var forwarded []workers.ProgressFragment
			publisher := workersessions.NewProviderSessionObservationPublisher(
				func(fragment workers.ProgressFragment) { forwarded = append(forwarded, fragment) })
			publisher.Bind(spy)
			publisher.Publish(tc.fragment)

			if len(forwarded) != 1 {
				t.Fatalf("forwarded fragments = %d, want 1 -- the CLI, dashboard, and SSE feed read that stream",
					len(forwarded))
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
			if req.SessionID != "d1" {
				t.Fatalf("SessionID = %q, want the dispatch id", req.SessionID)
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

// TestPublish_KeepsEachWorkerSessionSequenceIndependent proves two Workers do
// not share a sequence counter. PublishRecord rejects a regressing
// SourceSequence within one source, so a shared counter would silently drop
// records from whichever Worker fell behind.
func TestPublish_KeepsEachWorkerSessionSequenceIndependent(t *testing.T) {
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
	publisher.Bind(spy)

	for _, dispatch := range []string{"d1", "d2", "d1", "d2", "d1"} {
		publisher.Publish(workers.ProgressFragment{
			DispatchID: dispatch, Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "chunk",
			Metadata: map[string]string{"kind": "message", "item_id": "m1"},
		})
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
