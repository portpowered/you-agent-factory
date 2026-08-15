package workersessions_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestProviderSessionObservationPublisher_AssociatesBeforeForwardingExactProgress(t *testing.T) {
	var forwarded []workers.ProgressFragment
	publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
		forwarded = append(forwarded, workers.ProgressFragment{
			DispatchID:   fragment.DispatchID,
			Continuation: (fragment.Continuation).ClonePtr(),
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
		DispatchID:   "dispatch-1",
		Continuation: continuationFor(reference),
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
	if len(forwarded) != 2 || forwarded[1].Continuation == nil || forwarded[1].Continuation.ProviderSessionID != reference.ID {
		t.Fatalf("forwarded fragments = %#v, want exact continuation after observation", forwarded)
	}

	// The live reference hand-off commits the association but is internal
	// bookkeeping, so it cannot add a response-stream observation ahead of
	// provider-authored output.
	publisher.Publish(workers.ProgressFragment{
		DispatchID:   "dispatch-1",
		Kind:         workers.ProviderSessionObservedFragmentKind,
		Continuation: continuationFor(reference),
	})
	if len(first.requests) != 2 || len(forwarded) != 2 {
		t.Fatalf("live reference hand-off requests=%#v forwarded=%#v, want observed without forwarding", first.requests, forwarded)
	}

	fragment.Continuation.ProviderSessionID = "caller-mutated"
	if first.requests[0].Reference.ID != reference.ID || forwarded[1].Continuation.ProviderSessionID != reference.ID {
		t.Fatalf("association or forwarded continuation retained caller mutation: %#v %#v", first.requests[0], forwarded[1])
	}

	// A second Bind cannot reroute a live runtime to another Worker Sessions
	// registry, and a mismatched opaque continuation is not forwarded.
	second := &providerSessionObservationSpy{}
	publisher.Bind(second)
	first.err = workersessions.ErrProviderSessionAssociationAttemptMismatch
	mismatch := workers.ProgressFragment{
		DispatchID: "dispatch-1",
		Continuation: continuationFor(providers.SessionRef{
			Provider: reference.Provider, Kind: reference.Kind, ID: "different-session",
		}),
	}
	publisher.Publish(mismatch)
	if len(first.requests) != 3 || len(second.requests) != 0 || len(forwarded) != 2 {
		t.Fatalf("mismatched fragment rerouted or forwarded: first=%#v second=%#v forwarded=%#v", first.requests, second.requests, forwarded)
	}
	first.err = nil

	// An incomplete opaque continuation remains visible to the response stream,
	// but cannot be reconstructed into a resumable association.
	legacy := workers.ProgressFragment{
		DispatchID:   "dispatch-legacy",
		Continuation: &providers.ContinuationRef{Provider: reference.Provider.String(), Kind: reference.Kind},
	}
	publisher.Publish(legacy)
	if len(first.requests) != 3 || len(forwarded) != 3 ||
		forwarded[2].Continuation == nil ||
		forwarded[2].Continuation.ProviderSessionID != "" {
		t.Fatalf("incomplete continuation association or forwarding = requests:%#v forwarded:%#v", first.requests, forwarded)
	}
	noDownstream := workersessions.NewProviderSessionObservationPublisher(nil)
	noDownstream.Bind(first)
	noDownstream.Publish(legacy)
	if len(first.requests) != 3 {
		t.Fatalf("nil-downstream metadata-only output was observed: %#v", first.requests)
	}
	first.err = errors.New("association rejected")
	publisher.Publish(workers.ProgressFragment{
		DispatchID:   "dispatch-1",
		Continuation: (forwarded[1].Continuation).ClonePtr(),
	})
	if len(first.requests) != 4 || len(forwarded) != 3 {
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
// drafts use the Worker Sessions-owned binding capability before the draft is
// committed and still reach the downstream response publisher exactly once.
func TestPublish_CanonicalDraftBindsBeforeWorkerOutput(t *testing.T) {
	spy := &workerRecordSpy{}
	forwarded := 0
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {
		forwarded++
	})
	publisher.Bind(spy)
	publisher.Publish(workers.CanonicalDraftFragment("worker-1", workers.Draft{
		Kind:       workers.KindMessage,
		Phase:      workers.PhaseCompleted,
		DispatchID: "worker-1",
		Provenance: workers.Provenance{Provider: "codex", NativeEventType: "message.completed", Delivery: workers.DeliveryNativeFinal, Representation: workers.RepresentationSnapshot, Fidelity: workers.FidelityFinalOnly},
		Payload:    []byte(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"done"}]}`),
	}))

	if len(spy.bindings) != 1 || spy.bindings[0].Provider != "codex" || spy.bindings[0].DispatchID != "worker-1" {
		t.Fatalf("provider bindings = %#v, want one codex binding before output", spy.bindings)
	}
	if len(spy.published) != 1 || spy.published[0].Draft.Kind != workers.KindMessage || spy.published[0].Draft.Provenance.Provider != "codex" {
		t.Fatalf("published canonical records = %#v, want one codex MESSAGE record", spy.published)
	}
	if forwarded != 1 {
		t.Fatalf("forwarded canonical fragments = %d, want exactly one", forwarded)
	}
}

// TestPublish_NoProviderSessionReferenceStillBindsAndPreservesProvenance
// proves provider identity is sufficient to attribute a raw provider output
// even when the provider has no resumable native session reference to share.
func TestPublish_NoProviderSessionReferenceStillBindsAndPreservesProvenance(t *testing.T) {
	spy := &workerRecordSpy{}
	var forwarded []workers.ProgressFragment
	publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
		forwarded = append(forwarded, fragment)
	})
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "worker-1",
		Kind:       workers.ProgressFragmentKind,
		Type:       "message.completed",
		Payload:    "final-only output",
		Provider:   "antigravity",
		Metadata:   map[string]string{"item_id": "message-1"},
	})

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
	if len(forwarded) != 1 || forwarded[0].Provider != "antigravity" || forwarded[0].Continuation != nil {
		t.Fatalf("forwarded output = %#v, want provider identity without a synthesized session", forwarded)
	}
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

type recordingLogger struct {
	logging.NoopLogger
	messages []string
}

func (l *recordingLogger) Warn(msg string, args ...any) {
	entry := msg
	for index := 0; index+1 < len(args); index += 2 {
		entry += " " + fmt.Sprint(args[index]) + "=" + fmt.Sprint(args[index+1])
	}
	l.messages = append(l.messages, entry)
}

// TestPublish_ReportsARejectedWorkerRecordWithoutFailingTheDispatch covers the
// race this routing must survive: a Worker's terminal record can begin
// committing while a final observation is still in flight, and PublishRecord
// then refuses it. Losing that one record is acceptable; losing it silently is
// not, because a Worker whose output stops early would be undiagnosable.
func TestPublish_ReportsARejectedWorkerRecordWithoutFailingTheDispatch(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantOutcome string
	}{
		{"closed window", workersessions.ErrPublicationNotOpen, "outcome=publication_not_open"},
		{"out of order", workersessions.ErrOutOfOrderPublication, "outcome=out_of_order"},
		{"unknown session", workersessions.ErrSessionNotFound, "outcome=session_not_found"},
		{"anything else", errors.New("boom"), "outcome=rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := &recordingLogger{}
			var forwarded int
			publisher := workersessions.NewProviderSessionObservationPublisher(
				func(workers.ProgressFragment) { forwarded++ }).WithLogger(logger)
			publisher.Bind(&rejectingWorkerRecordSpy{err: tc.err})

			publisher.Publish(workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "hello",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			})

			if len(logger.messages) != 1 {
				t.Fatalf("warnings = %v, want exactly one -- a dropped observation must be diagnosable", logger.messages)
			}
			if !strings.Contains(logger.messages[0], tc.wantOutcome) {
				t.Fatalf("warning = %q, want it to carry %q", logger.messages[0], tc.wantOutcome)
			}
			if strings.Contains(logger.messages[0], "hello") {
				t.Fatalf("warning leaked payload content: %q", logger.messages[0])
			}
			// The dispatch continues regardless: Publish has a no-return
			// signature precisely so one record cannot fail a Worker.
			if forwarded != 1 {
				t.Fatalf("forwarded = %d, want 1 even though publication was rejected", forwarded)
			}
		})
	}
}

// TestPublish_IgnoresFragmentsThatNameNoWorkerSession covers the guards that
// keep a malformed or unroutable observation from reaching PublishRecord.
func TestPublish_IgnoresFragmentsThatNameNoWorkerSession(t *testing.T) {
	cases := []struct {
		name     string
		bind     bool
		fragment workers.ProgressFragment
	}{
		{
			name: "no dispatch to address",
			bind: true,
			fragment: workers.ProgressFragment{
				Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "hi",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
		{
			name: "no bound worker sessions service",
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "hi",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
		{
			name: "an unrecognized phase word",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "sideways", Payload: "hi",
				Metadata: map[string]string{"kind": "message", "item_id": "m1"},
			},
		},
		{
			name: "a native type carrying no phase at all",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "undotted", Payload: "hi",
			},
		},
		{
			name: "a file change with no path",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "file_change", "item_id": "f1", "operation": "created"},
			},
		},
		{
			name: "usage with no token count",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "usage", "item_id": "usage"},
			},
		},
		{
			name: "a tool fact with no tool call id",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "updated", Payload: "t",
				Metadata: map[string]string{"kind": "tool", "status": "completed"},
			},
		},
		{
			name: "generic progress with nothing to label it",
			bind: true,
			fragment: workers.ProgressFragment{
				DispatchID: "d1", Kind: workers.ProgressFragmentKind,
				Metadata: map[string]string{"kind": "mystery"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &workerRecordSpy{}
			publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
			if tc.bind {
				publisher.Bind(spy)
			}
			publisher.Publish(tc.fragment)
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
			spy := &workerRecordSpy{}
			publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
			publisher.Bind(spy)
			publisher.Publish(tc.fragment)

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
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "d1", Kind: workers.ResponseFragmentKind, Type: "delta", Payload: "final",
		Metadata: map[string]string{"kind": "message", "item_id": "m1"},
	})
	if len(spy.published) != 1 {
		t.Fatalf("published %d record(s), want the response fragment committed too", len(spy.published))
	}
}

// TestWithLogger_IsSafeOnANilPublisher keeps the option usable in the same
// nil-tolerant style as the rest of this type.
func TestWithLogger_IsSafeOnANilPublisher(t *testing.T) {
	var publisher *workersessions.ProviderSessionObservationPublisher
	if got := publisher.WithLogger(&recordingLogger{}); got != nil {
		t.Fatalf("WithLogger() on a nil publisher = %v, want nil", got)
	}
}

// TestPublish_DropsFactsThatCannotBecomeALegalRecord covers the conversions
// that resolve to a Kind/Phase pair or payload the Workers vocabulary refuses.
// Dropping them here is what keeps PublishRecord from rejecting a record and
// losing the observation with no explanation.
func TestPublish_DropsFactsThatCannotBecomeALegalRecord(t *testing.T) {
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
			spy := &workerRecordSpy{}
			publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
			publisher.Bind(spy)
			publisher.Publish(tc.fragment)
			if len(spy.published) != 0 {
				t.Fatalf("published %+v, want nothing committed", spy.published)
			}
		})
	}
}

// TestPublish_CoversTheRemainingPhaseVocabulary exercises the phase words and
// tool statuses the other tables do not reach.
func TestPublish_CoversTheRemainingPhaseVocabulary(t *testing.T) {
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
			spy := &workerRecordSpy{}
			publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
			publisher.Bind(spy)
			publisher.Publish(tc.fragment)
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

// TestPublish_WithoutALoggerStaysSilentAndSafe proves reporting is optional:
// a publisher constructed without a logger must still never fail a dispatch
// when a record is refused.
func TestPublish_WithoutALoggerStaysSilentAndSafe(t *testing.T) {
	var forwarded int
	publisher := workersessions.NewProviderSessionObservationPublisher(
		func(workers.ProgressFragment) { forwarded++ })
	publisher.Bind(&rejectingWorkerRecordSpy{err: workersessions.ErrPublicationNotOpen})
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "d1", Kind: workers.ProgressFragmentKind, Type: "delta", Payload: "hello",
		Metadata: map[string]string{"kind": "message", "item_id": "m1"},
	})
	if forwarded != 1 {
		t.Fatalf("forwarded = %d, want 1", forwarded)
	}
}
