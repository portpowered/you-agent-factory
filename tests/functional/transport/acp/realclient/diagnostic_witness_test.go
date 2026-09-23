package realclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	acpWitnessPrefix          = "ACP_FACTORY_EVENT_WITNESS "
	acpWitnessSchemaVersion   = "factory-reliability.acp-event-witness.v1"
	acpWitnessMaxEvents       = 64
	acpWitnessMaxBytes        = 32 * 1024
	acpWitnessReadLimit       = 64
	acpWitnessCursorReadLimit = 4096
	acpWitnessStallAfter      = 30 * time.Second
	acpWitnessReadTimeout     = 2 * time.Second
)

type canonicalEventReader interface {
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

type acpWitnessInvocation struct {
	Test             string  `json:"test"`
	RequestID        uint64  `json:"requestId"`
	ACPSessionID     string  `json:"acpSessionId"`
	FactorySessionID *string `json:"factorySessionId"`
	DispatchID       *string `json:"dispatchId"`
}

type acpWitnessEvent struct {
	ID              string `json:"id"`
	Sequence        int64  `json:"sequence"`
	SessionSequence *int   `json:"sessionSequence,omitempty"`
	Type            string `json:"type"`
	SessionID       string `json:"sessionId"`
	DispatchID      string `json:"dispatchId,omitempty"`
}

type acpWitnessCaptureError struct {
	Class  string `json:"class"`
	Detail string `json:"detail"`
}

type acpFactoryEventWitness struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Status        string                  `json:"status"`
	Invocation    acpWitnessInvocation    `json:"invocation"`
	Events        []acpWitnessEvent       `json:"events"`
	OmittedCount  int                     `json:"omittedCount"`
	CaptureError  *acpWitnessCaptureError `json:"captureError"`
}

type canonicalEventWindow struct {
	events       []recordings.CanonicalEvent
	omittedCount int
	lastCursor   *recordings.CanonicalEventCursor
	errorClass   string
}

type canonicalEventSourceContext struct {
	DispatchID      *string `json:"dispatchId"`
	SessionID       *string `json:"sessionId"`
	SessionSequence *int    `json:"sessionSequence"`
}

func (fixture *reusableACPFixture) startPromptWitness(
	t *testing.T,
	connection *reusableACPConnection,
	session reusableACPSession,
) (uint64, *acpWitnessEmitter) {
	t.Helper()
	requestID := connection.nextID + 1
	invocation := acpWitnessInvocation{
		Test:         "TestReusableACPServerTurnsThroughOneProcess",
		RequestID:    requestID,
		ACPSessionID: session.id,
	}
	if session.factorySessionID != "" {
		factorySessionID := session.factorySessionID
		invocation.FactorySessionID = &factorySessionID
	}
	after := fixture.eventCursorCopy(session.factorySessionID)
	excludedSessionIDs := fixture.seenFactorySessionIDsCopy()
	return requestID, newACPWitnessEmitter(t, func() string {
		return captureACPEventWitness(context.Background(), fixture.recordings, invocation, after, excludedSessionIDs...)
	})
}

func (fixture *reusableACPFixture) eventCursorCopy(factorySessionID string) *recordings.CanonicalEventCursor {
	if factorySessionID == "" || fixture.eventCursors[factorySessionID] == nil {
		return nil
	}
	cursor := *fixture.eventCursors[factorySessionID]
	return &cursor
}

func (fixture *reusableACPFixture) seenFactorySessionIDsCopy() []string {
	ids := make([]string, 0, len(fixture.seenFactorySessionIDs))
	for id := range fixture.seenFactorySessionIDs {
		ids = append(ids, id)
	}
	return ids
}

func (fixture *reusableACPFixture) advanceCanonicalCursor(factorySessionID string) string {
	if fixture.recordings == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), acpWitnessReadTimeout)
	defer cancel()
	scope := recordings.CanonicalEventScope{}
	after := (*recordings.CanonicalEventCursor)(nil)
	if factorySessionID != "" {
		scope.FactorySessionID = factorySessionID
		after = fixture.eventCursorCopy(factorySessionID)
	}
	window := readCanonicalEventWindow(
		ctx,
		fixture.recordings,
		after,
		scope,
		acpWitnessCursorReadLimit,
	)
	if window.errorClass != "" || window.omittedCount != 0 || len(window.events) == 0 {
		if factorySessionID != "" {
			return factorySessionID
		}
		return ""
	}
	if factorySessionID != "" {
		fixture.eventCursors[factorySessionID] = window.lastCursor
		fixture.seenFactorySessionIDs[factorySessionID] = struct{}{}
		return factorySessionID
	}
	sessionIDs := canonicalFactorySessionIDs(window.events, fixture.seenFactorySessionIDsCopy()...)
	if len(sessionIDs) != 1 {
		return ""
	}
	factorySessionID = sessionIDs[0]
	for _, event := range window.events {
		if event.Scope.FactorySessionID == factorySessionID {
			cursor := event.Cursor
			fixture.eventCursors[factorySessionID] = &cursor
		}
	}
	if fixture.eventCursors[factorySessionID] == nil {
		return ""
	}
	fixture.seenFactorySessionIDs[factorySessionID] = struct{}{}
	return factorySessionID
}

type acpWitnessEmitter struct {
	t       *testing.T
	capture func() string
	once    sync.Once
	stopCh  chan struct{}
	done    chan struct{}
	line    string
}

func newACPWitnessEmitter(t *testing.T, capture func() string) *acpWitnessEmitter {
	emitter := &acpWitnessEmitter{
		t:       t,
		capture: capture,
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}
	go func() {
		defer close(emitter.done)
		timer := time.NewTimer(acpWitnessStallAfter)
		defer timer.Stop()
		select {
		case <-emitter.stopCh:
			return
		case <-timer.C:
			emitter.emit()
		}
	}()
	return emitter
}

func (emitter *acpWitnessEmitter) emit() string {
	emitter.once.Do(func() {
		emitter.line = emitter.capture()
		emitter.t.Log(emitter.line)
	})
	return emitter.line
}

func (emitter *acpWitnessEmitter) stop() {
	close(emitter.stopCh)
	<-emitter.done
}

func captureACPEventWitness(
	ctx context.Context,
	reader canonicalEventReader,
	invocation acpWitnessInvocation,
	after *recordings.CanonicalEventCursor,
	excludedFactorySessionIDs ...string,
) string {
	readContext, cancel := context.WithTimeout(ctx, acpWitnessReadTimeout)
	defer cancel()
	invocation = safeACPWitnessInvocation(invocation)
	if reader == nil {
		return unavailableACPWitness(invocation, "canonical_reader_unavailable")
	}

	factorySessionID, errorClass := resolveACPWitnessFactorySession(
		readContext, reader, invocation, after, excludedFactorySessionIDs,
	)
	if errorClass != "" {
		return unavailableACPWitness(invocation, errorClass)
	}
	window := readCanonicalEventWindow(
		readContext,
		reader,
		after,
		recordings.CanonicalEventScope{FactorySessionID: factorySessionID},
		acpWitnessMaxEvents,
	)
	if window.errorClass != "" {
		return unavailableACPWitness(invocation, window.errorClass)
	}
	if len(window.events) == 0 {
		return unavailableACPWitness(invocation, "no_canonical_events")
	}
	return capturedACPWitness(invocation, factorySessionID, window)
}

func resolveACPWitnessFactorySession(
	ctx context.Context,
	reader canonicalEventReader,
	invocation acpWitnessInvocation,
	after *recordings.CanonicalEventCursor,
	excludedFactorySessionIDs []string,
) (string, string) {
	factorySessionID := dereferenceString(invocation.FactorySessionID)
	if factorySessionID == "" {
		mapping := readCanonicalEventWindow(ctx, reader, after, recordings.CanonicalEventScope{}, acpWitnessReadLimit)
		if mapping.errorClass != "" {
			return "", mapping.errorClass
		}
		if mapping.omittedCount > 0 {
			return "", "mapping_window_truncated"
		}
		sessionIDs := canonicalFactorySessionIDs(mapping.events, excludedFactorySessionIDs...)
		switch len(sessionIDs) {
		case 0:
			return "", "no_factory_session_mapping"
		case 1:
			factorySessionID = sessionIDs[0]
		default:
			return "", "multiple_factory_sessions"
		}
	}
	if !safeACPWitnessIdentifier(factorySessionID, 256) {
		return "", "invalid_factory_session_identity"
	}
	return factorySessionID, ""
}

func capturedACPWitness(
	invocation acpWitnessInvocation,
	factorySessionID string,
	window canonicalEventWindow,
) string {
	invocation.FactorySessionID = stringPointer(factorySessionID)
	witness := acpFactoryEventWitness{
		SchemaVersion: acpWitnessSchemaVersion,
		Status:        "captured",
		Invocation:    invocation,
		Events:        make([]acpWitnessEvent, 0, min(len(window.events), acpWitnessMaxEvents)),
		OmittedCount:  window.omittedCount,
	}
	for index, event := range window.events {
		projected, ok := projectACPWitnessEvent(event, factorySessionID)
		if !ok {
			return unavailableACPWitness(invocation, "invalid_canonical_event")
		}
		witness.Events = append(witness.Events, projected)
		previousDispatchID := witness.Invocation.DispatchID
		if projected.DispatchID != "" {
			witness.Invocation.DispatchID = stringPointer(projected.DispatchID)
		}
		_, marshalOK := marshalBoundedACPWitness(witness)
		if !marshalOK {
			witness.Events = witness.Events[:len(witness.Events)-1]
			witness.Invocation.DispatchID = previousDispatchID
			witness.OmittedCount += len(window.events) - index
			break
		}
		if len(witness.Events) == acpWitnessMaxEvents && index+1 < len(window.events) {
			witness.OmittedCount += len(window.events) - index - 1
			break
		}
	}
	line, ok := marshalBoundedACPWitness(witness)
	if !ok || len(witness.Events) == 0 {
		return unavailableACPWitness(invocation, "witness_output_too_large")
	}
	return line
}

func readCanonicalEventWindow(
	ctx context.Context,
	reader canonicalEventReader,
	after *recordings.CanonicalEventCursor,
	scope recordings.CanonicalEventScope,
	limit int,
) canonicalEventWindow {
	window := canonicalEventWindow{events: []recordings.CanonicalEvent{}}
	subscriptionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	subscribed, err := reader.SubscribeFrom(subscriptionContext, recordings.SubscribeRequest{Cursor: after, Scope: scope})
	if err != nil {
		window.errorClass = canonicalReadErrorClass(ctx, err)
		return window
	}
	if subscribed.RetainedEventCount < 0 || limit < 1 ||
		(subscribed.RetainedEventCount > 0 && subscribed.Subscription == nil) {
		window.errorClass = "canonical_read_unavailable"
		return window
	}
	readCount := min(subscribed.RetainedEventCount, limit)
	window.omittedCount = subscribed.RetainedEventCount - readCount
	for index := 0; index < readCount; index++ {
		outcome := subscribed.Subscription.Next(subscriptionContext)
		switch outcome.Kind {
		case recordings.SubscriptionEvent:
			event := outcome.Event
			window.events = append(window.events, event)
			cursor := event.Cursor
			window.lastCursor = &cursor
		case recordings.SubscriptionGap:
			window.events = nil
			window.errorClass = "canonical_read_gap"
			return window
		case recordings.SubscriptionClosed:
			window.events = nil
			window.errorClass = canonicalReadErrorClass(ctx, errors.New("stream closed"))
			return window
		default:
			window.events = nil
			window.errorClass = "canonical_read_unavailable"
			return window
		}
	}
	return window
}

func canonicalReadErrorClass(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "canonical_read_timeout"
	}
	if errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		return "canonical_read_cursor_unavailable"
	}
	if errors.Is(err, recordings.ErrReconnectCursorExpired) {
		return "canonical_read_cursor_expired"
	}
	return "canonical_read_failed"
}

func canonicalFactorySessionIDs(events []recordings.CanonicalEvent, excludedFactorySessionIDs ...string) []string {
	seen := make(map[string]struct{}, len(events))
	for _, id := range excludedFactorySessionIDs {
		seen[id] = struct{}{}
	}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		id := event.Scope.FactorySessionID
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func projectACPWitnessEvent(event recordings.CanonicalEvent, factorySessionID string) (acpWitnessEvent, bool) {
	if event.Scope.FactorySessionID != factorySessionID ||
		!safeACPWitnessIdentifier(string(event.ID), 256) ||
		!safeACPWitnessIdentifier(string(event.Kind), 80) || event.Sequence < 0 {
		return acpWitnessEvent{}, false
	}
	var sourceContext canonicalEventSourceContext
	if event.SourceContext != "" {
		if err := json.Unmarshal([]byte(event.SourceContext), &sourceContext); err != nil {
			return acpWitnessEvent{}, false
		}
	}
	if sourceContext.SessionID != nil && *sourceContext.SessionID != factorySessionID {
		return acpWitnessEvent{}, false
	}
	if sourceContext.DispatchID != nil && !safeACPWitnessIdentifier(*sourceContext.DispatchID, 256) {
		return acpWitnessEvent{}, false
	}
	if sourceContext.SessionSequence != nil && *sourceContext.SessionSequence < 0 {
		return acpWitnessEvent{}, false
	}
	return acpWitnessEvent{
		ID:              string(event.ID),
		Sequence:        int64(event.Sequence),
		SessionSequence: sourceContext.SessionSequence,
		Type:            string(event.Kind),
		SessionID:       factorySessionID,
		DispatchID:      dereferenceString(sourceContext.DispatchID),
	}, true
}

func safeACPWitnessIdentifier(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("-_.:/", char) {
			continue
		}
		return false
	}
	return true
}

func marshalBoundedACPWitness(witness acpFactoryEventWitness) (string, bool) {
	encoded, err := json.Marshal(witness)
	if err != nil {
		return "", false
	}
	line := acpWitnessPrefix + string(encoded)
	return line, len(line)+1 <= acpWitnessMaxBytes
}

func unavailableACPWitness(invocation acpWitnessInvocation, class string) string {
	invocation = safeACPWitnessInvocation(invocation)
	invocation.FactorySessionID = nil
	invocation.DispatchID = nil
	witness := acpFactoryEventWitness{
		SchemaVersion: acpWitnessSchemaVersion,
		Status:        "unavailable",
		Invocation:    invocation,
		Events:        []acpWitnessEvent{},
		CaptureError:  &acpWitnessCaptureError{Class: class, Detail: acpWitnessErrorDetail(class)},
	}
	line, ok := marshalBoundedACPWitness(witness)
	if ok {
		return line
	}
	return acpWitnessPrefix + `{"schemaVersion":"factory-reliability.acp-event-witness.v1","status":"unavailable","invocation":{"test":"TestReusableACPServerTurnsThroughOneProcess","requestId":0,"acpSessionId":"redacted","factorySessionId":null,"dispatchId":null},"events":[],"omittedCount":0,"captureError":{"class":"witness_output_too_large","detail":"canonical event witness exceeded the output bound"}}`
}

func safeACPWitnessInvocation(invocation acpWitnessInvocation) acpWitnessInvocation {
	if !safeACPWitnessIdentifier(invocation.Test, 128) {
		invocation.Test = "TestReusableACPServerTurnsThroughOneProcess"
	}
	if !safeACPWitnessIdentifier(invocation.ACPSessionID, 256) {
		invocation.ACPSessionID = "redacted"
	}
	if invocation.FactorySessionID != nil && !safeACPWitnessIdentifier(*invocation.FactorySessionID, 256) {
		invocation.FactorySessionID = nil
	}
	if invocation.DispatchID != nil && !safeACPWitnessIdentifier(*invocation.DispatchID, 256) {
		invocation.DispatchID = nil
	}
	return invocation
}

func acpWitnessErrorDetail(class string) string {
	switch class {
	case "canonical_reader_unavailable":
		return "canonical Factory Event reader unavailable"
	case "canonical_read_timeout":
		return "canonical Factory Event read exceeded its bounded deadline"
	case "canonical_read_gap":
		return "canonical Factory Event read reported a retention gap"
	case "canonical_read_unavailable":
		return "canonical Factory Event read returned no finite stream"
	case "canonical_read_cursor_unavailable":
		return "canonical Factory Event cursor belongs to another stream generation"
	case "canonical_read_cursor_expired":
		return "canonical Factory Event cursor is no longer retained"
	case "mapping_window_truncated":
		return "session mapping exceeded the bounded correlation window"
	case "multiple_factory_sessions":
		return "invocation window contained multiple Factory Sessions"
	case "no_factory_session_mapping":
		return "no canonical Factory Session observed before deadline"
	case "invalid_factory_session_identity":
		return "canonical Factory Session identity was not safe to emit"
	case "no_canonical_events":
		return "no canonical Factory Events observed before deadline"
	case "invalid_canonical_event":
		return "canonical Factory Event fields failed safe validation"
	case "witness_output_too_large":
		return "canonical event witness exceeded the output bound"
	default:
		return "canonical Factory Event read failed"
	}
}

func assertCapturedFactoryWitness(t *testing.T, line string) {
	t.Helper()
	if !strings.HasPrefix(line, acpWitnessPrefix) {
		t.Fatalf("ACP witness line prefix = %q, want %q", line, acpWitnessPrefix)
	}
	var witness acpFactoryEventWitness
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, acpWitnessPrefix)), &witness); err != nil {
		t.Fatalf("decode ACP witness: %v", err)
	}
	if witness.SchemaVersion != acpWitnessSchemaVersion || witness.Status != "captured" ||
		witness.CaptureError != nil || len(witness.Events) == 0 ||
		witness.Invocation.FactorySessionID == nil || *witness.Invocation.FactorySessionID == "" ||
		witness.Invocation.DispatchID == nil || *witness.Invocation.DispatchID == "" {
		t.Fatalf("ACP failure-time witness = %+v, want captured canonical dispatch correlation", witness)
	}
	for index, event := range witness.Events {
		if event.ID == "" || event.Sequence < 0 || event.SessionID != *witness.Invocation.FactorySessionID {
			t.Fatalf("ACP witness event %d = %+v, want exact ID, sequence and Factory Session correlation", index, event)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type acpWitnessReaderFake struct {
	events []recordings.CanonicalEvent
	err    error
	gap    bool
	closed bool
	wait   bool
}

func (fake acpWitnessReaderFake) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if fake.err != nil {
		return recordings.SubscribeResult{}, fake.err
	}
	events := make([]recordings.CanonicalEvent, 0, len(fake.events))
	for _, event := range fake.events {
		if request.Scope.FactorySessionID != "" && event.Scope.FactorySessionID != request.Scope.FactorySessionID {
			continue
		}
		if request.Cursor != nil && event.Sequence <= request.Cursor.Sequence {
			continue
		}
		events = append(events, event)
	}
	index := 0
	return recordings.SubscribeResult{
		RetainedEventCount: len(events),
		Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
			if fake.gap {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionGap}
			}
			if fake.wait {
				<-ctx.Done()
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			if fake.closed || index >= len(events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			outcome := recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: events[index]}
			index++
			return outcome
		},
	}, nil
}

func witnessTestEvent(id, sessionID, dispatchID string, sequence int64, sessionSequence int) recordings.CanonicalEvent {
	sourceContext, _ := json.Marshal(canonicalEventSourceContext{
		DispatchID:      stringPointer(dispatchID),
		SessionID:       stringPointer(sessionID),
		SessionSequence: &sessionSequence,
	})
	return recordings.CanonicalEvent{
		ID:            recordings.CanonicalEventID(id),
		Sequence:      recordings.CanonicalEventSequence(sequence),
		Scope:         recordings.CanonicalEventScope{FactorySessionID: sessionID},
		Cursor:        recordings.CanonicalEventCursor{StreamGenerationID: "test-generation", Sequence: recordings.CanonicalEventSequence(sequence)},
		Kind:          recordings.CanonicalEventKind("DISPATCH_REQUEST"),
		Payload:       `{"prompt":"private-payload"}`,
		SourceContext: string(sourceContext),
	}
}

func TestACPEventWitnessCapturesCanonicalFieldsWithoutPayload(t *testing.T) {
	t.Parallel()
	event := witnessTestEvent("factory-event/dispatch-request/dispatch-3/1", "factory-session-17", "dispatch-3", 42, 17)
	event.SourceContext = strings.TrimSuffix(event.SourceContext, "}") + `,"private":"private-context"}`
	line := captureACPEventWitness(context.Background(), acpWitnessReaderFake{events: []recordings.CanonicalEvent{event}}, acpWitnessInvocation{
		Test: "TestReusableACPServerTurnsThroughOneProcess", RequestID: 4, ACPSessionID: "acp-session-42",
	}, nil)
	assertCapturedFactoryWitness(t, line)
	if strings.Contains(line, "private-payload") || strings.Contains(line, "private-context") {
		t.Fatalf("ACP witness leaked event payload/context: %s", line)
	}
	var witness acpFactoryEventWitness
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, acpWitnessPrefix)), &witness); err != nil {
		t.Fatalf("decode witness: %v", err)
	}
	if got := witness.Events[0]; got.ID != string(event.ID) || got.Sequence != int64(event.Sequence) ||
		got.SessionSequence == nil || *got.SessionSequence != 17 || got.Type != "DISPATCH_REQUEST" ||
		got.SessionID != "factory-session-17" || got.DispatchID != "dispatch-3" {
		t.Fatalf("witness event = %+v, want copied canonical identities", got)
	}
}

func TestACPEventWitnessBoundsEventsAndBytes(t *testing.T) {
	t.Parallel()
	events := make([]recordings.CanonicalEvent, 80)
	for index := range events {
		events[index] = witnessTestEvent(
			fmt.Sprintf("factory-event/dispatch-request/dispatch-%d/%d", index, index),
			"factory-session-17", fmt.Sprintf("dispatch-%d", index), int64(index), index,
		)
	}
	line := captureACPEventWitness(context.Background(), acpWitnessReaderFake{events: events}, acpWitnessInvocation{
		Test: "TestReusableACPServerTurnsThroughOneProcess", RequestID: 4, ACPSessionID: "acp-session-42",
		FactorySessionID: stringPointer("factory-session-17"),
	}, nil)
	var witness acpFactoryEventWitness
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, acpWitnessPrefix)), &witness); err != nil {
		t.Fatalf("decode bounded witness: %v", err)
	}
	if len(witness.Events) != acpWitnessMaxEvents || witness.OmittedCount != 16 ||
		len(line)+1 > acpWitnessMaxBytes {
		t.Fatalf("bounded witness events=%d omitted=%d bytes=%d", len(witness.Events), witness.OmittedCount, len(line)+1)
	}
}

func TestACPEventWitnessReportsSafeUnavailableBoundaries(t *testing.T) {
	t.Parallel()
	secret := "private-read-error-token"
	expiredContext, cancelExpiredContext := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpiredContext()
	cases := []struct {
		name      string
		ctx       context.Context
		reader    acpWitnessReaderFake
		class     string
		factoryID *string
	}{
		{name: "empty history", reader: acpWitnessReaderFake{}, class: "no_factory_session_mapping"},
		{name: "read failure", reader: acpWitnessReaderFake{err: errors.New(secret)}, class: "canonical_read_failed"},
		{name: "multiple sessions", reader: acpWitnessReaderFake{events: []recordings.CanonicalEvent{
			witnessTestEvent("event/session-one/1", "session-one", "dispatch-one", 1, 1),
			witnessTestEvent("event/session-two/2", "session-two", "dispatch-two", 2, 1),
		}}, class: "multiple_factory_sessions"},
		{name: "retention gap", reader: acpWitnessReaderFake{events: []recordings.CanonicalEvent{
			witnessTestEvent("event/session-one/1", "session-one", "dispatch-one", 1, 1),
		}, gap: true}, class: "canonical_read_gap"},
		{name: "closed stream", reader: acpWitnessReaderFake{events: []recordings.CanonicalEvent{
			witnessTestEvent("event/session-one/1", "session-one", "dispatch-one", 1, 1),
		}, closed: true}, class: "canonical_read_failed", factoryID: stringPointer("session-one")},
		{name: "read deadline", reader: acpWitnessReaderFake{events: []recordings.CanonicalEvent{
			witnessTestEvent("event/session-one/1", "session-one", "dispatch-one", 1, 1),
		}, wait: true}, class: "canonical_read_timeout", factoryID: stringPointer("session-one"), ctx: expiredContext},
		{name: "known session without events", reader: acpWitnessReaderFake{}, class: "no_canonical_events", factoryID: stringPointer("factory-session-17")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ctx := testCase.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			line := captureACPEventWitness(ctx, testCase.reader, acpWitnessInvocation{
				Test: "TestReusableACPServerTurnsThroughOneProcess", RequestID: 4, ACPSessionID: "acp-session-42",
				FactorySessionID: testCase.factoryID,
			}, nil)
			var witness acpFactoryEventWitness
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, acpWitnessPrefix)), &witness); err != nil {
				t.Fatalf("decode unavailable witness: %v", err)
			}
			if witness.Status != "unavailable" || witness.CaptureError == nil || witness.CaptureError.Class != testCase.class ||
				len(witness.Events) != 0 || strings.Contains(line, secret) {
				t.Fatalf("unavailable witness = %+v; line=%s", witness, line)
			}
		})
	}
}

func TestACPEventWitnessRejectsUnsafeCanonicalIdentifiers(t *testing.T) {
	t.Parallel()
	if safeACPWitnessIdentifier("dispatch-3", 256) != true ||
		safeACPWitnessIdentifier("dispatch-3\nprivate-token", 256) ||
		safeACPWitnessIdentifier(strings.Repeat("x", 257), 256) {
		t.Fatal("witness identifier validation did not enforce character and byte bounds")
	}
	line := captureACPEventWitness(context.Background(), nil, acpWitnessInvocation{
		Test: "private\ntest", RequestID: 4, ACPSessionID: "private-token\nvalue",
	}, nil)
	if strings.Contains(line, "private") || !strings.Contains(line, `"acpSessionId":"redacted"`) {
		t.Fatalf("unsafe invocation identity was not redacted: %s", line)
	}
}
