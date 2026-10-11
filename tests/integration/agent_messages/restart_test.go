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
	second.stop(t, f)
}
