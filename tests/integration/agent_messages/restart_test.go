package agent_messages_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// One prebuilt artifact, two sequential real hosts and controlled local
// provider children protect the compiled CLI and durable restart boundary.
func TestPrebuiltAuthorizedInboxSurvivesHostRestart(t *testing.T) {
	t.Parallel()
	f := newArtifactFixture(t)
	first := f.startHost(t)
	lead := f.invoke(t, first.url, "lead", nil)
	lane := f.invoke(t, first.url, "lane", lead)
	f.releaseCaller(t, first.url, "lead")
	var sent api.AgentMessageSendResponse
	f.message(t, first.url, lane, &sent, "lead", "--request-id", "send", "--reply-hold", "--body", "compiled coordination")
	revived := f.confirmCompiledRevival(t, first.url, lane, sent)
	var page api.AgentMessageListResponse
	f.message(t, first.url, revived, &page, "list", "--to-me")
	if len(page.Messages) != 1 || page.Messages[0].MessageId != sent.Message.MessageId || page.Messages[0].Status != "DELIVERED_REVIVE" {
		t.Fatal("compiled successor did not receive the revived ask")
	}
	f.releaseCaller(t, first.url, "lane")
	var reply api.AgentMessageSendResponse
	f.message(t, first.url, revived, &reply, "reply", sent.Message.MessageId, "--request-id", "reply", "--body", "compiled acknowledgement")
	if reply.Message.ThreadId != sent.Message.ThreadId || reply.Message.To.WorkerSessionId != "lane" || reply.Message.IfEnded != "HOLD" || reply.Message.Status != "QUEUED" || reply.Message.RevivedWorkerSessionId != nil {
		t.Fatal("compiled reply changed recipient/thread")
	}
	before := f.readThread(t, first.url, sent.Message.ThreadId)
	f.releaseCaller(t, first.url, revived["YOU_WORKER_SESSION_ID"])
	first.stop(t, f)
	second := f.startHost(t)
	after := f.readThread(t, second.url, sent.Message.ThreadId)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("host reconstruction changed durable messages")
	}
	status, body := f.http(t, second.url, http.MethodPost, "/messages", map[string]any{"requestId": "old-token", "to": map[string]string{"workerSessionId": "lead"}, "body": "must refuse"}, lane)
	var failure api.ErrorResponse
	if status != http.StatusForbidden || json.Unmarshal(body, &failure) != nil || failure.Code != "MESSAGE_NOT_PERMITTED" {
		t.Fatal("host reconstruction retained old execution authority")
	}
	next := f.continueCallerAfterRestart(t, second.url, lane)
	f.assertRecoveredSuccessorInbox(t, second.url, next, lane, sent, reply)
	f.releaseCaller(t, second.url, "lane-next")
	second.stop(t, f)
}

// The real controlled child reports credentials/input before emitting native
// progress. OS file/HTTP polling observes that boundary; integration cannot
// replace it with an in-process command-runner edge.
func (f *artifactFixture) confirmCompiledRevival(t *testing.T, server string, lane map[string]string, sent api.AgentMessageSendResponse) map[string]string {
	t.Helper()
	if sent.Message.Status != "QUEUED" || sent.Message.Reason == nil || *sent.Message.Reason != "REVIVE_PENDING" || sent.Message.RevivedWorkerSessionId != nil {
		t.Fatal("compiled send guessed provider delivery")
	}
	status, body := f.http(t, server, http.MethodGet, "/worker-sessions/lead", nil, nil)
	var lead api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(body, &lead) != nil || lead.SuccessorWorkerSessionId == nil {
		t.Fatal("compiled revival did not retain successor lineage")
	}
	id := *lead.SuccessorWorkerSessionId
	next := f.workerCredentials(t, id)
	f.assertCompiledResumeInput(t, id, sent.Message.Body)
	if next["YOU_WORKER_SESSION_TOKEN"] == lane["YOU_WORKER_SESSION_TOKEN"] {
		t.Fatal("revival reused sender authority")
	}

	if err := os.WriteFile(filepath.Join(f.dir, id+"-progress"), []byte("progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.awaitCompiledDelivery(t, server, lane, sent.Message, id)
	return next
}

func (f *artifactFixture) assertCompiledResumeInput(t *testing.T, id, body string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, id+"-input.json"))
	var input struct {
		Args  []string `json:"args"`
		Input string   `json:"input"`
	}
	if err != nil || json.Unmarshal(data, &input) != nil || input.Input != body {
		t.Fatal("compiled provider did not receive the normalized original ask")
	}
	resume := false
	for i, arg := range input.Args {
		if arg == "resume" && i+1 < len(input.Args) && input.Args[i+1] == "controlled-lead" {
			resume = true
		}
	}
	if !resume {
		t.Fatal("compiled revival changed recorded resume argv or execution authority")
	}
}

func (f *artifactFixture) awaitCompiledDelivery(t *testing.T, server string, lane map[string]string, message api.AgentMessage, id string) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, body := f.http(t, server, http.MethodGet, "/messages/"+message.MessageId, nil, lane)
		var delivered api.AgentMessage
		if status == http.StatusOK && json.Unmarshal(body, &delivered) == nil && delivered.Status == "DELIVERED_REVIVE" {
			if delivered.RevivedWorkerSessionId == nil || *delivered.RevivedWorkerSessionId != id || delivered.Body != message.Body {
				t.Fatal("compiled delivery changed successor or input")
			}
			return
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatal("compiled provider progress did not settle delivery")
		}
	}
}

func (f *artifactFixture) assertRecoveredSuccessorInbox(t *testing.T, server string, next, previous map[string]string, sent, reply api.AgentMessageSendResponse) {
	t.Helper()
	status, body := f.http(t, server, http.MethodGet, "/messages?toMe=true", nil, next)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || len(page.Messages) != 1 ||
		!reflect.DeepEqual(page.Messages[0], reply.Message) {
		t.Fatalf("fresh successor did not recover the original inbox: status=%d", status)
	}
	var replay api.AgentMessageSendResponse
	f.message(t, server, next, &replay, "lead", "--request-id", "send", "--reply-hold", "--body", "compiled coordination")
	if replay.Message.MessageId != sent.Message.MessageId || replay.Message.Status != "REPLIED" {
		t.Fatal("fresh successor did not recover the original request alias")
	}
	status, _ = f.http(t, server, http.MethodGet, "/messages?toMe=true", nil, previous)
	if status != http.StatusForbidden {
		t.Fatal("stale predecessor token gained inbox authority after continuation")
	}
}
