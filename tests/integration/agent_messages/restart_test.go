package agent_messages_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

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
	var sent api.AgentMessageSendResponse
	f.message(t, first.url, lane, &sent, "lead", "--request-id", "send", "--body", "compiled coordination")
	var page api.AgentMessageListResponse
	f.message(t, first.url, lead, &page, "list", "--to-me", "--mark-read")
	if len(page.Messages) != 1 || page.Messages[0].MessageId != sent.Message.MessageId || page.Messages[0].Status != "READ" {
		t.Fatal("compiled recipient did not pull/read the queued message")
	}
	var reply api.AgentMessageSendResponse
	f.message(t, first.url, lead, &reply, "reply", sent.Message.MessageId, "--request-id", "reply", "--body", "compiled acknowledgement")
	if reply.Message.ThreadId != sent.Message.ThreadId || reply.Message.To.WorkerSessionId != "lane" {
		t.Fatal("compiled reply changed recipient/thread")
	}
	before := f.readThread(t, first.url, sent.Message.ThreadId)
	f.releaseCaller(t, first.url, "lane")
	f.releaseCaller(t, first.url, "lead")
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

func (f *artifactFixture) assertRecoveredSuccessorInbox(t *testing.T, server string, next, previous map[string]string, sent, reply api.AgentMessageSendResponse) {
	t.Helper()
	status, body := f.http(t, server, http.MethodGet, "/messages?toMe=true", nil, next)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || len(page.Messages) != 1 ||
		!reflect.DeepEqual(page.Messages[0], reply.Message) {
		t.Fatalf("fresh successor did not recover the original inbox: status=%d", status)
	}
	var replay api.AgentMessageSendResponse
	f.message(t, server, next, &replay, "lead", "--request-id", "send", "--body", "compiled coordination")
	if replay.Message.MessageId != sent.Message.MessageId || replay.Message.Status != "REPLIED" {
		t.Fatal("fresh successor did not recover the original request alias")
	}
	status, _ = f.http(t, server, http.MethodGet, "/messages?toMe=true", nil, previous)
	if status != http.StatusForbidden {
		t.Fatal("stale predecessor token gained inbox authority after continuation")
	}
}
