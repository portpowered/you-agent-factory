package apicontract_test

import (
	"encoding/json"
	"testing"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
