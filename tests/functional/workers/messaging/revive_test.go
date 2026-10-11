package messaging

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
