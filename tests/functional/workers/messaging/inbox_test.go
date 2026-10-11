package messaging

import (
	"encoding/json"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAuthorizedInboxReplyAndReconstruction(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	f := startHost(t, home, dir, "", "lead", "lane", "sibling")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	sibling := f.invoke(t, "sibling", lead)
	eventsBefore := messageFactoryStreams(t, f, "lead", "lane", "sibling")
	request := map[string]any{"requestId": "lane-send", "to": map[string]string{"workerSessionId": "lead"}, "body": "coordinate inbox"}
	message := f.cliSend(t, lane, "lead", "--request-id", "lane-send", "--body", "coordinate inbox")
	assertQueuedReplay(t, f, message, request, lane)
	assertDeniedReadersAndSenders(t, f, message.MessageId, lane, sibling)
	before := assertPullAndReply(t, f, message, lead)
	assertMessagePrivacy(t, f, message, lead, lane)
	if eventsAfter := messageFactoryStreams(t, f, "lead", "lane", "sibling"); !reflect.DeepEqual(eventsBefore, eventsAfter) {
		t.Fatal("send/replay/denial/read/reply changed canonical Factory Events")
	}
	f.server.Close(t)
	second := startHost(t, home, dir, "")
	status, body := second.request(t, http.MethodGet, "/messages/"+message.MessageId, nil, nil)
	if status != http.StatusOK || string(body) != before {
		t.Fatalf("reconstruction changed durable reply: %d %s", status, body)
	}
	assertError(t, second, http.MethodPost, "/messages", request, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	functionalevidence.Covers(t, "cli/you.message", "cli/you.message.list", "cli/you.message.reply", "rest/sendAgentMessage", "rest/listAgentMessages", "rest/getAgentMessage")
}

func messageFactoryStreams(t *testing.T, f *fixture, workers ...string) map[string][]api.FactoryEvent {
	t.Helper()
	result := map[string][]api.FactoryEvent{"~default": support.GetFactoryEventsAt(t, f.baseURL())}
	for _, worker := range workers {
		status, body := f.request(t, http.MethodGet, "/worker-sessions/"+worker, nil, nil)
		var observation api.WorkerSessionObservation
		if status != http.StatusOK || json.Unmarshal(body, &observation) != nil || observation.FactorySessionId == nil {
			t.Fatal("missing canonical Factory Session for running worker")
		}
		session := *observation.FactorySessionId
		result[session] = support.GetFactoryEventsForSessionAt(t, f.baseURL(), session)
	}
	return result
}

func assertQueuedReplay(t *testing.T, f *fixture, message api.AgentMessage, request map[string]any, lane map[string]string) {
	t.Helper()
	if message.Status != "QUEUED" || message.From.WorkerSessionId != "lane" || message.To.WorkerSessionId != "lead" || message.ThreadId != message.MessageId {
		t.Fatalf("incorrect queued identity: %+v", message)
	}
	if retry := f.send(t, request, lane); !reflect.DeepEqual(message, retry) {
		t.Fatal("request replay changed persisted message")
	}
	request["requestId"] = "body-duplicate"
	if retry := f.send(t, request, lane); retry.MessageId != message.MessageId {
		t.Fatal("body dedup created another message")
	}
	request["body"] = "changed"
	assertError(t, f, http.MethodPost, "/messages", request, lane, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
	request["body"] = "coordinate inbox"
}

func assertDeniedReadersAndSenders(t *testing.T, f *fixture, id string, lane, sibling map[string]string) {
	t.Helper()
	for _, cell := range []struct {
		name   string
		caller map[string]string
		target string
	}{
		{"operator write", nil, "lead"}, {"sibling write", lane, "sibling"}, {"unrelated read", sibling, ""},
	} {
		t.Run(cell.name, func(t *testing.T) {
			method, path, body := http.MethodPost, "/messages", any(map[string]any{"requestId": cell.name, "to": map[string]string{"workerSessionId": cell.target}, "body": "denied"})
			if cell.target == "" {
				method, path, body = http.MethodGet, "/messages/"+id, nil
			}
			assertError(t, f, method, path, body, cell.caller, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
		})
	}
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "unknown", "to": map[string]string{"workerSessionId": "unknown"}, "body": "unknown"}, lane, http.StatusNotFound, "MESSAGE_RECIPIENT_NOT_FOUND")
}

func assertPullAndReply(t *testing.T, f *fixture, message api.AgentMessage, lead map[string]string) string {
	t.Helper()
	output := f.cli(t, lead, "list", "--to-me", "--mark-read")
	var inbox api.AgentMessageListResponse
	if err := json.Unmarshal(output, &inbox); err != nil {
		t.Fatalf("CLI page: %v", err)
	}
	if len(inbox.Messages) != 1 || inbox.Messages[0].Status != "READ" || inbox.Messages[0].MessageId != message.MessageId {
		t.Fatalf("recipient pull: %+v", inbox)
	}
	reply := f.cliSend(t, lead, "reply", message.MessageId, "--request-id", "lead-reply", "--body", "acknowledged")
	if reply.To.WorkerSessionId != "lane" || reply.ThreadId != message.ThreadId || reply.InReplyTo == nil || *reply.InReplyTo != message.MessageId {
		t.Fatalf("reply relationship: %+v", reply)
	}
	assertReplyReplayAndConflict(t, f, message.MessageId, reply, lead)
	return assertRepliedParent(t, f, message.MessageId, reply.MessageId)
}

func assertReplyReplayAndConflict(t *testing.T, f *fixture, parentID string, reply api.AgentMessage, lead map[string]string) {
	t.Helper()
	request := map[string]any{"requestId": "lead-reply", "inReplyTo": parentID, "body": "acknowledged"}
	if replay := f.send(t, request, lead); !reflect.DeepEqual(replay, reply) {
		t.Fatal("reply retry changed the committed message")
	}
	request["body"] = "changed acknowledgement"
	assertError(t, f, http.MethodPost, "/messages", request, lead, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
	status, body := f.request(t, http.MethodGet, "/messages?threadId="+reply.ThreadId, nil, nil)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || len(page.Messages) != 2 {
		t.Fatal("reply retry or conflict created an additional thread message")
	}
}

func assertRepliedParent(t *testing.T, f *fixture, id, replyID string) string {
	t.Helper()
	status, body := f.request(t, http.MethodGet, "/messages/"+id, nil, nil)
	var original api.AgentMessage
	if status != http.StatusOK || json.Unmarshal(body, &original) != nil || original.Status != "REPLIED" || original.RepliedByMessageId == nil || *original.RepliedByMessageId != replyID {
		t.Fatalf("atomic reply: %d %s", status, body)
	}
	return string(body)
}

func assertMessagePrivacy(t *testing.T, f *fixture, message api.AgentMessage, lead, lane map[string]string) {
	t.Helper()
	for _, event := range f.server.GetFactoryEvents(t) {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{message.MessageId, message.ThreadId, message.Body, lead["YOU_WORKER_SESSION_TOKEN"], lane["YOU_WORKER_SESSION_TOKEN"]} {
			if strings.Contains(string(encoded), value) {
				t.Fatal("message or credential leaked to canonical Factory Events")
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(f.home, ".you-agent-factory", "messages", "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), lane["YOU_WORKER_SESSION_TOKEN"]) || strings.Contains(string(data), lead["YOU_WORKER_SESSION_TOKEN"]) {
		t.Fatal("credential persisted")
	}
}

func assertError(t *testing.T, f *fixture, method, path string, request any, caller map[string]string, expected int, code string) {
	t.Helper()
	status, data := f.request(t, method, path, request, caller)
	var result api.ErrorResponse
	if status != expected || json.Unmarshal(data, &result) != nil || string(result.Code) != code {
		t.Fatalf("typed refusal: %d %s", status, data)
	}
}

func TestEndedSenderRepliesPreserveInheritedDeliveryPolicy(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{true, false} {
		name := "default revive"
		if hold {
			name = "held until authorized continuation"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
			f := startHostRunner(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, "lead", "lane", "lane-next", "sibling")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			sibling := f.invoke(t, "sibling", lead)
			args := []string{"lead", "--request-id", "ask", "--body", "please answer"}
			if hold {
				args = append(args, "--reply-hold")
			}
			ask := f.cliSend(t, lane, args...)
			f.endWorker(t, "lane")
			before := messageFactoryStreams(t, f, "lead", "lane")
			reply := f.cliSend(t, lead, "reply", ask.MessageId, "--request-id", "answer", "--body", "original requester answer")
			if reply.ThreadId != ask.ThreadId || reply.InReplyTo == nil || *reply.InReplyTo != ask.MessageId || reply.To.WorkerSessionId != "lane" {
				t.Fatal("ended sender reply lost requester or thread")
			}
			if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
				t.Fatal("reply changed Factory Events")
			}
			assertError(t, f, http.MethodGet, "/messages/"+reply.MessageId, nil, sibling, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
			assertError(t, f, http.MethodGet, "/messages/"+reply.MessageId, nil, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
			if hold {
				assertHeldReplyContinuation(t, f, revival, reply)
			} else {
				if reply.IfEnded != "REVIVE" {
					t.Fatal("default reply did not inherit REVIVE")
				}
				head := assertReviveAdmission(t, f, revival, reply, lane)
				page := readPage(t, f, "/messages?toMe=true", head)
				if len(page.Messages) != 1 || page.Messages[0].MessageId != reply.MessageId {
					t.Fatal("revived requester lost reply inbox")
				}
				close(revival.release)
				f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
			}
		})
	}
}

func assertHeldReplyContinuation(t *testing.T, f *fixture, revival *workerGate, reply api.AgentMessage) {
	t.Helper()
	if reply.IfEnded != "HOLD" || reply.Status != "QUEUED" || reply.RevivedWorkerSessionId != nil || reply.Reason != nil {
		t.Fatal("held reply revived an ended asker")
	}
	select {
	case <-revival.requests:
		t.Fatal("HOLD started a provider")
	default:
	}
	next := f.continueCaller(t, "lane", "lane-next")
	var page api.AgentMessageListResponse
	if json.Unmarshal(f.cli(t, next, "list", "--to-me", "--mark-read"), &page) != nil || len(page.Messages) != 1 || page.Messages[0].MessageId != reply.MessageId || page.Messages[0].Status != "READ" {
		t.Fatal("fresh authorized requester continuation lost held reply")
	}
	f.endWorker(t, "lane-next")
	select {
	case <-revival.requests:
		t.Fatal("read held reply triggered revival")
	default:
	}
}

func TestHeldReplyIsVisibleToSameWorkRedispatch(t *testing.T) {
	t.Parallel()
	factory := &workerGate{started: make(chan map[string]string, 2), release: make(chan struct{})}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostDelivery(t, t.TempDir(), t.TempDir(), "", edges.Edges{}, nil, revival, factory, "lead", "sibling")
	t.Cleanup(func() { close(factory.release) })
	writeHeldReplyRedispatchFactory(t, f.dir)
	session := support.OpenFactorySessionAt(t, f.server.URL(), f.dir).Session.Id
	support.SubmitSessionWorkAt(t, f.server.URL(), session, api.SubmitWorkRequest{WorkTypeName: "seed", Payload: "produce an asker"})
	producer := awaitHeldReplyDispatch(t, revival)
	<-revival.requests
	close(revival.release)
	f.waitEndedWorker(t, producer["YOU_WORKER_SESSION_ID"])
	first := awaitHeldReplyDispatch(t, factory)
	lead := f.continueCaller(t, producer["YOU_WORKER_SESSION_ID"], "lead")
	if lead["YOU_WORK_ID"] == first["YOU_WORK_ID"] {
		t.Fatal("requester and asker must own distinct Work")
	}
	sibling := f.invoke(t, "sibling", nil)
	ask := f.cliSend(t, first, "lead", "--request-id", "held-ask", "--body", "answer on my next Work visit", "--reply-hold")
	select {
	case factory.release <- struct{}{}:
	case <-time.After(15 * time.Second):
		t.Fatal("first Work visit did not reach completion gate")
	}
	f.waitEndedWorker(t, first["YOU_WORKER_SESSION_ID"])
	next := awaitHeldReplyDispatch(t, factory)
	assertSameWorkFreshDispatch(t, f, first, next)
	before := messageFactoryStreams(t, f, first["YOU_WORKER_SESSION_ID"], next["YOU_WORKER_SESSION_ID"], "lead")
	reply := f.cliSend(t, lead, "reply", ask.MessageId, "--request-id", "held-answer", "--body", "answer for the original Work")
	assertHeldReplyRedispatchInbox(t, f, first, next, sibling, ask, reply)
	if after := messageFactoryStreams(t, f, first["YOU_WORKER_SESSION_ID"], next["YOU_WORKER_SESSION_ID"], "lead"); !reflect.DeepEqual(before, after) {
		t.Fatal("held reply and same-Work inbox reads changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("held reply started a direct continuation")
	default:
	}
	select {
	case factory.release <- struct{}{}:
	case <-time.After(15 * time.Second):
		t.Fatal("second Work visit did not reach completion gate")
	}
	f.waitEndedWorker(t, next["YOU_WORKER_SESSION_ID"])
}

func writeHeldReplyRedispatchFactory(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	workType := document["workTypes"].([]any)[0].(map[string]any)
	workType["states"] = append(workType["states"].([]any), map[string]any{"name": "revisit", "type": "NORMAL"})
	first := document["workstations"].([]any)[0].(map[string]any)
	first["outputs"] = []any{map[string]any{"workType": "task", "state": "revisit"}}
	document["workstations"] = append(document["workstations"].([]any), map[string]any{
		"name": "revisit", "worker": "worker", "inputs": []any{map[string]any{"workType": "task", "state": "revisit"}},
		"outputs":   []any{map[string]any{"workType": "task", "state": "done"}},
		"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
	})
	document["workTypes"] = append(document["workTypes"].([]any), map[string]any{"name": "seed", "states": []any{
		map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "failed", "type": "FAILED"},
	}})
	document["workstations"] = append(document["workstations"].([]any), map[string]any{
		"name": "produce", "worker": "worker", "inputs": []any{map[string]any{"workType": "seed", "state": "init"}},
		"workPropagation": map[string]any{"mode": "PRESERVE_INPUT"},
		"outputs":         []any{map[string]any{"workType": "task", "state": "init"}},
		"onFailure":       []any{map[string]any{"workType": "seed", "state": "failed"}},
	})
	raw, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	support.WriteWorkstationConfig(t, dir, "revisit", "---\ntype: MODEL_WORKSTATION\n---\nFactory-origin lead revisits the same Work.\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nFactory-origin lead first asker visit.\n")
	support.WriteWorkstationConfig(t, dir, "produce", "---\ntype: MODEL_WORKSTATION\n---\nProduce the asker Work.\n")
}

func awaitHeldReplyDispatch(t *testing.T, gate *workerGate) map[string]string {
	t.Helper()
	select {
	case caller := <-gate.started:
		return caller
	case <-time.After(15 * time.Second):
		t.Fatal("Factory Work visit did not start")
	}
	return nil
}

func assertSameWorkFreshDispatch(t *testing.T, f *fixture, first, next map[string]string) {
	t.Helper()
	if first["YOU_WORK_ID"] == "" || next["YOU_WORK_ID"] != first["YOU_WORK_ID"] ||
		next["YOU_FACTORY_SESSION_ID"] != first["YOU_FACTORY_SESSION_ID"] ||
		next["YOU_WORKER_SESSION_ID"] == first["YOU_WORKER_SESSION_ID"] ||
		next["YOU_WORKER_SESSION_TOKEN"] == "" || next["YOU_WORKER_SESSION_TOKEN"] == first["YOU_WORKER_SESSION_TOKEN"] {
		t.Fatal("redispatch did not retain exact Work and issue fresh execution authority")
	}
	var observations []api.WorkerSessionObservation
	for _, caller := range []map[string]string{first, next} {
		observations = append(observations, sameWorkFactoryObservation(t, f, caller, first["YOU_WORK_ID"]))
	}
	if observations[0].State != "COMPLETED" || observations[1].State != "RUNNING" ||
		observations[0].AttemptId == observations[1].AttemptId ||
		!reflect.DeepEqual(observations[0].Requester, observations[1].Requester) {
		t.Fatal("same-Work redispatch lost terminal/running attempts or original requester")
	}
}

func sameWorkFactoryObservation(t *testing.T, f *fixture, caller map[string]string, work string) api.WorkerSessionObservation {
	t.Helper()
	status, raw := f.request(t, http.MethodGet, "/worker-sessions/"+caller["YOU_WORKER_SESSION_ID"], nil, nil)
	var observation api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(raw, &observation) != nil || observation.Direct ||
		len(observation.WorkIds) != 1 || observation.WorkIds[0] != work || observation.PredecessorWorkerSessionId != nil {
		t.Fatalf("expected separate Factory dispatches of the same Work: %d %s", status, raw)
	}
	return observation
}

func assertHeldReplyRedispatchInbox(t *testing.T, f *fixture, first, next, sibling map[string]string, ask, reply api.AgentMessage) {
	t.Helper()
	assertHeldReplyOriginalRequester(t, first["YOU_WORKER_SESSION_ID"], ask, reply)
	assertError(t, f, http.MethodGet, "/messages/"+reply.MessageId, nil, first, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	assertError(t, f, http.MethodGet, "/messages/"+reply.MessageId, nil, sibling, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	queued := readPage(t, f, "/messages?toMe=true", next)
	if len(queued.Messages) != 1 || !reflect.DeepEqual(queued.Messages[0], reply) {
		t.Fatal("HTTP same-Work inbox lost the queued reply")
	}
	var page api.AgentMessageListResponse
	if json.Unmarshal(f.cli(t, next, "list", "--to-me", "--mark-read"), &page) != nil ||
		len(page.Messages) != 1 || page.Messages[0].MessageId != reply.MessageId || page.Messages[0].Status != "READ" {
		t.Fatalf("fresh same-Work visit did not pull the original held reply: %+v", page)
	}
	if observed := readPage(t, f, "/messages?toMe=true", next); !reflect.DeepEqual(page, observed) {
		t.Fatal("CLI and HTTP same-Work inbox representations differ")
	}
	assertRepliedParent(t, f, ask.MessageId, reply.MessageId)
}

func assertHeldReplyOriginalRequester(t *testing.T, original string, ask, reply api.AgentMessage) {
	t.Helper()
	if reply.IfEnded != "HOLD" || reply.Status != "QUEUED" || reply.RevivedWorkerSessionId != nil || reply.Reason != nil ||
		reply.To.WorkerSessionId != original || reply.ThreadId != ask.ThreadId || reply.InReplyTo == nil || *reply.InReplyTo != ask.MessageId {
		t.Fatalf("held reply lost original requester or delivery policy: %+v", reply)
	}
}
