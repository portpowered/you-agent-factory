package workersessions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func observationTestProviderSession() providers.SessionRef {
	return providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
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
		{"list blank", (ListObservationsRequest{WorkID: "  "}).Validate(), ErrInvalidObservationWorkID},
		{"get valid", (GetObservationRequest{ProviderSession: valid}).Validate(), nil},
		{"get invalid", (GetObservationRequest{}).Validate(), ErrInvalidObservationIdentity},
		{"stream valid zero limit", (StreamObservationsRequest{ProviderSession: valid}).Validate(), nil},
		{"stream invalid identity", (StreamObservationsRequest{Limit: 1}).Validate(), ErrInvalidObservationIdentity},
		{"stream negative limit", (StreamObservationsRequest{ProviderSession: valid, Limit: -1}).Validate(), ErrInvalidObservationStreamLimit},
		{"read valid", (ReadTranscriptRequest{ProviderSession: valid}).Validate(), nil},
		{"read invalid", (ReadTranscriptRequest{}).Validate(), ErrInvalidObservationIdentity},
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

type publisherServiceSpy struct {
	Service
	ensureResult ProviderBindingResult
	ensureErr    error
	resolvedID   string
	resolveErr   error
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

func (s *publisherServiceSpy) WorkerSessionIDForDispatch(context.Context, string) (string, error) {
	return s.resolvedID, s.resolveErr
}

func (s *publisherServiceSpy) PublishRecord(context.Context, PublishRecordRequest) (PublishRecordResult, error) {
	return PublishRecordResult{}, s.publishErr
}

func TestPublisher_IdentityAndCanonicalDraftEdges(t *testing.T) {
	reference := &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}
	metadata := &workers.ProviderSessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "session-1"}

	if !providerFragmentAgrees(workers.ProgressFragment{}) {
		t.Fatal("empty provider fragment should agree")
	}
	if providerFragmentAgrees(workers.ProgressFragment{
		Provider:                 "codex",
		ProviderSessionReference: reference,
		ProviderSessionRef:       &workers.ProviderSessionMetadata{Provider: "claude", Kind: providers.SessionIDKind, ID: "session-1"},
	}) {
		t.Fatal("provider/reference metadata mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{
		Provider:                 "codex",
		ProviderSessionReference: reference,
		ProviderSessionRef:       &workers.ProviderSessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "other"},
	}) {
		t.Fatal("reference/metadata identity mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{Provider: "claude", ProviderSessionReference: reference}) {
		t.Fatal("explicit provider/reference mismatch should be rejected")
	}
	if providerFragmentAgrees(workers.ProgressFragment{Provider: "claude", ProviderSessionRef: metadata}) {
		t.Fatal("explicit provider/metadata mismatch should be rejected")
	}
	if !providerFragmentAgrees(workers.ProgressFragment{Provider: "CoDeX", ProviderSessionRef: metadata}) {
		t.Fatal("provider identity comparison should be case-insensitive")
	}

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
	if got := providerIdentityForFragment(workers.ProgressFragment{ProviderSessionReference: reference}, nil); got != "codex" {
		t.Fatalf("reference provider = %q, want codex", got)
	}
	if got := providerIdentityForFragment(workers.ProgressFragment{ProviderSessionRef: metadata}, nil); got != "codex" {
		t.Fatalf("metadata provider = %q, want codex", got)
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
	if providerIdentityAgrees(workers.ProgressFragment{ProviderSessionReference: &providers.SessionRef{Provider: providers.IDClaude}}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("reference provider mismatch should be rejected")
	}
	if providerIdentityAgrees(workers.ProgressFragment{ProviderSessionRef: &workers.ProviderSessionMetadata{Provider: "claude"}}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("metadata provider mismatch should be rejected")
	}
	if !providerIdentityAgrees(workers.ProgressFragment{Provider: "CoDeX", ProviderSessionRef: metadata}, workers.Draft{
		Provenance: workers.Provenance{Provider: "codex"},
	}) {
		t.Fatal("matching provider identities should agree")
	}

	if got := progressDraftProvenance(workers.ProgressFragment{Type: "message.delta"}); got.Provider != "" {
		t.Fatalf("providerless provenance provider = %q, want empty", got.Provider)
	}
	if got := progressDraftProvenance(workers.ProgressFragment{
		Provider: "codex",
		Metadata: map[string]string{"native_type": "message.delta"},
	}); got.NativeEventType != "message.delta" {
		t.Fatalf("metadata native event type = %q, want message.delta", got.NativeEventType)
	}
}

func TestPublisher_CanonicalPublicationErrorPolicy(t *testing.T) {
	draft := publisherTestDraft()
	draft.DispatchID = "dispatch-1"
	fragment := workers.ProgressFragment{DispatchID: "dispatch-1"}
	publisher := NewProviderSessionObservationPublisher(nil)

	if publisher.publishCanonicalWorkerRecord(nil, fragment, draft) {
		t.Fatal("nil observer should reject canonical publication")
	}
	if publisher.publishCanonicalWorkerRecord(&publisherServiceSpy{}, workers.ProgressFragment{}, draft) {
		t.Fatal("blank dispatch should reject canonical publication")
	}
	contradictory := draft
	contradictory.Provenance.Provider = "codex"
	if publisher.publishCanonicalWorkerRecord(&publisherServiceSpy{}, workers.ProgressFragment{DispatchID: "dispatch-1", Provider: "claude"}, contradictory) {
		t.Fatal("contradictory canonical provider should be rejected")
	}

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "binding ordinary error forwards", err: errors.New("binding failed"), want: true},
		{name: "binding conflict suppresses", err: ErrProviderBindingConflict, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			providerDraft := draft
			providerDraft.Provenance.Provider = "codex"
			spy := &publisherServiceSpy{ensureErr: test.err}
			if got := publisher.publishCanonicalWorkerRecord(spy, fragment, providerDraft); got != test.want {
				t.Fatalf("canonical binding result = %v, want %v", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "resolve ordinary error forwards", err: errors.New("resolve failed"), want: true},
		{name: "resolve conflict suppresses", err: ErrProviderBindingConflict, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := &publisherServiceSpy{resolveErr: test.err}
			if got := publisher.publishCanonicalWorkerRecord(spy, fragment, draft); got != test.want {
				t.Fatalf("canonical resolve result = %v, want %v", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "publish ordinary error forwards", err: errors.New("publish failed"), want: true},
		{name: "publish conflict suppresses", err: ErrProviderBindingConflict, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := &publisherServiceSpy{resolvedID: "worker-1", publishErr: test.err}
			if got := publisher.publishCanonicalWorkerRecord(spy, fragment, draft); got != test.want {
				t.Fatalf("canonical publish result = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPublisher_WorkerPublicationErrorPolicy(t *testing.T) {
	publisher := NewProviderSessionObservationPublisher(nil)
	if !publisher.publishWorkerRecord(nil, workers.ProgressFragment{DispatchID: "dispatch-1"}) {
		t.Fatal("nil observer should not fail a worker publication")
	}
	if !publisher.publishWorkerRecord(&publisherServiceSpy{}, workers.ProgressFragment{DispatchID: "dispatch-1", Type: "unknown"}) {
		t.Fatal("unrecognized worker fragment should be ignored")
	}

	base := workers.ProgressFragment{
		DispatchID: "dispatch-1",
		Kind:       workers.ProgressFragmentKind,
		Type:       "message.delta",
		Payload:    "hello",
	}
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "binding ordinary error forwards", err: errors.New("binding failed"), want: true},
		{name: "binding conflict suppresses", err: ErrProviderBindingConflict, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fragment := base
			fragment.Provider = "codex"
			spy := &publisherServiceSpy{ensureErr: test.err}
			if got := publisher.publishWorkerRecord(spy, fragment); got != test.want {
				t.Fatalf("worker binding result = %v, want %v", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "resolve ordinary error forwards", err: errors.New("resolve failed"), want: true},
		{name: "resolve conflict forwards", err: ErrProviderBindingConflict, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := &publisherServiceSpy{resolveErr: test.err}
			if got := publisher.publishWorkerRecord(spy, base); got != test.want {
				t.Fatalf("worker resolve result = %v, want %v", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "publish ordinary error forwards", err: errors.New("publish failed"), want: true},
		{name: "publish conflict suppresses", err: ErrProviderBindingConflict, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := &publisherServiceSpy{resolvedID: "worker-1", publishErr: test.err}
			if got := publisher.publishWorkerRecord(spy, base); got != test.want {
				t.Fatalf("worker publish result = %v, want %v", got, test.want)
			}
		})
	}

	if err := (ProviderBindingRequest{}).Validate(); !errors.Is(err, ErrInvalidProviderBinding) {
		t.Fatalf("empty provider binding validation = %v, want ErrInvalidProviderBinding", err)
	}
	if err := (ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "codex"}).Validate(); err != nil {
		t.Fatalf("valid provider binding validation = %v, want nil", err)
	}

	if publisher.publishWorkerDraft(nil, "worker-1", publisherTestDraft()) != nil {
		t.Fatal("nil observer should skip worker draft publication")
	}
	if publisher.publishWorkerDraft(&publisherServiceSpy{}, " ", publisherTestDraft()) != nil {
		t.Fatal("blank session should skip worker draft publication")
	}
}

func TestPublisher_DoesNotForwardInternalProviderObservation(t *testing.T) {
	forwarded := 0
	publisher := NewProviderSessionObservationPublisher(func(workers.ProgressFragment) { forwarded++ })
	publisher.Publish(workers.ProgressFragment{Kind: workers.ProviderSessionObservedFragmentKind})
	if forwarded != 0 {
		t.Fatalf("forwarded internal provider observation count = %d, want 0", forwarded)
	}
}

func TestPublisher_SuppressesConflictingCanonicalOutput(t *testing.T) {
	forwarded := 0
	publisher := NewProviderSessionObservationPublisher(func(workers.ProgressFragment) { forwarded++ })
	publisher.Bind(&publisherServiceSpy{ensureErr: ErrProviderBindingConflict})
	draft := publisherTestDraft()
	draft.DispatchID = "dispatch-1"
	draft.Provenance.Provider = "codex"
	publisher.Publish(workers.CanonicalDraftFragment("dispatch-1", draft))
	if forwarded != 0 {
		t.Fatalf("forwarded conflicting canonical output count = %d, want 0", forwarded)
	}
}

func TestObservationClone_DetachesNestedValues(t *testing.T) {
	started := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	ended := started.Add(time.Second)
	duration := time.Second
	wantStarted, wantEnded, wantDuration := started, ended, duration
	cacheWrite, cachedInput, input, output, reasoning, total := 1, 2, 3, 4, 5, 6
	wantInput := input
	failure := FailureCause{Kind: FailureCauseWorkersExecutionFailure, Detail: "failed"}
	original := Observation{
		WorkerSessionID:          "worker-1",
		ProviderSession:          observationTestProviderSession(),
		ProviderSessionAvailable: true,
		WorkIDs:                  []string{"work-1"},
		AttemptID:                "attempt-1",
		State:                    StateFailed,
		StartedAt:                &started,
		EndedAt:                  &ended,
		Duration:                 &duration,
		DurationBasis:            DurationBasisRecordedTimestamps,
		TokenUsage:               &TokenUsage{CacheWriteTokens: &cacheWrite, CachedInputTokens: &cachedInput, InputTokens: &input, OutputTokens: &output, ReasoningOutputTokens: &reasoning, TotalTokens: &total},
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
	original.Failure.Detail = "mutated"
	original.Parse.Errors[0].Message = "mutated"
	if clone.WorkIDs[0] != "work-1" || !clone.StartedAt.Equal(wantStarted) || !clone.EndedAt.Equal(wantEnded) || *clone.Duration != wantDuration ||
		*clone.TokenUsage.InputTokens != wantInput || clone.Failure.Detail != "failed" || clone.Parse.Errors[0].Message != "malformed" {
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
