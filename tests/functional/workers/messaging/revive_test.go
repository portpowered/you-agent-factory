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
	reject           bool
}

type reviveRecordingDirectory string

func (d reviveRecordingDirectory) Getwd() (string, error) { return string(d), nil }

func (g *reviveInputGate) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	return g.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (g *reviveInputGate) RecoverWorkerOwners(ctx context.Context) error {
	return g.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

func (g *reviveInputGate) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if strings.HasPrefix(key.RequestID, "continue/msg-") {
		close(g.entered)
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

func TestReviveInputRedactsExecutionTokenBeforeDurableIntent(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	raw := "private execution credential: " + lane["YOU_WORKER_SESSION_TOKEN"]
	message := f.cliSend(t, lane, "lead", "--request-id", "redacted", "--body", raw)
	if message.Body == raw || strings.Contains(message.Body, lane["YOU_WORKER_SESSION_TOKEN"]) || message.BodyRedactionCount != 1 {
		t.Fatal("execution token reached public delivery body")
	}
	assertReviveAdmission(t, f, revival, message, lead)
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
	f := startHostDelivery(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, factory, "lane")
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
