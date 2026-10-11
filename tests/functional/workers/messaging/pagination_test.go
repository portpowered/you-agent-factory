package messaging

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInboxPaginationFiltersAndRecipientRead(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane", "sibling")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	sibling := f.invoke(t, "sibling", lead)
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first")
	second := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second")
	f.cliSend(t, sibling, "lead", "--request-id", "other", "--body", "other sender")
	query := "/messages?toMe=true&fromWorkerSessionId=lane&status=QUEUED&maxResults=1"
	page := readPage(t, f, query, lead)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != first.MessageId || page.NextToken == nil {
		t.Fatal("first authorized page did not follow send order")
	}
	next := readPage(t, f, query+"&nextToken="+url.QueryEscape(*page.NextToken), lead)
	if len(next.Messages) != 1 || next.Messages[0].MessageId != second.MessageId || next.NextToken != nil {
		t.Fatal("second authorized page repeated, skipped or included another sender")
	}
	var cliPage api.AgentMessageListResponse
	if err := json.Unmarshal(f.cli(t, lead, "list", "--to-me", "--from-worker-session-id", "lane", "--status", "QUEUED", "--max-results", "1"), &cliPage); err != nil || !reflect.DeepEqual(page, cliPage) {
		t.Fatal("CLI and HTTP pagination representations differ")
	}
	empty := readPage(t, f, query+"&threadId="+url.QueryEscape(second.ThreadId)+"&toWorkerSessionId=sibling", lead)
	if len(empty.Messages) != 0 || empty.NextToken != nil {
		t.Fatal("inbox selectors were not combined with AND")
	}
	if own := readPage(t, f, "/messages?toMe=true", sibling); len(own.Messages) != 0 {
		t.Fatal("unrelated recipient received another inbox")
	}
	assertOnlyRecipientMarksRead(t, f, first, lane, lead)
}

func assertOnlyRecipientMarksRead(t *testing.T, f *fixture, first api.AgentMessage, lane, lead map[string]string) {
	t.Helper()
	for _, observed := range readPage(t, f, "/messages?toWorkerSessionId=lead&markRead=true", lane).Messages {
		if observed.Status != "QUEUED" {
			t.Fatal("sender mark-read changed recipient state")
		}
	}
	if observed := readPage(t, f, "/messages?threadId="+first.ThreadId, lane); len(observed.Messages) != 1 || observed.Messages[0].Status != "QUEUED" {
		t.Fatal("sender observation changed recipient read status")
	}
	status, bytes := f.request(t, http.MethodGet, "/messages/"+first.MessageId, nil, lead)
	var read api.AgentMessage
	if status != http.StatusOK || json.Unmarshal(bytes, &read) != nil || read.Status != "READ" {
		t.Fatal("recipient get did not mark the message read")
	}
	reply := f.cliSend(t, lead, "reply", first.MessageId, "--request-id", "reply", "--body", "ack")
	readPage(t, f, "/messages?toMe=true&markRead=true", lead)
	assertRepliedParent(t, f, first.MessageId, reply.MessageId)
}
