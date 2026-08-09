package workersessions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
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
