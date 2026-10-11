package messaging

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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

func readPage(t *testing.T, f *fixture, path string, caller map[string]string) api.AgentMessageListResponse {
	t.Helper()
	status, bytes := f.request(t, http.MethodGet, path, nil, caller)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(bytes, &page) != nil {
		t.Fatalf("public inbox: %d %s", status, bytes)
	}
	return page
}
