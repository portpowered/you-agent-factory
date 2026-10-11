package apicontract_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAgentMessageGeneratedSendPreservesAddressAndBody(t *testing.T) {
	t.Parallel()
	caller, scope := "worker/one", "factory/one"
	request, err := generatedclient.NewSendAgentMessageRequest("http://localhost:7437", &generatedclient.SendAgentMessageParams{
		XYouWorkerSessionId: &caller, FactorySessionId: &scope,
	}, generatedclient.AgentMessageSendRequest{
		RequestId: "send-one", Body: "hello", To: &generatedclient.AgentMessageAddress{WorkerSessionId: "recipient"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := request.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	if request.Method != http.MethodPost || request.URL.Path != "/messages" || request.URL.Query().Get("factorySessionId") != scope || request.Header.Get("X-You-Worker-Session-Id") != caller {
		t.Fatal("generated send request lost its address or caller selector")
	}
	var body any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if err := loadValidatedOpenAPIContract(t).Components.Schemas["AgentMessageSendRequest"].Value.VisitJSON(body); err != nil {
		t.Fatalf("generated send body violates published schema: %v", err)
	}
}

func TestAgentMessageGeneratedListPreservesInboxFilters(t *testing.T) {
	t.Parallel()
	caller, scope := "worker/one", "factory/one"
	toMe, markRead, follow, maximum := true, true, false, 7
	to, from, thread, work, correlationScope, cursor := "recipient", "sender", "msg-thread", "work/one", "correlation/factory", "opaque+cursor"
	statuses := []generatedclient.AgentMessageDeliveryStatus{generatedclient.AgentMessageDeliveryStatusQUEUED, generatedclient.AgentMessageDeliveryStatusREAD}
	request, err := generatedclient.NewListAgentMessagesRequest("http://localhost:7437", &generatedclient.ListAgentMessagesParams{
		XYouWorkerSessionId: &caller, FactorySessionId: &scope, ToMe: &toMe, MarkRead: &markRead, Follow: &follow,
		ToWorkerSessionId: &to, FromWorkerSessionId: &from, ThreadId: &thread, CorrelationWorkId: &work,
		CorrelationFactorySessionId: &correlationScope, NextToken: &cursor, MaxResults: &maximum, Status: &statuses,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodGet || request.URL.Path != "/messages" || request.Header.Get("X-You-Worker-Session-Id") != caller {
		t.Fatal("generated inbox request lost its route or caller selector")
	}
	query := request.URL.Query()
	for key, expected := range map[string]string{
		"factorySessionId": scope, "toMe": "true", "markRead": "true", "follow": "false", "maxResults": "7",
		"toWorkerSessionId": to, "fromWorkerSessionId": from, "threadId": thread, "correlationWorkId": work,
		"correlationFactorySessionId": correlationScope, "nextToken": cursor,
	} {
		if query.Get(key) != expected {
			t.Errorf("%s = %q, want %q", key, query.Get(key), expected)
		}
	}
	if !reflect.DeepEqual(query["status"], []string{"QUEUED", "READ"}) {
		t.Fatalf("status filter is not an exploded array: %v", query["status"])
	}
}

func TestAgentMessageGeneratedGetPreservesMessageSelector(t *testing.T) {
	t.Parallel()
	caller := "worker/one"
	request, err := generatedclient.NewGetAgentMessageRequest("http://localhost:7437", "msg-one", &generatedclient.GetAgentMessageParams{XYouWorkerSessionId: &caller})
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodGet || request.URL.Path != "/messages/msg-one" || request.Header.Get("X-You-Worker-Session-Id") != caller {
		t.Fatal("generated get request lost its message or caller selector")
	}
}

func TestAgentMessageGeneratedClientDecodesUnavailableAndLimits(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status int
		code   string
	}{
		{http.StatusServiceUnavailable, "MESSAGING_DISABLED"},
		{http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED"},
	} {
		response, err := generatedclient.ParseSendAgentMessageClientResponse(&http.Response{
			StatusCode: test.status, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"message":"` + test.code + `","family":"INTERNAL_SERVER_ERROR","code":"` + test.code + `"}`)),
		})
		if err != nil {
			t.Fatal(err)
		}
		decoded := response.JSON503
		if test.status == http.StatusTooManyRequests {
			decoded = response.JSON429
		}
		if decoded == nil || string(decoded.Code) != test.code {
			t.Fatalf("status %d lost structured error: %+v", test.status, response)
		}
	}
}

func TestAgentMessageSendSchemaAndGeneratedValues(t *testing.T) {
	t.Parallel()
	schema := loadValidatedOpenAPIContract(t).Components.Schemas["AgentMessageSendRequest"].Value
	for _, test := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"send", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"}}`, true},
		{"reply", `{"requestId":"request","body":"hello","inReplyTo":"parent"}`, true},
		{"explicit reply", `{"requestId":"request","body":"hello","inReplyTo":"parent","to":{"workerSessionId":"worker"},"bodySecret":false,"delivery":"QUEUE","ifEnded":"HOLD","replyIfEnded":"REVIVE","expiresInSeconds":60}`, true},
		{"future interrupt intent", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"},"delivery":"INTERRUPT","expiresInSeconds":604800}`, true},
		{"missing recipient", `{"requestId":"request","body":"hello"}`, false},
		{"empty reply", `{"requestId":"request","body":"hello","inReplyTo":""}`, false},
		{"operator target", `{"requestId":"request","body":"hello","to":{"operator":true}}`, false},
		{"scope in address", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker","factorySessionId":"factory"}}`, false},
		{"forged sender", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"},"from":{"workerSessionId":"sender"}}`, false},
		{"body credential", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"},"token":"credential"}`, false},
		{"empty body", `{"requestId":"request","body":"","to":{"workerSessionId":"worker"}}`, false},
		{"expiry too short", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"},"expiresInSeconds":59}`, false},
		{"expiry too long", `{"requestId":"request","body":"hello","to":{"workerSessionId":"worker"},"expiresInSeconds":604801}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var input any
			if err := json.Unmarshal([]byte(test.body), &input); err != nil {
				t.Fatal(err)
			}
			if err := schema.VisitJSON(input); (err == nil) != test.valid {
				t.Fatalf("valid = %v, schema error = %v", test.valid, err)
			}
			if !test.valid {
				return
			}
			var server factoryapi.AgentMessageSendRequest
			if err := json.Unmarshal([]byte(test.body), &server); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(server)
			if err != nil {
				t.Fatal(err)
			}
			var output any
			if err := json.Unmarshal(encoded, &output); err != nil {
				t.Fatal(err)
			}
			if err := schema.VisitJSON(output); err != nil {
				t.Fatalf("generated round trip violates schema: %v", err)
			}
			var client generatedclient.AgentMessageSendRequest
			if err := json.Unmarshal(encoded, &client); err != nil {
				t.Fatal(err)
			}
			if client.Body != "hello" || client.RequestId != "request" || (client.To == nil && client.InReplyTo == nil) {
				t.Fatalf("generated client lost send/reply fields: %+v", client)
			}
		})
	}
}

func TestAgentMessageRepresentationsAcceptQueueMilestone(t *testing.T) {
	t.Parallel()
	doc := loadValidatedOpenAPIContract(t)
	message := map[string]any{
		"messageId": "msg-one", "threadId": "msg-one", "inReplyTo": nil,
		"from": map[string]any{"principal": "WORKER", "workerSessionId": "sender", "factorySessionId": nil},
		"to":   map[string]any{"kind": "WORKER_SESSION", "workerSessionId": "recipient", "factorySessionId": nil, "deliveredWorkerSessionId": nil},
		"body": "[REDACTED]", "bodySha256": "62a44c0656c231465f80ba45ce11ca177dc58220f70279db2a1a5ebf260cc662",
		"bodyRedactionCount": 1, "delivery": "QUEUE", "ifEnded": "REVIVE", "replyIfEnded": "REVIVE",
		"reason": "REVIVE_UNSUPPORTED", "hop": 0, "sentAt": "2026-10-11T01:00:00Z", "expiresAt": "2026-10-12T01:00:00Z",
	}
	for _, status := range []string{"QUEUED", "READ", "REPLIED", "EXPIRED"} {
		message["status"] = status
		if err := doc.Components.Schemas["AgentMessage"].Value.VisitJSON(message); err != nil {
			t.Fatalf("%s message violates schema: %v", status, err)
		}
		observation := map[string]any{"recordId": "transaction/message", "sequence": 1, "kind": "SENT", "message": message}
		if err := doc.Components.Schemas["AgentMessageObservation"].Value.VisitJSON(observation); err != nil {
			t.Fatalf("observation violates schema: %v", err)
		}
	}
	if err := doc.Components.Schemas["AgentMessageListResponse"].Value.VisitJSON(map[string]any{"messages": []any{}, "nextToken": nil}); err != nil {
		t.Fatalf("empty inbox violates schema: %v", err)
	}
	message["from"] = map[string]any{"principal": "OPERATOR", "workerSessionId": "sender"}
	if err := doc.Components.Schemas["AgentMessage"].Value.VisitJSON(message); err == nil {
		t.Fatal("schema accepted an operator sender")
	}
}

func TestAgentMessageStreamErrorsMatchPublishedAndGeneratedContracts(t *testing.T) {
	t.Parallel()
	schema := loadValidatedOpenAPIContract(t).Components.Schemas["ErrorResponse"].Value
	for _, code := range []factoryapi.ErrorResponseCode{
		factoryapi.ErrorResponseCodeMESSAGESTREAMUNAVAILABLE,
		factoryapi.ErrorResponseCodeMESSAGESTREAMGAP,
		factoryapi.ErrorResponseCodeMESSAGESTREAMBACKPRESSURE,
	} {
		family := factoryapi.ErrorFamilyInternalServerError
		if code == factoryapi.ErrorResponseCodeMESSAGESTREAMGAP {
			family = factoryapi.ErrorFamilyGone
		}
		encoded, err := json.Marshal(factoryapi.ErrorResponse{Code: code, Message: string(code), Family: family})
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.VisitJSON(value); err != nil {
			t.Fatalf("stream error violates public schema: %v", err)
		}
		var client generatedclient.ErrorResponse
		if err := json.Unmarshal(encoded, &client); err != nil || string(client.Code) != string(code) || string(client.Family) != string(family) {
			t.Fatalf("generated client lost stream failure: %+v %v", client, err)
		}
	}
}
