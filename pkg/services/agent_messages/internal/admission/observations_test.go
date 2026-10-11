package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

type observationAppender struct {
	requests    []events.AppendRequest
	attachments []events.AttachSourceRequest
	attachError error
	err         error
	before      func(context.Context, events.AppendRequest)
}

func (a *observationAppender) AttachSource(_ context.Context, r events.AttachSourceRequest) (events.AttachSourceResult, error) {
	a.attachments = append(a.attachments, r)
	return events.AttachSourceResult{}, a.attachError
}

func (*observationAppender) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, agentmessages.ErrStreamUnavailable
}

func (*observationAppender) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, agentmessages.ErrStreamUnavailable
}

func (a *observationAppender) Append(ctx context.Context, r events.AppendRequest) (events.AppendResult, error) {
	if a.before != nil {
		a.before(ctx, r)
	}
	a.requests = append(a.requests, r.Detached())
	return events.AppendResult{}, a.err
}

func decodedObservations(t *testing.T, a *observationAppender) []agentmessages.Observation {
	t.Helper()
	result := make([]agentmessages.Observation, 0, len(a.requests))
	for _, r := range a.requests {
		if err := r.Validate(); err != nil {
			t.Fatalf("invalid observation envelope: %v", err)
		}
		if r.SchemaID != agentmessages.ObservationSchema || r.SourceType != "agent-message" {
			t.Fatal("observation escaped its source-native messaging contract")
		}
		var observation agentmessages.Observation
		if err := json.Unmarshal(r.Payload, &observation); err != nil {
			t.Fatal(err)
		}
		if r.Topic != observation.Message.To.ObservationTopic() {
			t.Fatal("observation escaped its exact recipient stream")
		}
		if string(r.SourceID) != observation.Message.MessageID || string(r.SourceEventID) != observation.RecordID || uint64(r.SourceSequence) != observation.Sequence {
			t.Fatal("observation lost its committed identity")
		}
		result = append(result, observation)
	}
	return result
}

func TestObservationsFollowDurableSendReplyAndRead(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	a.before = func(_ context.Context, r events.AppendRequest) {
		assertObservationCommitted(t, f.ledger, r)
	}
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"first", "alias"} {
		if _, err := f.engine.Send(context.Background(), engineRequest(key, "question"), f.caller, "factory"); err != nil {
			t.Fatal(err)
		}
	}
	if len(a.requests) != 1 {
		t.Fatal("idempotent retry or alias fabricated a message observation")
	}
	assertAggregateAttachment(t, a, first)
	f.authority.sender = engineIdentity("lead")
	if _, err := f.engine.Get(context.Background(), agentmessages.GetRequest{MessageID: first.MessageID, Caller: f.caller}); err != nil {
		t.Fatal(err)
	}
	reply, err := f.engine.Send(context.Background(), agentmessages.SendRequest{RequestID: "reply", InReplyTo: first.MessageID, Body: "answer"}, f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	assertObservationKinds(t, observations, []string{store.Sent, store.Read, store.Sent, store.Replied})
	child, parent := observations[2], observations[3]
	if a.requests[0].Topic != a.requests[1].Topic || a.requests[0].Topic != a.requests[3].Topic ||
		a.requests[2].Topic == a.requests[3].Topic {
		t.Fatal("reply or parent transition was routed to the wrong recipient")
	}
	if child.RecordID != parent.RecordID || child.Sequence != parent.Sequence || child.Message.InReplyTo != first.MessageID ||
		child.Message.ThreadID != first.ThreadID || parent.Message.RepliedByMessageID != reply.MessageID {
		t.Fatal("atomic reply observations lost their common commit or relationship")
	}
}

func assertAggregateAttachment(t *testing.T, a *observationAppender, message agentmessages.Message) {
	t.Helper()
	if len(a.attachments) != 1 || a.attachments[0].Validate() != nil ||
		a.attachments[0].Source != message.To.ObservationTopic() ||
		a.attachments[0].Destination != agentmessages.ObservationStream || a.attachments[0].Mode != events.AttachModeLiveOnly {
		t.Fatal("recipient stream was not attached to authorized query aggregate")
	}
}

func assertObservationCommitted(t *testing.T, ledger *engineLedger, r events.AppendRequest) {
	t.Helper()
	var observation agentmessages.Observation
	if err := json.Unmarshal(r.Payload, &observation); err != nil {
		t.Fatal(err)
	}
	entry, exists, sequence, err := ledger.LookupMessage(observation.Message.MessageID)
	if err != nil || !exists || sequence != observation.Sequence || entry.Message.Status != observation.Message.Status {
		t.Fatal("observation published before durable transaction")
	}
}

func TestObservationsSeparateLegacyRecipientScopes(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	a.before = func(_ context.Context, r events.AppendRequest) {
		assertObservationCommitted(t, f.ledger, r)
	}
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	// Authorization and address resolution are controlled collaborators here.
	// A second exact owner has the same legacy public Worker Session ID.
	other := engineIdentity("lead")
	other.Observation.FactorySessionID = "other-factory"
	other.Chain = "other-lead-chain"
	other.Work = "other-lead-work"
	f.authority.recipients["lead"] = other
	second, err := f.engine.Send(context.Background(), engineRequest("second", "question"), f.caller, "other-factory")
	if err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	if len(observations) != 2 || first.MessageID == second.MessageID || a.requests[0].Topic == a.requests[1].Topic {
		t.Fatal("distinct legacy recipient scopes shared a record or stream")
	}
	if observations[0].Message.To.FactorySessionID != "factory" || observations[1].Message.To.FactorySessionID != "other-factory" {
		t.Fatal("publication lost the admitted exact recipient scope")
	}
}

func assertObservationKinds(t *testing.T, observations []agentmessages.Observation, kinds []string) {
	t.Helper()
	if len(observations) != len(kinds) {
		t.Fatalf("observation count = %d, want %d", len(observations), len(kinds))
	}
	for i, kind := range kinds {
		if observations[i].Kind != kind {
			t.Fatalf("observation %d kind = %q, want %q", i, observations[i].Kind, kind)
		}
	}
}

func TestObservationsBatchReadAndAdmissionExpiry(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	for _, key := range []string{"one", "two"} {
		if _, err := f.engine.Send(context.Background(), engineRequest(key, key), f.caller, "factory"); err != nil {
			t.Fatal(err)
		}
	}
	f.authority.sender = engineIdentity("lead")
	if _, err := f.engine.List(context.Background(), agentmessages.ListRequest{Caller: f.caller, ToMe: true, MarkRead: true}); err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	if len(observations) != 4 || observations[2].Kind != store.Read || observations[3].Kind != store.Read ||
		observations[2].Sequence != observations[3].Sequence || observations[2].RecordID != observations[3].RecordID {
		t.Fatal("page read did not publish one committed batch")
	}
	f.authority.sender = engineIdentity("worker")
	f.now = f.now.Add(24 * time.Hour)
	if _, err := f.engine.Send(context.Background(), engineRequest("three", "three"), f.caller, "factory"); err != nil {
		t.Fatal(err)
	}
	observations = decodedObservations(t, a)
	if len(observations) != 7 || observations[4].Kind != store.Sent || observations[5].Kind != store.Expired || observations[6].Kind != store.Expired ||
		observations[4].RecordID != observations[6].RecordID {
		t.Fatal("admission maintenance lost its durable expiry observations")
	}
}

func TestObservationsOmitDeniedAndFailedTransactions(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"denied", "owner-lost", "store"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			a := &observationAppender{}
			f.engine.events = a
			switch failure {
			case "denied":
				f.authority.denied = true
			case "owner-lost":
				f.authority.finalError = agentmessages.ErrNotPermitted
			case "store":
				f.ledger.commitError = store.ErrUnavailable
			}
			if _, err := f.engine.Send(context.Background(), engineRequest("one", "question"), f.caller, "factory"); err == nil {
				t.Fatal("failed admission succeeded")
			}
			if len(a.requests) != 0 || len(f.ledger.entries) != 0 {
				t.Fatal("failed admission published an observation or durable message")
			}
		})
	}
}

func TestObservationFailurePreservesDisconnectedDurableSuccessAndSafeTelemetry(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	var telemetry bytes.Buffer
	f.engine.logger = observationLogger{buffer: &telemetry}
	a := &observationAppender{err: errors.New("planted-secret caller-secret")}
	f.engine.events = a
	ctx, cancel := context.WithCancel(context.Background())
	f.ledger.beforeCommit = func(store.Transaction) { cancel() }
	a.before = func(ctx context.Context, _ events.AppendRequest) {
		if ctx.Err() != nil {
			t.Fatal("client disconnection suppressed a committed observation")
		}
	}
	message, err := f.engine.Send(ctx, engineRequest("one", "safe normalized body"), f.caller, "factory")
	if err != nil || len(f.ledger.entries) != 1 || len(a.requests) != 1 {
		t.Fatalf("durable success changed by stream failure: %v", err)
	}
	f.ledger.beforeCommit = nil
	retry, err := f.engine.Send(context.Background(), engineRequest("one", "safe normalized body"), f.caller, "factory")
	if err != nil || retry.MessageID != message.MessageID || len(a.requests) != 1 {
		t.Fatal("stream failure changed idempotent retry or fabricated an observation")
	}
	for _, secret := range []string{"planted-secret", f.caller.Token, "safe normalized body"} {
		if strings.Contains(telemetry.String(), secret) {
			t.Fatal("operational telemetry leaked collaborator diagnostics or content")
		}
	}
	if !strings.Contains(telemetry.String(), "Agent Message observation unavailable") || bytes.Contains(a.requests[0].Payload, []byte(f.caller.Token)) {
		t.Fatal("missing safe failure diagnostic or leaked caller credentials")
	}
}

func TestAggregateAttachmentFailurePreservesRecipientObservation(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	var telemetry bytes.Buffer
	f.engine.logger = observationLogger{buffer: &telemetry}
	a := &observationAppender{attachError: errors.New("caller-secret planted-secret")}
	f.engine.events = a
	m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
	if err != nil || len(a.requests) != 1 || len(f.ledger.entries) != 1 {
		t.Fatalf("attachment changed admission/publication: %v", err)
	}
	if a.requests[0].Topic != m.To.ObservationTopic() {
		t.Fatal("attachment failure redirected recipient observation")
	}
	if strings.Contains(telemetry.String(), "planted-secret") || strings.Contains(telemetry.String(), f.caller.Token) {
		t.Fatal("attachment diagnostics leaked")
	}
}

type observationLogger struct {
	logging.NoopLogger
	buffer *bytes.Buffer
}

func (l observationLogger) Warn(message string, fields ...any) {
	fmt.Fprintln(l.buffer, message, fields)
}
