package workersessions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func observationTestProviderSession() providers.SessionRef {
	return providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
}

func testContinuationFromSessionRef(reference *providers.SessionRef) *providers.ContinuationRef {
	if reference == nil {
		return nil
	}
	continuation := reference.ContinuationRef()
	return &continuation
}

func testContinuationFromMetadata(metadata *providers.SessionMetadata) *providers.ContinuationRef {
	return (metadata).ContinuationRef()
}

func validObservationForTest(state State) Observation {
	started := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	observation := Observation{
		WorkerSessionID:          "worker-1",
		ProviderSession:          observationTestProviderSession(),
		ProviderSessionAvailable: true,
		WorkIDs:                  []string{"work-1"},
		TurnID:                   "turn-1",
		AttemptID:                "attempt-1",
		State:                    state,
		StartedAt:                &started,
		DurationBasis:            DurationBasisActiveClock,
		Transcript:               TranscriptAvailabilityAvailable,
	}
	if state.Terminal() {
		ended := started.Add(2 * time.Second)
		duration := 2 * time.Second
		observation.EndedAt = &ended
		observation.Duration = &duration
		observation.DurationBasis = DurationBasisRecordedTimestamps
	}
	return observation
}

func TestObservationRequests_ValidateIdentityAndBounds(t *testing.T) {
	valid := observationTestProviderSession()
	cases := []struct {
		name string
		got  error
		want error
	}{
		{"list valid", (ListObservationsRequest{WorkID: "work-1"}).Validate(), nil},
		{"list blank Factory Session", (ListObservationsRequest{WorkID: "work-1", FactorySessionID: " "}).Validate(), ErrInvalidObservationFactorySessionID},
		{"list blank", (ListObservationsRequest{WorkID: "  "}).Validate(), ErrInvalidObservationWorkID},
		{"get valid", (GetObservationRequest{ProviderSession: valid}).Validate(), nil},
		{"get invalid", (GetObservationRequest{}).Validate(), ErrInvalidObservationIdentity},
		{"get by Worker Session valid", (GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}).Validate(), nil},
		{"get by Worker Session invalid", (GetObservationByWorkerSessionIDRequest{}).Validate(), ErrInvalidSessionID},
		{"read by Worker Session valid", (ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}).Validate(), nil},
		{"read by Worker Session invalid", (ReadTranscriptByWorkerSessionIDRequest{}).Validate(), ErrInvalidSessionID},
		{"stream valid zero limit", (StreamObservationsRequest{ProviderSession: valid}).Validate(), nil},
		{"stream invalid identity", (StreamObservationsRequest{Limit: 1}).Validate(), ErrInvalidObservationIdentity},
		{"stream negative limit", (StreamObservationsRequest{ProviderSession: valid, Limit: -1}).Validate(), ErrInvalidObservationStreamLimit},
		{"stream by Worker Session valid", (StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}).Validate(), nil},
		{"stream by Worker Session invalid", (StreamObservationsByWorkerSessionIDRequest{}).Validate(), ErrInvalidSessionID},
		{"stream by Worker Session negative limit", (StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1", Limit: -1}).Validate(), ErrInvalidObservationStreamLimit},
		{"read valid", (ReadTranscriptRequest{ProviderSession: valid}).Validate(), nil},
		{"read by Worker Session valid", (ReadTranscriptRequest{WorkerSessionID: "worker-1"}).Validate(), nil},
		{"read ambiguous identities", (ReadTranscriptRequest{WorkerSessionID: "worker-1", ProviderSession: valid}).Validate(), ErrInvalidObservationIdentity},
		{"read invalid", (ReadTranscriptRequest{}).Validate(), ErrInvalidObservationIdentity},
		{"top-level list default", (ListWorkerSessionObservationsRequest{}).Validate(), nil},
		{"top-level list direct", (ListWorkerSessionObservationsRequest{Scope: ObservationScopeDirect}).Validate(), nil},
		{"top-level list factory", (ListWorkerSessionObservationsRequest{Scope: ObservationScopeFactory}).Validate(), nil},
		{"top-level list all", (ListWorkerSessionObservationsRequest{Scope: ObservationScopeAll, States: []State{StateCompleted}, MaxResults: 1, NextToken: base64.StdEncoding.EncodeToString([]byte("worker-1"))}).Validate(), nil},
		{"top-level list invalid scope", (ListWorkerSessionObservationsRequest{Scope: "other"}).Validate(), ErrInvalidObservationScope},
		{"top-level list invalid state", (ListWorkerSessionObservationsRequest{States: []State{"INTERRUPTED"}}).Validate(), ErrInvalidState},
		{"top-level list negative max", (ListWorkerSessionObservationsRequest{MaxResults: -1}).Validate(), ErrInvalidObservationPagination},
		{"top-level list malformed cursor", (ListWorkerSessionObservationsRequest{NextToken: "%%%"}).Validate(), ErrInvalidObservationPagination},
		{"top-level list blank cursor", (ListWorkerSessionObservationsRequest{NextToken: base64.StdEncoding.EncodeToString([]byte(" "))}).Validate(), ErrInvalidObservationPagination},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.want == nil {
				if test.got != nil {
					t.Fatalf("Validate() = %v, want nil", test.got)
				}
				return
			}
			if !errors.Is(test.got, test.want) {
				t.Fatalf("Validate() = %v, want %v", test.got, test.want)
			}
		})
	}
	if got := (ObservationScope("")).Normalized(); got != ObservationScopeAll {
		t.Fatalf("empty ObservationScope.Normalized() = %q, want %q", got, ObservationScopeAll)
	}
	if got := ObservationScopeFactory.Normalized(); got != ObservationScopeFactory {
		t.Fatalf("factory ObservationScope.Normalized() = %q, want unchanged factory scope", got)
	}

	validCursor := &ObservationCursor{WorkerSessionID: "worker-1", StreamGenerationID: "generation-1", Position: 1}
	for _, test := range []struct {
		name string
		got  error
		want error
	}{
		{"provider stream valid cursor", (StreamObservationsRequest{ProviderSession: valid, Cursor: validCursor}).Validate(), nil},
		{"provider stream zero cursor position", (StreamObservationsRequest{ProviderSession: valid, Cursor: &ObservationCursor{WorkerSessionID: "worker-1", Position: 0}}).Validate(), ErrInvalidObservationCursor},
		{"provider stream padded worker cursor", (StreamObservationsRequest{ProviderSession: valid, Cursor: &ObservationCursor{WorkerSessionID: " worker-1", Position: 1}}).Validate(), ErrInvalidObservationCursor},
		{"worker stream valid cursor", (StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1", Cursor: validCursor}).Validate(), nil},
		{"worker stream padded generation cursor", (StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1", Cursor: &ObservationCursor{StreamGenerationID: "generation-1 ", Position: 1}}).Validate(), ErrInvalidObservationCursor},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.want == nil {
				if test.got != nil {
					t.Fatalf("Validate() = %v, want nil", test.got)
				}
				return
			}
			if !errors.Is(test.got, test.want) {
				t.Fatalf("Validate() = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestObservation_ValidateLifecycleTimingAndFailure(t *testing.T) {
	validActive := validObservationForTest(StateRunning)
	validTerminal := validObservationForTest(StateCompleted)
	failure := FailureCause{Kind: FailureCauseWorkersExecutionFailure, Detail: "worker failed"}
	cases := []struct {
		name string
		make func() Observation
		want error
	}{
		{"valid active", func() Observation { return validActive }, nil},
		{"valid terminal", func() Observation { return validTerminal }, nil},
		{"missing worker identity", func() Observation { o := validActive; o.WorkerSessionID = " "; return o }, ErrInvalidObservationIdentity},
		{"invalid provider identity", func() Observation { o := validActive; o.ProviderSession.ID = ""; return o }, ErrInvalidObservationIdentity},
		{"invalid state", func() Observation { o := validActive; o.State = State("UNKNOWN"); return o }, ErrInvalidState},
		{"missing attempt", func() Observation { o := validActive; o.AttemptID = ""; return o }, ErrInvalidObservationAttempt},
		{"invalid duration basis", func() Observation { o := validActive; o.DurationBasis = DurationBasis("UNKNOWN"); return o }, ErrInvalidObservationDuration},
		{"invalid transcript availability", func() Observation { o := validActive; o.Transcript = TranscriptAvailability("UNKNOWN"); return o }, ErrObservationProjectionUnavailable},
		{"terminal active clock", func() Observation { o := validTerminal; o.DurationBasis = DurationBasisActiveClock; return o }, ErrInvalidObservationDuration},
		{"active recorded timestamps", func() Observation { o := validActive; o.DurationBasis = DurationBasisRecordedTimestamps; return o }, ErrInvalidObservationDuration},
		{"unavailable with duration", func() Observation {
			o := validActive
			duration := time.Second
			o.DurationBasis = DurationBasisUnavailable
			o.Duration = &duration
			return o
		}, ErrInvalidObservationDuration},
		{"negative duration", func() Observation { o := validActive; duration := -time.Second; o.Duration = &duration; return o }, ErrInvalidObservationDuration},
		{"end precedes start", func() Observation {
			o := validTerminal
			ended := o.StartedAt.Add(-time.Second)
			o.EndedAt = &ended
			return o
		}, ErrInvalidObservationDuration},
		{"self predecessor lineage", func() Observation {
			o := validActive
			o.PredecessorWorkerSessionID = o.WorkerSessionID
			return o
		}, ErrInvalidContinuationLineage},
		{"malformed successor lineage", func() Observation {
			o := validActive
			o.SuccessorWorkerSessionID = "   "
			return o
		}, ErrInvalidContinuationLineage},
		{"valid failed cause", func() Observation { o := validObservationForTest(StateFailed); o.Failure = &failure; return o }, nil},
		{"invalid failed cause", func() Observation {
			o := validObservationForTest(StateFailed)
			bad := failure
			bad.Detail = " "
			o.Failure = &bad
			return o
		}, ErrInvalidFailureCause},
		{"failure on active", func() Observation { o := validActive; o.Failure = &failure; return o }, ErrInvalidObservationFailure},
		{"failure on completed", func() Observation { o := validTerminal; o.Failure = &failure; return o }, ErrInvalidObservationFailure},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := test.make().Validate()
			if test.want == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestObservation_ValidateRecordingHealth(t *testing.T) {
	for _, status := range []recordings.WorkerRecordingStatus{
		recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete,
	} {
		t.Run(string(status), func(t *testing.T) {
			observation := validObservationForTest(StateCompleted)
			observation.RecordingHealth = status
			if err := observation.Validate(); err != nil {
				t.Fatalf("Validate() with %s = %v, want nil", status, err)
			}
		})
	}

	withReason := validObservationForTest(StateCompleted)
	withReason.RecordingHealthReason = "capture interrupted"
	if err := withReason.Validate(); !errors.Is(err, ErrInvalidObservationRecordingHealth) {
		t.Fatalf("Validate() with health reason but no status = %v, want invalid recording health", err)
	}
	unknown := validObservationForTest(StateCompleted)
	unknown.RecordingHealth = recordings.WorkerRecordingStatus("UNKNOWN")
	if err := unknown.Validate(); !errors.Is(err, ErrInvalidObservationRecordingHealth) {
		t.Fatalf("Validate() with unknown health = %v, want invalid recording health", err)
	}
}

type publisherServiceSpy struct {
	Service
	ensureResult ProviderBindingResult
	ensureErr    error
	publishErr   error
}

func publisherTestDraft() workers.Draft {
	return workers.Draft{
		Kind:    workers.KindMessage,
		Phase:   workers.PhaseCompleted,
		Payload: []byte(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"test"}]}`),
	}
}

func (s *publisherServiceSpy) EnsureProviderBinding(
	context.Context,
	ProviderBindingRequest,
) (ProviderBindingResult, error) {
	return s.ensureResult, s.ensureErr
}

func (s *publisherServiceSpy) PublishRecord(context.Context, PublishRecordRequest) (PublishRecordResult, error) {
	return PublishRecordResult{}, s.publishErr
}

func TestPublisher_IdentityAndCanonicalDraftEdges(t *testing.T) {
	reference := &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}
	metadata := &providers.SessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "session-1"}
	assertProviderFragmentIdentity(t, reference, metadata)
	assertCanonicalDraftIdentity(t)
	assertProviderIdentityResolution(t, reference, metadata)
	assertProgressProvenance(t)
}

func assertProviderFragmentIdentity(t *testing.T, reference *providers.SessionRef, metadata *providers.SessionMetadata) {
	t.Helper()
	claudeContinuation := testContinuationFromSessionRef(&providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "session-1"})
	otherContinuation := testContinuationFromSessionRef(&providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "other"})
	continuation := testContinuationFromSessionRef(reference)
	metadataContinuation := testContinuationFromMetadata(metadata)
	if !providerFragmentAgrees(workers.ProgressFragment{}) {
		t.Fatal("empty provider fragment should agree")
	}
	if providerFragmentAgrees(workers.ProgressFragment{
		Provider:     "codex",
		Continuation: claudeContinuation,
	}) {
		t.Fatal("provider/continuation provider mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{
		Provider:     "codex",
		Continuation: otherContinuation,
	}) {
		t.Fatal("provider/continuation identity mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{Provider: "claude", Continuation: continuation}) {
		t.Fatal("explicit provider/continuation mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{Provider: "claude", Continuation: metadataContinuation}) {
		t.Fatal("explicit provider/continuation metadata mismatch should be rejected")
	}
	if !providerFragmentAgrees(workers.ProgressFragment{Provider: "CoDeX", Continuation: metadataContinuation}) {
		t.Fatal("provider identity comparison should be case-insensitive")
	}
}

func assertCanonicalDraftIdentity(t *testing.T) {
	t.Helper()
	draft := publisherTestDraft()
	fragment := workers.ProgressFragment{DispatchID: "dispatch-1"}
	cases := []struct {
		name      string
		canonical any
		want      bool
		wantID    string
	}{
		{name: "value", canonical: draft, want: true, wantID: "dispatch-1"},
		{name: "pointer", canonical: &draft, want: true, wantID: "dispatch-1"},
		{name: "nil pointer", canonical: (*workers.Draft)(nil)},
		{name: "unsupported type", canonical: "not a draft"},
		{name: "empty kind", canonical: workers.Draft{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := canonicalDraftFromFragment(workers.ProgressFragment{
				DispatchID:     fragment.DispatchID,
				CanonicalDraft: test.canonical,
			})
			if ok != test.want {
				t.Fatalf("canonicalDraftFromFragment() ok = %v, want %v", ok, test.want)
			}
			if test.want && got.DispatchID != test.wantID {
				t.Fatalf("canonical draft DispatchID = %q, want %q", got.DispatchID, test.wantID)
			}
		})
	}
}

func assertProviderIdentityResolution(t *testing.T, reference *providers.SessionRef, metadata *providers.SessionMetadata) {
	t.Helper()
	if got := providerIdentityForFragment(workers.ProgressFragment{Provider: "claude"}, &workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}); got != "codex" {
		t.Fatalf("draft provider = %q, want codex", got)
	}
	if got := providerIdentityForFragment(workers.ProgressFragment{Provider: "claude"}, &workers.Draft{
		Provenance: workers.Provenance{Provider: "agent-run"},
	}); got != "claude" {
		t.Fatalf("synthetic draft provider fallback = %q, want claude", got)
	}
	if got := providerIdentityForFragment(workers.ProgressFragment{Continuation: testContinuationFromSessionRef(reference)}, nil); got != "codex" {
		t.Fatalf("continuation provider = %q, want codex", got)
	}
	if got := providerIdentityForFragment(workers.ProgressFragment{Continuation: testContinuationFromMetadata(metadata)}, nil); got != "codex" {
		t.Fatalf("continuation metadata provider = %q, want codex", got)
	}
	if got := providerIdentityForFragment(workers.ProgressFragment{}, nil); got != "" {
		t.Fatalf("empty provider identity = %q, want empty", got)
	}

	if !providerIdentityAgrees(workers.ProgressFragment{}, workers.Draft{}) {
		t.Fatal("draft without provider should agree")
	}
	if !providerIdentityAgrees(workers.ProgressFragment{Provider: "claude"}, workers.Draft{
		Provenance: workers.Provenance{Provider: "agent-run"},
	}) {
		t.Fatal("synthetic Worker provider should not conflict with provider output")
	}
	if providerIdentityAgrees(workers.ProgressFragment{Provider: "claude"}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("explicit provider mismatch should be rejected")
	}
	if providerIdentityAgrees(workers.ProgressFragment{Continuation: testContinuationFromSessionRef(&providers.SessionRef{Provider: providers.IDClaude})}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("continuation provider mismatch should be rejected")
	}
	if providerIdentityAgrees(workers.ProgressFragment{Continuation: testContinuationFromMetadata(&providers.SessionMetadata{Provider: "claude", ID: "session-1"})}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("continuation metadata provider mismatch should be rejected")
	}
	if !providerIdentityAgrees(workers.ProgressFragment{Provider: "CoDeX", Continuation: testContinuationFromMetadata(metadata)}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("matching provider identities should agree")
	}
}

func assertProgressProvenance(t *testing.T) {
	t.Helper()
	if got := progressDraftProvenance(workers.ProgressFragment{Type: "message.delta"}); got.Provider != "" {
		t.Fatalf("providerless provenance provider = %q, want empty", got.Provider)
	}
	if got := progressDraftProvenance(workers.ProgressFragment{
		Provider: "codex",
		Metadata: map[string]string{"native_type": "message.delta"},
	}); got.NativeEventType != "message.delta" {
		t.Fatalf("metadata native event type = %q, want message.delta", got.NativeEventType)
	}
	if got := progressDraftProvenance(workers.ProgressFragment{
		Provider: workers.RunnerIDAntigravity,
		Type:     "message.completed",
	}); got.Delivery != workers.DeliveryNativeFinal || got.Fidelity != workers.FidelityFinalOnly || got.Representation != workers.RepresentationSnapshot {
		t.Fatalf("Antigravity final message provenance = %#v, want native-final/final-only/snapshot", got)
	}
	if got := progressDraftProvenance(workers.ProgressFragment{
		Provider: workers.RunnerIDAntigravity,
		Type:     "run.completed",
	}); got.Delivery != workers.DeliverySynthesized || got.Fidelity != workers.FidelityLifecycleOnly || got.Representation != workers.RepresentationNotification {
		t.Fatalf("Antigravity lifecycle provenance = %#v, want synthesized/lifecycle-only/notification", got)
	}
}

func TestObservationClone_DetachesNestedValues(t *testing.T) {
	started := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	ended := started.Add(time.Second)
	duration := time.Second
	wantStarted, wantEnded, wantDuration := started, ended, duration
	cacheWrite, cachedInput, input, output, reasoning, total := 1, 2, 3, 4, 5, 6
	wantInput := input
	turnUsage := TurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 450}
	failure := FailureCause{Kind: FailureCauseWorkersExecutionFailure, Detail: "failed"}
	model, reasoningEffort := "gpt-5.6-luna", "high"
	original := Observation{
		WorkerSessionID:          "worker-1",
		ProviderSession:          observationTestProviderSession(),
		ProviderSessionAvailable: true,
		Model:                    &model,
		ReasoningEffort:          &reasoningEffort,
		WorkIDs:                  []string{"work-1"},
		AttemptID:                "attempt-1",
		State:                    StateFailed,
		StartedAt:                &started,
		EndedAt:                  &ended,
		Duration:                 &duration,
		DurationBasis:            DurationBasisRecordedTimestamps,
		TokenUsage:               &TokenUsage{CacheWriteTokens: &cacheWrite, CachedInputTokens: &cachedInput, InputTokens: &input, OutputTokens: &output, ReasoningOutputTokens: &reasoning, TotalTokens: &total},
		TurnUsage:                &turnUsage,
		Transcript:               TranscriptAvailabilityAvailable,
		Failure:                  &failure,
		Parse:                    ParseDiagnostics{EventCount: 2, Errors: []ParseDiagnostic{{Code: "bad", LineNumber: 3, Message: "malformed"}}},
	}
	clone := original.Clone()
	original.WorkIDs[0] = "mutated-work"
	*original.StartedAt = started.Add(time.Hour)
	*original.EndedAt = ended.Add(time.Hour)
	*original.Duration = time.Hour
	*original.TokenUsage.InputTokens = 99
	original.TurnUsage.TurnCount = 99
	*original.Model = "mutated-model"
	*original.ReasoningEffort = "mutated-effort"
	original.Failure.Detail = "mutated"
	original.Parse.Errors[0].Message = "mutated"
	if clone.WorkIDs[0] != "work-1" || !clone.StartedAt.Equal(wantStarted) || !clone.EndedAt.Equal(wantEnded) || *clone.Duration != wantDuration ||
		*clone.TokenUsage.InputTokens != wantInput || *clone.Model != "gpt-5.6-luna" || *clone.ReasoningEffort != "high" ||
		clone.TurnUsage == nil || clone.TurnUsage.TurnCount != 3 ||
		clone.Failure.Detail != "failed" || clone.Parse.Errors[0].Message != "malformed" {
		t.Fatalf("Clone() retained mutable source state: %#v", clone)
	}
}

func TestObservationSupportTypesValidate(t *testing.T) {
	for _, basis := range []DurationBasis{DurationBasisUnavailable, DurationBasisActiveClock, DurationBasisRecordedTimestamps} {
		if !basis.Valid() {
			t.Errorf("DurationBasis(%q).Valid() = false", basis)
		}
	}
	if (DurationBasis("bad")).Valid() {
		t.Fatal("unknown DurationBasis.Valid() = true")
	}
	for _, availability := range []TranscriptAvailability{TranscriptAvailabilityUnavailable, TranscriptAvailabilityAvailable} {
		if !availability.Valid() {
			t.Errorf("TranscriptAvailability(%q).Valid() = false", availability)
		}
	}
	if (TranscriptAvailability("bad")).Valid() {
		t.Fatal("unknown TranscriptAvailability.Valid() = true")
	}

	usage := TokenUsage{InputTokens: intPointer(7)}
	usageClone := usage.Clone()
	*usage.InputTokens = 8
	if *usageClone.InputTokens != 7 {
		t.Fatalf("TokenUsage.Clone() shared pointer: %#v", usageClone)
	}
	parse := ParseDiagnostics{Errors: []ParseDiagnostic{{Code: "bad"}}}
	parseClone := parse.Clone()
	parse.Errors[0].Code = "mutated"
	if parseClone.Errors[0].Code != "bad" {
		t.Fatalf("ParseDiagnostics.Clone() shared slice: %#v", parseClone)
	}
}

func TestObservationSubscriptionCloneAndClose(t *testing.T) {
	event := ObservationEvent{Payload: json.RawMessage(`{"value":"original"}`)}
	eventClone := event.Clone()
	event.Payload[0] = '{'
	if string(eventClone.Payload) != `{"value":"original"}` {
		t.Fatalf("ObservationEvent.Clone() shared payload: %s", eventClone.Payload)
	}

	closed := (ObservationSubscription{}).Next(context.Background())
	if closed.Kind != ObservationDeliveryClosed || !errors.Is(closed.Err, ErrObservationSourceClosed) {
		t.Fatalf("nil subscription Next() = %#v, want CLOSED", closed)
	}
	called := false
	subscription := ObservationSubscription{
		NextFunc: func(context.Context) ObservationDelivery {
			return ObservationDelivery{Kind: ObservationDeliveryRecord, Event: eventClone}
		},
		CloseFunc: func() { called = true },
	}
	if got := subscription.Next(context.Background()); got.Kind != ObservationDeliveryRecord || string(got.Event.Payload) != string(eventClone.Payload) {
		t.Fatalf("subscription Next() = %#v, want record", got)
	}
	subscription.Close()
	if !called {
		t.Fatal("subscription Close() did not call CloseFunc")
	}
	(ObservationSubscription{}).Close()
	if cloneBool(nil) != nil || cloneString(nil) != nil || cloneTime(nil) != nil {
		t.Fatal("nil optional clone helpers returned non-nil values")
	}
}

func TestReadTranscriptResultAndEntry_ValidateAndClone(t *testing.T) {
	text, args, line, encrypted, timestamp, turn := "hello", "{}", 4, true, time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), 1
	entry := TranscriptEntry{
		Arguments: &args, CallID: stringPointer("call-1"), Encrypted: &encrypted, EncryptedContent: stringPointer("cipher"),
		LineNumber: &line, Name: stringPointer("tool"), Order: 1, Output: stringPointer("output"), SourceType: stringPointer("provider"),
		Status: stringPointer("completed"), Summary: stringPointer("summary"), Text: &text, Timestamp: &timestamp, TurnIndex: &turn,
		Type: TranscriptToolOutput,
	}
	valid := ReadTranscriptResult{
		WorkerSessionID: "worker-1", ProviderSession: observationTestProviderSession(), WorkIDs: []string{"work-1"},
		TurnID: "turn-1", AttemptID: "attempt-1", State: StateCompleted, Entries: []TranscriptEntry{entry},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid transcript Validate() = %v", err)
	}
	clone := valid.Clone()
	valid.WorkIDs[0] = "mutated"
	*valid.Entries[0].Text = "mutated"
	if clone.WorkIDs[0] != "work-1" || *clone.Entries[0].Text != "hello" {
		t.Fatalf("transcript Clone() retained mutable source state: %#v", clone)
	}

	cases := []struct {
		name string
		make func() ReadTranscriptResult
		want error
	}{
		{"missing worker", func() ReadTranscriptResult { r := valid; r.WorkerSessionID = ""; return r }, ErrInvalidObservationIdentity},
		{"invalid provider", func() ReadTranscriptResult { r := valid; r.ProviderSession.ID = ""; return r }, ErrInvalidObservationIdentity},
		{"invalid state", func() ReadTranscriptResult { r := valid; r.State = State("bad"); return r }, ErrInvalidState},
		{"active", func() ReadTranscriptResult { r := valid; r.State = StateRunning; return r }, ErrObservationTranscriptActive},
		{"missing attempt", func() ReadTranscriptResult { r := valid; r.AttemptID = ""; return r }, ErrInvalidObservationAttempt},
		{"invalid entry", func() ReadTranscriptResult {
			r := valid
			r.Entries = []TranscriptEntry{{Order: -1, Type: TranscriptToolCall}}
			return r
		}, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := test.make().Validate()
			if test.name == "invalid entry" {
				if err == nil {
					t.Fatal("invalid entry Validate() = nil")
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want %v", err, test.want)
			}
		})
	}

	if err := (TranscriptEntry{Order: -1, Type: TranscriptToolCall}).Validate(); err == nil {
		t.Fatal("negative transcript order Validate() = nil")
	}
	if err := (TranscriptEntry{Order: 0}).Validate(); err == nil {
		t.Fatal("missing transcript type Validate() = nil")
	}
	if err := (TranscriptEntry{Order: 0, Type: TranscriptToolCall}).Validate(); err != nil {
		t.Fatalf("valid TranscriptEntry.Validate() = %v", err)
	}
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

func TestRuntimeProgressPublisher_CapturesScopeAndPreservesForwardingDecision(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		err           error
		kind          string
		wantForwarded int
	}{
		{name: "accepted", wantForwarded: 1},
		{name: "rejected", err: ErrProviderBindingAttemptMismatch},
		{name: "standalone", err: ErrRuntimeProgressUnsupervised, wantForwarded: 1},
		{name: "internal standalone", err: ErrRuntimeProgressUnsupervised, kind: workers.ProviderSessionObservedFragmentKind},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			forwarded := 0
			calls := 0
			ctx := t.Context()
			operation := RuntimeProgressPublisher(func(gotCtx context.Context, key RuntimeAttemptKey, fragment workers.ProgressFragment, next workers.ProgressPublisher) error {
				calls++
				if gotCtx != ctx {
					t.Error("publication lost its captured operation context")
				}
				if key != (RuntimeAttemptKey{RuntimeID: "owned-runtime", DispatchID: "logical-dispatch"}) {
					t.Errorf("publication key = %#v, want captured runtime and logical dispatch", key)
				}
				if fragment.Correlation.RuntimeID != "foreign-runtime" || fragment.DispatchID != "physical-attempt" {
					t.Errorf("fragment correlation was rewritten: %#v", fragment)
				}
				if test.err == nil {
					next(fragment)
				}
				return test.err
			})
			publish := operation.ForRuntime(ctx, " owned-runtime ", func(workers.ProgressFragment) { forwarded++ })
			operation = func(context.Context, RuntimeAttemptKey, workers.ProgressFragment, workers.ProgressPublisher) error {
				t.Error("replacement operation used after scope capture")
				return nil
			}
			publish(workers.ProgressFragment{
				DispatchID: "physical-attempt", Kind: test.kind,
				Correlation: workers.ExecutionCorrelation{RuntimeID: "foreign-runtime", DispatchID: " logical-dispatch "},
			})
			if calls != 1 || forwarded != test.wantForwarded {
				t.Fatalf("calls=%d forwarded=%d, want 1/%d", calls, forwarded, test.wantForwarded)
			}
		})
	}
}

func TestRuntimeProgressPublisher_RejectsProviderConflictBeforePublication(t *testing.T) {
	t.Parallel()
	operation := RuntimeProgressPublisher(func(context.Context, RuntimeAttemptKey, workers.ProgressFragment, workers.ProgressPublisher) error {
		t.Error("conflicting provider reached publication")
		return nil
	})
	operation.ForRuntime(t.Context(), "runtime", func(workers.ProgressFragment) { t.Error("conflicting provider forwarded") })(workers.ProgressFragment{
		Provider:     "claude",
		Continuation: &providers.ContinuationRef{Provider: "codex"},
	})
}

// Selected publication returns the exact effect error; the immutable owning
// runtime decides suppression and reports rejected records through its logger.
func TestPublisher_SelectedPublicationReturnsEffectErrors(t *testing.T) {
	t.Parallel()
	for _, canonical := range []bool{false, true} {
		for _, stage := range []string{"binding", "record"} {
			for _, want := range []error{ErrProviderBindingConflict, ErrProviderBindingAttemptMismatch, ErrPublicationNotOpen, ErrOutOfOrderPublication, ErrSessionNotFound, errors.New("effect failed")} {
				t.Run(fmt.Sprintf("canonical=%t/%s/%v", canonical, stage, want), func(t *testing.T) {
					t.Parallel()
					spy := &publisherServiceSpy{}
					if stage == "binding" {
						spy.ensureErr = want
					} else {
						spy.publishErr = want
					}
					fragment := workers.ProgressFragment{DispatchID: "logical", Provider: "codex", Kind: workers.ProgressFragmentKind, Type: "message.delta", Payload: "hello", Correlation: workers.ExecutionCorrelation{DispatchID: "logical", AttemptID: "physical"}}
					if canonical {
						draft := publisherTestDraft()
						draft.DispatchID = "physical"
						draft.Provenance.Provider = "codex"
						fragment = workers.CanonicalDraftFragment("logical", draft)
						fragment.Correlation = workers.ExecutionCorrelation{DispatchID: "logical", AttemptID: "physical"}
					}
					publisher := &ProviderSessionObservationPublisher{}
					if err := publisher.PublishWorkerSessionProgress(context.Background(), spy, "selected-worker", fragment); !errors.Is(err, want) {
						t.Fatalf("selected publication error = %v, want %v", err, want)
					}
				})
			}
		}
	}
}
