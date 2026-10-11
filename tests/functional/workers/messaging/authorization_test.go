package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Hold the existing timestamp effect after authentication and before the
// durable write. Lifecycle deadlines continue using the real scheduler.
type admissionClock struct {
	armed            atomic.Bool
	entered, release chan struct{}
}

func (c *admissionClock) Now() time.Time {
	if c.armed.CompareAndSwap(true, false) {
		close(c.entered)
		<-c.release
	}
	return time.Now().UTC()
}

func TestAdmissionRevalidatesCallerAfterWaiting(t *testing.T) {
	t.Parallel()
	clock := &admissionClock{entered: make(chan struct{}), release: make(chan struct{})}
	f := startHostEffects(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{Clock: clock}, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	existing := f.send(t, map[string]any{"requestId": "existing", "to": map[string]string{"workerSessionId": "lead"}, "body": "existing inbox"}, lane)
	clock.armed.Store(true)
	released := false
	defer func() {
		if !released {
			close(clock.release)
		}
	}()
	finished := make(chan struct{}, 1)
	go func() {
		defer close(finished)
		assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "waiting", "to": map[string]string{"workerSessionId": "lead"}, "body": "must never be admitted"}, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	select {
	case <-clock.entered:
	case <-ctx.Done():
		t.Fatal("admission did not reach timestamp gate")
	}
	status, _ := f.request(t, http.MethodPost, "/worker-sessions/lane/cancel", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatal("caller did not lose execution authority")
	}
	close(clock.release)
	released = true
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("waiting admission did not join")
	}
	page := readPage(t, f, "/messages", nil)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != existing.MessageId || page.Messages[0].Status != "QUEUED" {
		t.Fatal("caller loss persisted a waiting send or changed the existing inbox")
	}
}

func TestLegacyRecipientAmbiguityRequiresExplicitOwner(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	f.invoke(t, "lane", lead)
	status, data := f.request(t, http.MethodGet, "/worker-sessions/lane", nil, nil)
	var live api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(data, &live) != nil || live.FactorySessionId == nil {
		t.Fatal("missing live owner identity")
	}
	peer := uuid.NewString()
	f.replayPeer(t, "lane", peer)
	request := map[string]any{"requestId": "resolve-recipient", "to": map[string]string{"workerSessionId": "lane"}, "body": "explicit owned recipient", "ifEnded": "HOLD"}
	status, data = f.request(t, http.MethodPost, "/messages", request, lead)
	var refused api.ErrorResponse
	if status != http.StatusConflict || json.Unmarshal(data, &refused) != nil || refused.Code != "WORKER_SESSION_AMBIGUOUS" {
		t.Fatalf("legacy recipient ambiguity: %d %s", status, data)
	}
	assertRecipientCandidates(t, data, *live.FactorySessionId, peer)
	assertCLIAmbiguousRecipient(t, f, lead, *live.FactorySessionId, peer)
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("ambiguous recipient created a message")
	}
	status, data = f.request(t, http.MethodPost, "/messages?factorySessionId="+*live.FactorySessionId, request, lead)
	var result api.AgentMessageSendResponse
	if status != http.StatusAccepted || json.Unmarshal(data, &result) != nil || result.Message.To.FactorySessionId == nil || *result.Message.To.FactorySessionId != *live.FactorySessionId {
		t.Fatalf("scoped owned recipient: %d %s", status, data)
	}
	if replay := f.cliSend(t, lead, "lane", "--session", *live.FactorySessionId, "--request-id", "resolve-recipient", "--body", "explicit owned recipient", "--if-ended", "hold"); replay.MessageId != result.Message.MessageId {
		t.Fatal("CLI explicit recipient scope did not replay the HTTP record")
	}
	assertError(t, f, http.MethodPost, "/messages?factorySessionId="+peer, map[string]any{"requestId": "peer", "to": map[string]string{"workerSessionId": "lane"}, "body": "foreign historical peer"}, lead, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 1 {
		t.Fatal("explicit scope widened permission or duplicated the record")
	}
	// HOLD prevents teardown from starting a revival. Join complete captures
	// before profile removal using the exact owner for the ambiguous recipient;
	// the equal-ID replay peer keeps its existing independent cleanup join.
	f.endWorker(t, "lead")
	close(f.runner.workers["lane"].release)
	support.GetWorkerSessionEventsForSessionByIDAt(t, f.baseURL(), *live.FactorySessionId, "lane")
}

func assertCLIAmbiguousRecipient(t *testing.T, f *fixture, lead map[string]string, live, peer string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", f.server.URL(), "message", "lane", "--request-id", "resolve-recipient", "--body", "explicit owned recipient", "--output", "json"})
	inputs.Input.Env = append(append([]string{}, f.env...), "YOU_WORKER_SESSION_ID=lead", "YOU_WORKER_SESSION_TOKEN="+lead["YOU_WORKER_SESSION_TOKEN"])
	inputs.Input.WorkingDirectory = f.dir
	if err := f.server.Execute(t, inputs.Input); err == nil {
		t.Fatal("CLI admitted an ambiguous recipient")
	}
	if inputs.Stdout() != "" {
		t.Fatal("CLI refusal emitted a success response")
	}
	assertRecipientCandidates(t, []byte(inputs.Stderr()), live, peer)
}

func assertRecipientCandidates(t *testing.T, data []byte, live, peer string) {
	t.Helper()
	var result struct {
		Code    string `json:"code"`
		Details struct {
			Candidates []api.WorkerSessionAddressCandidate `json:"candidates"`
		} `json:"details"`
	}
	if json.Unmarshal(data, &result) != nil || result.Code != "WORKER_SESSION_AMBIGUOUS" || len(result.Details.Candidates) != 2 {
		t.Fatalf("missing typed ambiguity candidates: %s", data)
	}
	owners := map[string]string{}
	for _, candidate := range result.Details.Candidates {
		if candidate.WorkerSessionId != "lane" {
			t.Fatal("ambiguity candidate has a foreign identity")
		}
		owners[candidate.FactorySessionId] = string(candidate.State)
	}
	if owners[live] != "RUNNING" || owners[peer] != "COMPLETED" {
		t.Fatalf("ambiguity candidates lost exact owner or state: live=%s peer=%s response=%s", live, peer, data)
	}
}

func TestProjectNamesAndSiblingLabelsGrantNoMessageAuthority(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":1}}}`, "lead", "lane", "sibling", "foreign-lead", "foreign-lane")
	lead := f.invokeProject(t, "lead", "owned-project", nil)
	lane := f.invokeProject(t, "lane", "owned-project", lead)
	sibling := f.invokeProject(t, "sibling", "owned-project", lead)
	foreignLead := f.invokeProject(t, "foreign-lead", "foreign-project", nil)
	foreign := f.invokeProject(t, "foreign-lane", "foreign-project", foreignLead)
	_, laneDetail := f.request(t, http.MethodGet, "/worker-sessions/lane", nil, nil)
	_, siblingDetail := f.request(t, http.MethodGet, "/worker-sessions/sibling", nil, nil)
	var laneObservation, siblingObservation api.WorkerSessionObservation
	if json.Unmarshal(laneDetail, &laneObservation) != nil || json.Unmarshal(siblingDetail, &siblingObservation) != nil ||
		laneObservation.Labels == nil || len(*laneObservation.Labels) == 0 || !reflect.DeepEqual(laneObservation.Labels, siblingObservation.Labels) {
		t.Fatal("fixture must expose genuine siblings with identical server-held labels")
	}
	for _, target := range []string{"sibling", "foreign-lane", "foreign-lead"} {
		assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "retry-owned", "to": map[string]string{"workerSessionId": target}, "body": "denied across relationship"}, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	}
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "foreign", "to": map[string]string{"workerSessionId": "lane"}, "body": "foreign refusal"}, foreign, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("denied relationship stored a message")
	}
	// Reusing the denied request key at the one-send limit proves that neither
	// aliases nor successful-send quota were spent by the denials.
	message := f.cliSend(t, lane, "lead", "--request-id", "retry-owned", "--body", "permitted requester")
	assertError(t, f, http.MethodGet, "/messages/"+message.MessageId, nil, sibling, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	assertError(t, f, http.MethodGet, "/messages/"+message.MessageId, nil, foreign, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	for _, caller := range []map[string]string{sibling, foreign} {
		if page := readPage(t, f, "/messages?toMe=true&markRead=true", caller); len(page.Messages) != 0 {
			t.Fatal("unrelated label/project granted inbox authority")
		}
	}
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" {
		t.Fatal("unrelated readers changed recipient state")
	}
}

func TestInvalidCallerNeverFallsBackToOperator(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane", "root")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	root := f.invoke(t, "root", nil)
	message := f.cliSend(t, lane, "lead", "--request-id", "valid", "--body", "authorized")
	for _, cell := range []struct {
		name   string
		caller map[string]string
	}{
		{"id only", map[string]string{"YOU_WORKER_SESSION_ID": "lane"}},
		{"token only", map[string]string{"YOU_WORKER_SESSION_TOKEN": lane["YOU_WORKER_SESSION_TOKEN"]}},
		{"forged", map[string]string{"YOU_WORKER_SESSION_ID": "lane", "YOU_WORKER_SESSION_TOKEN": strings.Repeat("A", 43)}},
		{"foreign", map[string]string{"YOU_WORKER_SESSION_ID": "root", "YOU_WORKER_SESSION_TOKEN": lane["YOU_WORKER_SESSION_TOKEN"]}},
	} {
		t.Run(cell.name, func(t *testing.T) {
			code := "MESSAGE_NOT_PERMITTED"
			if cell.name == "id only" || cell.name == "token only" {
				code = "WORKER_SESSION_CALLER_INVALID"
			}
			request := map[string]any{"requestId": cell.name, "to": map[string]string{"workerSessionId": "lead"}, "body": "refused"}
			assertError(t, f, http.MethodPost, "/messages", request, cell.caller, http.StatusForbidden, code)
			assertError(t, f, http.MethodGet, "/messages/"+message.MessageId, nil, cell.caller, http.StatusForbidden, code)
			assertError(t, f, http.MethodGet, "/messages?follow=true", nil, cell.caller, http.StatusForbidden, code)
		})
	}
	for _, caller := range []map[string]string{lane, root} {
		target := "root"
		if caller["YOU_WORKER_SESSION_ID"] == "root" {
			target = "lead"
		}
		assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "unrelated", "to": map[string]string{"workerSessionId": target}, "body": "refused"}, caller, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	}
	assertError(t, f, http.MethodGet, "/messages?toMe=true&markRead=true", nil, nil, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	page := readPage(t, f, "/messages", nil)
	if len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" {
		t.Fatal("refused callers created a message or changed recipient state")
	}
	// The genuine lead's descendant permission survives the denied requests.
	f.cliSend(t, lead, "lane", "--request-id", "descendant", "--body", "lead to own worker")
}

func TestEndedRecipientKeepsQueueIntentWithoutExecution(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane")
	// Cancellation precedes native provider-session discovery in this cell.
	f.runner.workers["lead"].progress = make(chan struct{})
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	status, _ := f.request(t, http.MethodPost, "/worker-sessions/lead/cancel", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatal("recipient cancellation failed")
	}
	status, before := f.request(t, http.MethodGet, "/worker-sessions/lead", nil, nil)
	var ended api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(before, &ended) != nil || ended.State != "CANCELED" || ended.SuccessorWorkerSessionId != nil {
		t.Fatal("recipient did not end without a successor")
	}
	for _, cell := range []struct{ policy, body string }{{"HOLD", "hold for later pull"}, {"", "default revive intent"}} {
		request := map[string]any{"requestId": cell.body, "to": map[string]string{"workerSessionId": "lead"}, "body": cell.body}
		if cell.policy != "" {
			request["ifEnded"] = cell.policy
		}
		message := f.send(t, request, lane)
		assertEndedQueueIntent(t, message, cell.policy)
	}
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "interrupt", "to": map[string]string{"workerSessionId": "lead"}, "body": "unsupported interrupt", "delivery": "INTERRUPT"}, lane, http.StatusBadRequest, "MESSAGE_INTERRUPT_UNSUPPORTED")
	assertError(t, f, http.MethodGet, "/messages?toMe=true", nil, lead, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	if page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil); len(page.Messages) != 2 {
		t.Fatal("refused interrupt changed the ended recipient inbox")
	}
	status, after := f.request(t, http.MethodGet, "/worker-sessions/lead", nil, nil)
	if status != http.StatusOK || !bytes.Equal(before, after) {
		t.Fatal("queued messaging changed the terminal Worker Session")
	}
}

func assertEndedQueueIntent(t *testing.T, message api.AgentMessage, policy string) {
	t.Helper()
	if policy == "" {
		policy = "REVIVE"
	}
	if message.Status != "QUEUED" || string(message.IfEnded) != policy || message.RevivedWorkerSessionId != nil || message.To.DeliveredWorkerSessionId != nil {
		t.Fatal("ended recipient message changed queue intent or claimed execution")
	}
	if policy == "REVIVE" && (message.Reason == nil || *message.Reason != "REVIVE_UNSUPPORTED") {
		t.Fatal("default revive intent omitted its T5 limitation")
	}
	if policy == "HOLD" && message.Reason != nil {
		t.Fatal("hold incorrectly claimed unsupported revival")
	}
}

func readPage(t *testing.T, f *fixture, path string, caller map[string]string) api.AgentMessageListResponse {
	t.Helper()
	status, bytes := f.request(t, http.MethodGet, path, nil, caller)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(bytes, &page) != nil {
		t.Fatalf("public inbox: %d %s", status, bytes)
	}
	return page
}
