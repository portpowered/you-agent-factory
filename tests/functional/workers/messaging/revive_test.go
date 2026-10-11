package messaging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Control only the external continuation-input sync. The real selected-profile
// store and Worker Sessions still own reservation, admission and replay.
type reviveInputGate struct {
	recordings.WorkerRecordingStore
	entered, release chan struct{}
	unblock          sync.Once
	enteredOnce      sync.Once
	reject           bool
}

type reviveRecordingDirectory string

func TestProviderConfirmationSettlesPendingRevival(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, reason string
		result       *platformprocess.CommandResult
	}{
		{"resumed progress", "", nil},
		{"missing rollout", "REVIVE_UNSUPPORTED", &platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("No rollout found for thread id controlled-thread\n")}},
		{"execution failure", "REVIVE_FAILED", &platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("controlled execution failure\n")}},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			progress := make(chan struct{})
			revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{}), progress: progress, result: cell.result}
			core, _ := observer.New(zap.InfoLevel)
			signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
			f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{ProcessLogger: zap.New(signal)}, nil, revival, "lead", "lane")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			f.endWorker(t, "lead")
			before := messageFactoryStreams(t, f, "lead", "lane")
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			frames, finished := make(chan []byte, 8), make(chan error, 1)
			go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", lane, frames, finished)
			select {
			case <-signal.ready:
			case <-ctx.Done():
				t.Fatal("follow did not register")
			}
			request := map[string]any{"requestId": "provider-confirmation", "to": map[string]string{"workerSessionId": "lead"}, "body": "normalized continuation input"}
			message := f.sendRaw(t, request, lane)
			assertPendingProviderResponse(t, message)
			assertPendingCLIParity(t, f, lane, message)
			assertProviderRetry(t, f, request, lane, message)
			assertPendingProviderFrames(t, ctx, message, frames, finished)
			var successor map[string]string
			select {
			case successor = <-revival.started:
			case <-ctx.Done():
				t.Fatal("successor did not start")
			}
			command := <-revival.requests
			if string(command.Stdin) != message.Body || !strings.Contains(strings.Join(command.Args, " "), "resume controlled-thread") {
				t.Fatal("pending revival changed recorded resume input")
			}
			if cell.reason != "" {
				close(revival.release)
			} else {
				close(progress)
			}
			final := f.awaitDeliveryOutcome(t, lane, message)
			assertProviderFinalOutcome(t, ctx, f, final, successor, frames, finished, cell.reason)
			assertProviderRetry(t, f, request, lane, final)
			assertPendingCLIParity(t, f, lane, final)
			select {
			case <-revival.requests:
				t.Fatal("final outcome retry started another continuation")
			default:
			}
			if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
				t.Fatal("provider outcome changed Factory Events")
			}
			cancel()
			select {
			case <-finished:
			case <-time.After(15 * time.Second):
				t.Fatal("provider outcome follow did not join")
			}
			if cell.reason == "" {
				close(revival.release)
			}
			f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
		})
	}
}

func assertProviderRetry(t *testing.T, f *fixture, request map[string]any, lane map[string]string, message api.AgentMessage) {
	t.Helper()
	if retry := f.sendRaw(t, request, lane); !reflect.DeepEqual(retry, message) {
		t.Fatal("retry changed durable provider outcome")
	}
}

func assertPendingCLIParity(t *testing.T, f *fixture, lane map[string]string, message api.AgentMessage) {
	t.Helper()
	var response api.AgentMessageSendResponse
	raw := f.cli(t, lane, "lead", "--request-id", "provider-confirmation", "--body", "normalized continuation input")
	if json.Unmarshal(raw, &response) != nil || !reflect.DeepEqual(response.Message, message) {
		t.Fatal("CLI exact retry differs from pending HTTP response")
	}
}

func (f *fixture) sendRaw(t *testing.T, request map[string]any, caller map[string]string) api.AgentMessage {
	t.Helper()
	status, raw := f.request(t, http.MethodPost, "/messages", request, caller)
	var response api.AgentMessageSendResponse
	if status != http.StatusAccepted || json.Unmarshal(raw, &response) != nil {
		t.Fatalf("send: %d %s", status, raw)
	}
	return response.Message
}

func assertPendingProviderResponse(t *testing.T, message api.AgentMessage) {
	t.Helper()
	if message.Status != "QUEUED" || message.Reason == nil || *message.Reason != "REVIVE_PENDING" || message.RevivedWorkerSessionId != nil || message.To.DeliveredWorkerSessionId != nil {
		t.Fatal("admission claimed provider delivery")
	}
}

func assertPendingProviderFrames(t *testing.T, ctx context.Context, message api.AgentMessage, frames <-chan []byte, finished <-chan error) {
	t.Helper()
	for _, kind := range []string{"SENT", "DELIVERY_UPDATED"} {
		var observation api.AgentMessageObservation
		if json.Unmarshal(nextFrame(t, ctx, frames, finished), &observation) != nil || string(observation.Kind) != kind || observation.Message.MessageId != message.MessageId || observation.Message.RevivedWorkerSessionId != nil {
			t.Fatal("pending follow lost ordered admission")
		}
		if kind == "DELIVERY_UPDATED" {
			assertPendingProviderResponse(t, observation.Message)
		}
	}
	select {
	case <-frames:
		t.Fatal("delivery published before provider progress")
	default:
	}
}

func assertProviderFinalOutcome(t *testing.T, ctx context.Context, f *fixture, final api.AgentMessage, successor map[string]string, frames <-chan []byte, finished <-chan error, reason string) {
	t.Helper()
	want := "DELIVERED_REVIVE"
	if reason != "" {
		want = "DELIVERY_UPDATED"
		assertRejectedProviderOutcome(t, f, final, successor, reason)
	} else if final.Status != "DELIVERED_REVIVE" || final.RevivedWorkerSessionId == nil || *final.RevivedWorkerSessionId != successor["YOU_WORKER_SESSION_ID"] || final.Reason != nil {
		t.Fatal("resumed progress lost durable successor facts")
	}
	var outcome api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &outcome) != nil || string(outcome.Kind) != want || !reflect.DeepEqual(outcome.Message, final) {
		t.Fatal("provider outcome differs between follow and public read")
	}
}

func (d reviveRecordingDirectory) Getwd() (string, error) { return string(d), nil }

func (g *reviveInputGate) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	return g.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (g *reviveInputGate) RecoverWorkerOwners(ctx context.Context) error {
	return g.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

func (g *reviveInputGate) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if strings.HasPrefix(key.RequestID, "continue/msg-") {
		g.enteredOnce.Do(func() { close(g.entered) })
		select {
		case <-g.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if g.reject {
			return "", errors.New("private-controlled-continuation-sync-failure")
		}
	}
	return g.WorkerRecordingStore.PersistWorkerControlInput(ctx, key, input)
}

func TestReviveAdmissionPublishesDurableOutcomeToReadAndFollow(t *testing.T) {
	t.Parallel()
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "rejected"}[reject], func(t *testing.T) {
			t.Parallel()
			gate := &reviveInputGate{entered: make(chan struct{}), release: make(chan struct{}), reject: reject}
			defer gate.unblock.Do(func() { close(gate.release) })
			core, _ := observer.New(zap.InfoLevel)
			signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
			revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
			dir := t.TempDir()
			f := startHostRunner(t, t.TempDir(), dir, "", edges.Edges{
				FactorySessionsWorkingDirectory: reviveRecordingDirectory(dir),
				WorkerRecordingWriter:           gate, WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { gate.WorkerRecordingStore = store },
				ProcessLogger: zap.New(signal),
			}, nil, revival, "lead", "lane")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			f.endWorker(t, "lead")
			before := messageFactoryStreams(t, f, "lead", "lane")
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			frames, finished := make(chan []byte, 4), make(chan error, 1)
			go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", lane, frames, finished)
			select {
			case <-signal.ready:
			case <-ctx.Done():
				t.Fatal("sender follow did not register")
			}
			result := make(chan api.AgentMessage, 1)
			go func() {
				result <- f.cliSend(t, lane, "lead", "--request-id", "barrier", "--body", "durable admission input")
			}()
			select {
			case <-gate.entered:
			case message := <-result:
				t.Fatalf("send finished without input sync: status=%s reason=%v", message.Status, message.Reason)
			case <-ctx.Done():
				t.Fatal("continuation did not reach input sync barrier")
			}
			assertReviveWaitsForAdmission(t, result, revival)
			gate.unblock.Do(func() { close(gate.release) })
			var message api.AgentMessage
			select {
			case message = <-result:
			case <-ctx.Done():
				t.Fatal("send did not finish after input sync")
			}
			assertReviveOutcomeObservers(t, ctx, f, lane, message, frames, finished, reject)
			if reject {
				select {
				case <-revival.requests:
					t.Fatal("rejected input started a provider")
				default:
				}
			} else {
				assertReviveAdmission(t, f, revival, message, lead)
			}
			if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
				t.Fatal("admission outcome changed canonical Factory Events")
			}
			cancel()
			select {
			case <-finished:
			case <-time.After(15 * time.Second):
				t.Fatal("admission follow did not join")
			}
		})
	}
}

func assertReviveWaitsForAdmission(t *testing.T, result <-chan api.AgentMessage, revival *workerGate) {
	t.Helper()
	select {
	case <-result:
		t.Fatal("send claimed an outcome before continuation input sync")
	default:
	}
	select {
	case <-revival.requests:
		t.Fatal("provider started before continuation input sync")
	default:
	}
}

func assertReviveOutcomeObservers(t *testing.T, ctx context.Context, f *fixture, lane map[string]string, message api.AgentMessage, frames <-chan []byte, finished <-chan error, reject bool) {
	t.Helper()
	assertReviveOutcomeFrames(t, ctx, message, frames, finished, reject)
	var read api.AgentMessageListResponse
	if json.Unmarshal(f.cli(t, lane, "list", "--thread", message.ThreadId), &read) != nil || len(read.Messages) != 1 || !reflect.DeepEqual(read.Messages[0], message) {
		t.Fatal("CLI read differs from send and HTTP follow outcome")
	}
	request := map[string]any{"requestId": "barrier", "to": map[string]string{"workerSessionId": "lead"}, "body": "durable admission input"}
	if retry := f.send(t, request, lane); !reflect.DeepEqual(retry, message) {
		t.Fatal("admission retry lost durable outcome")
	}
}

func assertReviveOutcomeFrames(t *testing.T, ctx context.Context, message api.AgentMessage, frames <-chan []byte, finished <-chan error, reject bool) {
	t.Helper()
	var sent, outcome api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &sent) != nil || sent.Kind != "SENT" || sent.Message.MessageId != message.MessageId ||
		sent.Message.Status != "QUEUED" || sent.Message.RevivedWorkerSessionId != nil || sent.Message.To.DeliveredWorkerSessionId != nil {
		t.Fatal("follow lost the queued admission or published premature delivery facts")
	}
	wantKind := "DELIVERED_REVIVE"
	if reject {
		wantKind = "DELIVERY_UPDATED"
		assertRejectedRevive(t, message)
	} else {
		assertPendingOutcomeFrame(t, ctx, frames, finished)
	}
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &outcome) != nil || string(outcome.Kind) != wantKind || !reflect.DeepEqual(outcome.Message, message) || outcome.Sequence <= sent.Sequence {
		t.Fatal("follow lost durable admission outcome")
	}
}

func assertRejectedRevive(t *testing.T, message api.AgentMessage) {
	t.Helper()
	if message.Status != "QUEUED" || message.Reason == nil || *message.Reason != "REVIVE_FAILED" || message.RevivedWorkerSessionId != nil {
		t.Fatal("rejected admission claimed success or lost typed reason")
	}
}

func TestPendingReviveAllowsInboxAndOtherAdmissions(t *testing.T) {
	t.Parallel()
	gate := &reviveInputGate{entered: make(chan struct{}), release: make(chan struct{})}
	defer gate.unblock.Do(func() { close(gate.release) })
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	dir := t.TempDir()
	f := startHostRunner(t, t.TempDir(), dir, "", edges.Edges{
		Clock: struct{ platformclock.Source }{clock}, FactorySessionsWorkingDirectory: reviveRecordingDirectory(dir),
		WorkerRecordingWriter: gate, WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { gate.WorkerRecordingStore = store },
	}, nil, revival, "lead", "lane", "other-lead", "other-lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	otherLead := f.invoke(t, "other-lead", nil)
	otherLane := f.invoke(t, "other-lane", otherLead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane", "other-lead", "other-lane")
	result := make(chan api.AgentMessage, 1)
	go func() {
		result <- f.cliSend(t, lane, "lead", "--request-id", "pending", "--body", "claimed input", "--expires", "1m")
	}()
	select {
	case <-gate.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("continuation did not reach input barrier")
	}
	// These public operations must finish while the original admission is
	// still gated. A test deadline only bounds failure; it never releases it.
	page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" || page.Messages[0].RevivedWorkerSessionId != nil {
		t.Fatal("pending intent hid the inbox or claimed delivery")
	}
	request := map[string]any{"requestId": "pending", "to": map[string]string{"workerSessionId": "lead"}, "body": "claimed input", "expiresInSeconds": 60}
	if retry := f.send(t, request, lane); !reflect.DeepEqual(retry, page.Messages[0]) {
		t.Fatal("in-flight retry changed the pending admission")
	}
	request["body"] = "conflicting input"
	assertError(t, f, http.MethodPost, "/messages", request, lane, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
	queued := f.cliSend(t, lane, "lead", "--request-id", "queued", "--body", "next head input")
	if queued.Status != "QUEUED" || queued.RevivedWorkerSessionId != nil || queued.Reason != nil {
		t.Fatal("pending chain branched")
	}
	if other := f.cliSend(t, otherLane, "other-lead", "--request-id", "unrelated", "--body", "independent inbox"); other.Status != "QUEUED" {
		t.Fatal("unrelated admission failed")
	}
	clock.SetTick(60)
	status, raw := f.request(t, http.MethodGet, "/messages/"+page.Messages[0].MessageId, nil, nil)
	var expired api.AgentMessage
	if status != http.StatusOK || json.Unmarshal(raw, &expired) != nil || expired.Status != "EXPIRED" {
		t.Fatalf("pending admission blocked expiry: %d %s", status, raw)
	}
	assertReviveWaitsForAdmission(t, result, revival)
	gate.unblock.Do(func() { close(gate.release) })
	assertExpiredReviveOutcome(t, f, lead, revival, result, before)
	close(revival.release)
	for _, id := range []string{"lane", "other-lane", "other-lead"} {
		f.endWorker(t, id)
	}
	f.server.Close(t)
}

func assertExpiredReviveOutcome(t *testing.T, f *fixture, lead map[string]string, revival *workerGate, result <-chan api.AgentMessage, before map[string][]api.FactoryEvent) {
	t.Helper()
	var message api.AgentMessage
	select {
	case message = <-result:
	case <-time.After(15 * time.Second):
		t.Fatal("pending send did not finish")
	}
	if message.Status != "EXPIRED" || message.RevivedWorkerSessionId == nil || message.Reason != nil {
		t.Fatal("admitted outcome regressed expiry or lost delivery facts")
	}
	copy := message
	copy.Status = "DELIVERED_REVIVE"
	assertReviveAdmission(t, f, revival, copy, lead)
	page := readPage(t, f, "/messages?threadId="+message.ThreadId, nil)
	if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], message) {
		t.Fatal("send outcome differs from durable read after concurrent expiry")
	}
	if after := messageFactoryStreams(t, f, "lead", "lane", "other-lead", "other-lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("pending delivery changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("pending retries or sends branched continuation")
	default:
	}
}

func TestHeadEndDeliversOrderedBatchWithoutAnotherSend(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 2), requests: make(chan platformprocess.CommandRequest, 2), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first input")
	head := assertReviveAdmission(t, f, revival, first, lead)
	queued := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second input")
	third := f.cliSend(t, lane, "lead", "--request-id", "third", "--body", "third input")
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	pending := "REVIVE_PENDING"
	queued.Reason = &pending
	batch := f.awaitDeliveryOutcome(t, lane, queued)
	batch.Body = queued.Body + "\n\n" + third.Body
	newHead := assertReviveAdmission(t, f, revival, batch, head)
	f.waitEndedWorker(t, newHead["YOU_WORKER_SESSION_ID"])
	assertContinuationLinks(t, f, head["YOU_WORKER_SESSION_ID"], newHead["YOU_WORKER_SESSION_ID"])
	page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != 3 || page.Messages[0].MessageId != first.MessageId ||
		page.Messages[1].MessageId != queued.MessageId || page.Messages[2].MessageId != third.MessageId ||
		page.Messages[1].RevivedWorkerSessionId == nil || *page.Messages[1].RevivedWorkerSessionId != newHead["YOU_WORKER_SESSION_ID"] {
		t.Fatal("ordered batch invented or lost an original message")
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("follow-on-head delivery changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("batch started another continuation")
	default:
	}
}

func TestRecipientEndDeliversOnlyUnreadReviveMessages(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{Clock: struct{ platformclock.Source }{clock}}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	read := f.cliSend(t, lane, "lead", "--request-id", "read", "--body", "already read")
	status, raw := f.request(t, http.MethodGet, "/messages/"+read.MessageId, nil, lead)
	var observed api.AgentMessage
	if status != http.StatusOK || json.Unmarshal(raw, &observed) != nil || observed.Status != "READ" {
		t.Fatal("recipient did not mark the first input read")
	}
	replied := f.cliSend(t, lane, "lead", "--request-id", "replied", "--body", "already answered", "--reply-hold")
	f.cliSend(t, lead, "reply", replied.MessageId, "--request-id", "answer", "--body", "held answer")
	expired := f.cliSend(t, lane, "lead", "--request-id", "expired", "--body", "expired input", "--expires", "1m")
	held := f.cliSend(t, lane, "lead", "--request-id", "held", "--body", "held input", "--if-ended", "hold")
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first eligible")
	second := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second eligible")
	clock.SetTick(60)
	page := readPage(t, f, "/messages?threadId="+expired.ThreadId, lane)
	if len(page.Messages) != 1 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("expiry did not exclude the old input")
	}
	before := messageFactoryStreams(t, f, "lead", "lane")
	f.endWorker(t, "lead")
	pending := "REVIVE_PENDING"
	first.Reason = &pending
	final := f.awaitDeliveryOutcome(t, lane, first)
	batch := final
	batch.Body = first.Body + "\n\n" + second.Body
	head := assertReviveAdmission(t, f, revival, batch, lead)
	page = readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	assertEndDeliverySelection(t, page, head["YOU_WORKER_SESSION_ID"], read, replied, expired, held, first, second)
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("recipient-end delivery changed Factory Events")
	}
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	if replay := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second eligible"); replay.RevivedWorkerSessionId == nil || *replay.RevivedWorkerSessionId != head["YOU_WORKER_SESSION_ID"] {
		t.Fatal("exact retry lost the shared successor")
	}
	select {
	case <-revival.requests:
		t.Fatal("excluded messages or exact retry started another successor")
	default:
	}
}

func assertEndDeliverySelection(t *testing.T, page api.AgentMessageListResponse, head string, messages ...api.AgentMessage) {
	t.Helper()
	statuses := []string{"READ", "REPLIED", "EXPIRED", "QUEUED", "DELIVERED_REVIVE", "DELIVERED_REVIVE"}
	if len(page.Messages) != len(messages) {
		t.Fatal("end delivery invented or lost a message")
	}
	for i, message := range page.Messages {
		if message.MessageId != messages[i].MessageId || string(message.Status) != statuses[i] {
			t.Fatalf("end delivery changed selection/order at %d: %+v", i, message)
		}
		if i < 4 && message.RevivedWorkerSessionId != nil || i >= 4 && (message.RevivedWorkerSessionId == nil || *message.RevivedWorkerSessionId != head) {
			t.Fatal("end delivery attached incorrect successor facts")
		}
	}
}

func assertContinuationLinks(t *testing.T, f *fixture, source, successor string) {
	t.Helper()
	for _, id := range []string{source, successor} {
		status, raw := f.request(t, http.MethodGet, "/worker-sessions/"+id, nil, nil)
		var observation api.WorkerSessionObservation
		if status != http.StatusOK || json.Unmarshal(raw, &observation) != nil {
			t.Fatal("missing continuation observation")
		}
		if id == source && (observation.SuccessorWorkerSessionId == nil || *observation.SuccessorWorkerSessionId != successor) {
			t.Fatal("later send branched from stale source")
		}
		if id == successor && (observation.PredecessorWorkerSessionId == nil || *observation.PredecessorWorkerSessionId != source) {
			t.Fatal("batch did not use newest ended head")
		}
	}
}

func TestReviveAliasSurvivesReconstructionWithFreshCaller(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, home, dir, "", edges.Edges{}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	message := f.cliSend(t, lane, "lead", "--request-id", "original", "--body", "persist normalized input")
	head := assertReviveAdmission(t, f, revival, message, lead)
	alias := f.cliSend(t, lane, "lead", "--request-id", "alias", "--body", "persist normalized input")
	if !reflect.DeepEqual(alias, message) {
		t.Fatal("body alias changed admitted delivery")
	}
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	f.endWorker(t, "lane")
	f.server.Close(t)
	second := startHost(t, home, dir, "", "lane-next")
	// Only the existing public Worker Sessions continuation contract restores
	// this caller. Historical execution tokens must remain unusable.
	status, raw := second.request(t, http.MethodPost, "/worker-sessions/lane/continue", map[string]any{
		"resolveHead": true, "requestId": "sender-next-visit", "successorWorkerSessionId": "lane-next", "followUpInput": "authorized later visit",
	}, nil)
	if status != http.StatusAccepted {
		t.Fatalf("sender continuation after restart: %d %s", status, raw)
	}
	var caller map[string]string
	select {
	case caller = <-second.runner.workers["lane-next"].started:
	case <-time.After(15 * time.Second):
		t.Fatal("new sender did not start")
	}
	if caller["YOU_WORKER_SESSION_TOKEN"] == "" || caller["YOU_WORKER_SESSION_TOKEN"] == lane["YOU_WORKER_SESSION_TOKEN"] {
		t.Fatal("reconstruction reused expired execution credentials")
	}
	request := map[string]any{"requestId": "alias", "to": map[string]string{"workerSessionId": "lead"}, "body": "persist normalized input"}
	// The archived caller retains its original owner correlation; rebuilding
	// the host does not reopen that Factory Session. Observe this host's live
	// canonical stream rather than claiming the archived session is running.
	before := second.server.GetFactoryEvents(t)
	if retry := second.send(t, request, caller); !reflect.DeepEqual(retry, message) {
		t.Fatal("reconstructed alias changed original delivery")
	}
	assertError(t, second, http.MethodPost, "/messages", request, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	request["body"] = "changed alias input"
	assertError(t, second, http.MethodPost, "/messages", request, caller, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
	page := readPage(t, second, "/messages", nil)
	if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], message) {
		t.Fatal("reconstructed alias created a new public message")
	}
	if after := second.server.GetFactoryEvents(t); !reflect.DeepEqual(before, after) {
		t.Fatal("alias retry changed canonical Factory Events")
	}
	second.endWorker(t, "lane-next")
	second.server.Close(t)
}

func TestConcurrentMessagesUseOneSuccessorAndStableRequest(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 2), requests: make(chan platformprocess.CommandRequest, 2), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane-a", "lane-b")
	lead := f.invoke(t, "lead", nil)
	a, b := f.invoke(t, "lane-a", lead), f.invoke(t, "lane-b", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane-a", "lane-b")
	start := make(chan struct{})
	results := make(chan api.AgentMessage, 2)
	var group sync.WaitGroup
	for _, caller := range []map[string]string{a, b} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- f.send(t, map[string]any{"requestId": "ask", "to": map[string]string{"workerSessionId": "lead"}, "body": caller["YOU_WORKER_SESSION_ID"]}, caller)
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var delivered, queued api.AgentMessage
	for message := range results {
		switch message.Status {
		case "DELIVERED_REVIVE":
			if delivered.MessageId != "" {
				t.Fatal("concurrent sends branched")
			}
			delivered = message
		case "QUEUED":
			if queued.MessageId != "" {
				t.Fatal("neither send revived the ended recipient")
			}
			queued = message
		default:
			t.Fatal("unexpected delivery status", message.Status)
		}
	}
	if delivered.MessageId == "" || queued.MessageId == "" || queued.RevivedWorkerSessionId != nil {
		t.Fatal("missing admitted delivery or queued second message")
	}
	successor := assertReviveAdmission(t, f, revival, delivered, lead)
	caller := a
	if delivered.From.WorkerSessionId == b["YOU_WORKER_SESSION_ID"] {
		caller = b
	}
	assertConcurrentReviveRetries(t, f, caller, delivered)
	page := readPage(t, f, "/messages?toMe=true", successor)
	if len(page.Messages) != 2 {
		t.Fatal("successor lost queued chain inbox")
	}
	if after := messageFactoryStreams(t, f, "lead", "lane-a", "lane-b"); !reflect.DeepEqual(before, after) {
		t.Fatal("concurrent messaging changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("concurrent sends or retries started another successor")
	default:
	}
}

func assertConcurrentReviveRetries(t *testing.T, f *fixture, caller map[string]string, original api.AgentMessage) {
	t.Helper()
	request := map[string]any{"requestId": "ask", "to": map[string]string{"workerSessionId": "lead"}, "body": original.Body}
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			if got := f.send(t, request, caller); !reflect.DeepEqual(got, original) {
				t.Error("concurrent retry changed original admission")
			}
		}()
	}
	group.Wait()
	request["body"] = "changed normalized input"
	assertError(t, f, http.MethodPost, "/messages", request, caller, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
}

func TestMissingProviderReferenceKeepsReviveReadable(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane")
	f.runner.workers["lead"].result = &platformprocess.CommandResult{Stdout: []byte("{\"type\":\"turn.completed\"}\n")}
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	message := f.cliSend(t, lane, "lead", "--request-id", "unsupported", "--body", "retain this ask")
	if message.Status != "QUEUED" || message.Reason == nil || *message.Reason != "REVIVE_UNSUPPORTED" || message.RevivedWorkerSessionId != nil {
		t.Fatal("missing provider reference claimed delivery")
	}
	request := map[string]any{"requestId": "unsupported", "to": map[string]string{"workerSessionId": "lead"}, "body": "retain this ask"}
	assertProviderRetry(t, f, request, lane, message)
	if retry := f.cliSend(t, lane, "lead", "--request-id", "unsupported", "--body", "retain this ask"); !reflect.DeepEqual(retry, message) {
		t.Fatal("CLI retry changed unsupported outcome")
	}
	var got api.AgentMessageListResponse
	if err := json.Unmarshal(f.cli(t, lane, "list", "--thread", message.ThreadId), &got); err != nil || len(got.Messages) != 1 || !reflect.DeepEqual(got.Messages[0], message) {
		t.Fatal("unsupported revival lost readable message", err)
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("unsupported revival changed Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("missing reference reached provider")
	default:
	}
}

func TestDeniedEndedRecipientSendHasNoDeliveryEffect(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane", "sibling", "root")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	sibling := f.invoke(t, "sibling", nil)
	f.invoke(t, "root", nil)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	request := map[string]any{"requestId": "denied", "to": map[string]string{"workerSessionId": "lead"}, "body": "never persist or resume"}
	for _, caller := range []map[string]string{
		nil,
		lead, // Ended execution credentials do not authenticate a new send.
		sibling,
		{"YOU_WORKER_SESSION_ID": "lane", "YOU_WORKER_SESSION_TOKEN": strings.Repeat("A", 43)},
		{"YOU_WORKER_SESSION_ID": "root", "YOU_WORKER_SESSION_TOKEN": lane["YOU_WORKER_SESSION_TOKEN"]},
	} {
		assertError(t, f, http.MethodPost, "/messages", request, caller, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	}
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("denied caller persisted an ended-recipient ask")
	}
	select {
	case <-revival.requests:
		t.Fatal("denied caller started a provider continuation")
	default:
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("denied send changed Factory Events")
	}
	// Denials consume neither this request key nor the authorized sender's quota.
	message := f.cliSend(t, lane, "lead", "--request-id", "denied", "--body", "never persist or resume")
	successor := assertReviveAdmission(t, f, revival, message, lead)
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
}

func TestReviveInputRedactsExecutionTokenBeforeDurableIntent(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	core, logs := observer.New(zap.InfoLevel)
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{ProcessLogger: zap.New(core)}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	raw := "private execution credential: " + lane["YOU_WORKER_SESSION_TOKEN"]
	message := f.cliSend(t, lane, "lead", "--request-id", "redacted", "--body", raw)
	if message.Body == raw || strings.Contains(message.Body, lane["YOU_WORKER_SESSION_TOKEN"]) || message.BodyRedactionCount != 1 {
		t.Fatal("execution token reached public delivery body")
	}
	successor := assertReviveAdmission(t, f, revival, message, lead)
	journal, err := os.ReadFile(filepath.Join(f.home, ".you-agent-factory", "messages", "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(journal), lane["YOU_WORKER_SESSION_TOKEN"]) || strings.Contains(string(journal), raw) {
		t.Fatal("execution token reached durable message or intent")
	}
	alias := f.cliSend(t, lane, "lead", "--request-id", "redacted-alias", "--body", raw)
	if alias.MessageId != message.MessageId {
		t.Fatal("normalized private body lost dedup")
	}
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	f.server.Close(t)
	private := []string{raw, lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"], successor["YOU_WORKER_SESSION_TOKEN"]}
	assertPrivateArtifacts(t, f.home, private)
	assertPrivateArtifacts(t, f.dir, private)
	assertPrivateStreamDiagnostics(t, logs, private...)
}

// Separate selected profiles are necessary here: identical Worker Session IDs
// on two active hosts must never share execution authority or delivery state.
func TestRevivalIsolatedAcrossHostsWithIdenticalWorkerIDs(t *testing.T) {
	t.Parallel()
	var hosts []*fixture
	var callers, leads []map[string]string
	for range 2 {
		revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
		f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane")
		lead := f.invoke(t, "lead", nil)
		callers = append(callers, f.invoke(t, "lane", lead))
		leads, hosts = append(leads, lead), append(hosts, f)
		f.endWorker(t, "lead")
	}
	request := map[string]any{"requestId": "same-host-key", "to": map[string]string{"workerSessionId": "lead"}, "body": "host-owned ask"}
	var messages []api.AgentMessage
	for i, f := range hosts {
		assertError(t, f, http.MethodPost, "/messages", request, callers[1-i], http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
		if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
			t.Fatal("foreign-host credential persisted a message")
		}
		select {
		case <-f.runner.revival.requests:
			t.Fatal("foreign-host credential started revival")
		default:
		}
		message := f.cliSend(t, callers[i], "lead", "--request-id", "same-host-key", "--body", "host-owned ask")
		successor := assertReviveAdmission(t, f, f.runner.revival, message, leads[i])
		assertProviderRetry(t, f, request, callers[i], message)
		page := readPage(t, f, "/messages?toMe=true", successor)
		if len(page.Messages) != 1 || page.Messages[0].MessageId != message.MessageId {
			t.Fatal("host successor lost its own inbox")
		}
		messages = append(messages, message)
		close(f.runner.revival.release)
		f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	}
	if messages[0].MessageId == messages[1].MessageId || *messages[0].RevivedWorkerSessionId == *messages[1].RevivedWorkerSessionId {
		t.Fatal("independent hosts shared message or successor identity")
	}
	for i, f := range hosts {
		assertError(t, f, http.MethodGet, "/messages/"+messages[1-i].MessageId, nil, callers[i], http.StatusNotFound, "MESSAGE_NOT_FOUND")
		if page := readPage(t, f, "/messages", nil); len(page.Messages) != 1 || page.Messages[0].MessageId != messages[i].MessageId {
			t.Fatal("host inbox contains foreign delivery state")
		}
	}
}

func TestEndedRecipientRevivesOnceThroughCLIAndHTTP(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 2), requests: make(chan platformprocess.CommandRequest, 2), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	hold := f.cliSend(t, lane, "lead", "--request-id", "hold", "--body", "retained for pull", "--if-ended", "hold")
	if hold.Status != "QUEUED" || hold.Reason != nil {
		t.Fatal("HOLD caused a delivery effect")
	}
	message := f.cliSend(t, lane, "lead", "--request-id", "ask", "--body", "review sanitized input")
	successor := assertReviveAdmission(t, f, revival, message, lead)
	assertReviveRetriesAndInbox(t, f, message, lane, successor)
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("Messaging changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("retry or second send started another continuation")
	default:
	}
}

func TestFactoryOriginRecipientRevivesWithoutAnotherWorkResult(t *testing.T) {
	t.Parallel()
	factory := &workerGate{started: make(chan map[string]string, 1), release: make(chan struct{})}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	core, logs := observer.New(zap.InfoLevel)
	f := startHostDelivery(t, t.TempDir(), t.TempDir(), "", edges.Edges{ProcessLogger: zap.New(core)}, nil, revival, factory, "lane")
	session := support.OpenFactorySessionAt(t, f.server.URL(), f.dir).Session.Id
	support.SubmitSessionWorkAt(t, f.server.URL(), session, api.SubmitWorkRequest{WorkTypeName: "task", Payload: "Factory-origin lead"})
	var lead map[string]string
	select {
	case lead = <-factory.started:
	case <-time.After(15 * time.Second):
		t.Fatal("Factory worker did not start")
	}
	id := lead["YOU_WORKER_SESSION_ID"]
	lane := f.invoke(t, "lane", lead)
	close(factory.release)
	f.waitEndedWorker(t, id)
	before := messageFactoryStreams(t, f, id, "lane")
	message := f.cliSend(t, lane, id, "--request-id", "factory-ask", "--body", "declared private delivery", "--body-secret")
	if message.Body == "declared private delivery" || message.BodyRedactionCount != 1 {
		t.Fatal("declared secret entered delivery input")
	}
	successor := assertReviveAdmission(t, f, revival, message, lead)
	status, raw := f.request(t, http.MethodGet, "/worker-sessions/"+successor["YOU_WORKER_SESSION_ID"], nil, nil)
	var observation api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(raw, &observation) != nil || !observation.Direct ||
		observation.Correlation == nil || observation.Correlation.WorkId == nil || *observation.Correlation.WorkId != lead["YOU_WORK_ID"] {
		t.Fatal("Factory successor lost direct execution or original Work correlation")
	}
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	if after := messageFactoryStreams(t, f, id, "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("Factory-origin revival changed canonical Factory Events")
	}
	f.server.Close(t)
	private := []string{"declared private delivery", lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"], successor["YOU_WORKER_SESSION_TOKEN"]}
	assertPrivateArtifacts(t, f.home, private)
	assertPrivateArtifacts(t, f.dir, private)
	assertPrivateStreamDiagnostics(t, logs, private...)
}

func assertReviveAdmission(t *testing.T, f *fixture, revival *workerGate, message api.AgentMessage, lead map[string]string) map[string]string {
	t.Helper()
	if message.Status != "DELIVERED_REVIVE" || message.RevivedWorkerSessionId == nil || message.To.DeliveredWorkerSessionId == nil ||
		*message.RevivedWorkerSessionId != *message.To.DeliveredWorkerSessionId || message.Reason != nil {
		t.Fatalf("missing durable delivery: %+v", message)
	}
	var successor map[string]string
	select {
	case successor = <-revival.started:
	case <-time.After(15 * time.Second):
		t.Fatal("successor did not reach command edge")
	}
	request := <-revival.requests
	if request.Command != "codex" || !strings.Contains(strings.Join(request.Args, " "), "resume controlled-thread") ||
		string(request.Stdin) != message.Body || request.WorkDir != f.dir {
		t.Fatalf("resume contract differs: argv=%q stdin=%q dir=%q", request.Args, request.Stdin, request.WorkDir)
	}
	if successor["YOU_WORKER_SESSION_ID"] != *message.RevivedWorkerSessionId || successor["YOU_WORKER_SESSION_TOKEN"] == "" ||
		successor["YOU_WORKER_SESSION_TOKEN"] == lead["YOU_WORKER_SESSION_TOKEN"] {
		t.Fatal("successor reused old execution authority")
	}
	return successor
}

func assertReviveRetriesAndInbox(t *testing.T, f *fixture, message api.AgentMessage, lane, successor map[string]string) {
	t.Helper()
	retry := f.send(t, map[string]any{"requestId": "ask", "to": map[string]string{"workerSessionId": "lead"}, "body": "review sanitized input"}, lane)
	if !reflect.DeepEqual(retry, message) {
		t.Fatal("HTTP retry changed CLI delivery facts")
	}
	duplicate := f.cliSend(t, lane, "lead", "--request-id", "alias", "--body", "review sanitized input")
	if duplicate.MessageId != message.MessageId {
		t.Fatal("normalized body dedup created a new message")
	}
	queued := f.send(t, map[string]any{"requestId": "second", "to": map[string]string{"workerSessionId": "lead"}, "body": "queued against active head"}, lane)
	if queued.Status != "QUEUED" || queued.RevivedWorkerSessionId != nil || queued.Reason != nil {
		t.Fatal("second send branched the continuation chain")
	}
	page := readPage(t, f, "/messages?toMe=true&status=DELIVERED_REVIVE", successor)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != message.MessageId {
		t.Fatal("successor cannot read its chain inbox")
	}
}

// The public Worker stream closes after its terminal frame. Retained replay
// plus live delivery makes completion deterministic without timing polls.
func (f *fixture) endWorker(t *testing.T, id string) {
	t.Helper()
	close(f.runner.workers[id].release)
	f.waitEndedWorker(t, id)
}

func (f *fixture) waitEndedWorker(t *testing.T, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.server.URL()+"/worker-sessions/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("Worker stream unavailable", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event.SchemaId == "workers.draft.v1" && frame.Event.Payload["kind"] == "SESSION" &&
			(frame.Event.Payload["phase"] == "COMPLETED" || frame.Event.Payload["phase"] == "FAILED") {
			return
		}
	}
	// Factory-origin streams may close with the Factory-owned terminal record
	// rather than a direct Worker lifecycle draft. Confirm the public state once
	// at that EOF; an early disconnect cannot be mistaken for completion.
	if scanner.Err() == nil && f.workerEnded(t, id) {
		return
	}
	t.Fatal("missing terminal Worker observation", scanner.Err())
}

func (f *fixture) workerEnded(t *testing.T, id string) bool {
	t.Helper()
	status, raw := f.request(t, http.MethodGet, "/worker-sessions/"+id, nil, nil)
	var observation api.WorkerSessionObservation
	return status == http.StatusOK && json.Unmarshal(raw, &observation) == nil &&
		(observation.State == "COMPLETED" || observation.State == "FAILED")
}

func assertPendingOutcomeFrame(t *testing.T, ctx context.Context, frames <-chan []byte, finished <-chan error) {
	t.Helper()
	var pending api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &pending) != nil || pending.Kind != "DELIVERY_UPDATED" {
		t.Fatal("follow lost pending admission")
	}
	assertPendingProviderResponse(t, pending.Message)
}

func assertRejectedProviderOutcome(t *testing.T, f *fixture, final api.AgentMessage, successor map[string]string, reason string) {
	t.Helper()
	if final.Status != "QUEUED" || final.Reason == nil || *final.Reason != reason || final.RevivedWorkerSessionId != nil || final.To.DeliveredWorkerSessionId != nil {
		t.Fatal("provider rejection claimed delivery or lost readable typed reason")
	}
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	status, raw := f.request(t, http.MethodGet, "/worker-sessions/"+successor["YOU_WORKER_SESSION_ID"], nil, nil)
	var observation api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(raw, &observation) != nil || observation.State != "FAILED" {
		t.Fatal("rejected provider lost failed successor")
	}
	if observation.Failure == nil || observation.Failure.Kind == "" || observation.Failure.Detail == "" {
		t.Fatal("rejected successor lost its public failure cause")
	}
}
