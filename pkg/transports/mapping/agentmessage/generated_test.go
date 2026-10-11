package agentmessage

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSendRequestPreservesOmissionsAndDetachesExplicitValues(t *testing.T) {
	t.Parallel()
	mapper := Mapper{}
	var omitted factoryapi.AgentMessageSendRequest
	if err := json.Unmarshal([]byte(`{"requestId":"request","body":" reply ","inReplyTo":"parent"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	result := mapper.SendRequest(omitted)
	want := agentmessages.SendRequest{RequestID: "request", Body: " reply ", InReplyTo: "parent"}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("mapping applied policy or lost the reply: %+v", result)
	}
	var explicit factoryapi.AgentMessageSendRequest
	if err := json.Unmarshal([]byte(`{"requestId":"request","body":"secret","bodySecret":true,"to":{"workerSessionId":"worker"},"delivery":"INTERRUPT","ifEnded":"HOLD","replyIfEnded":"REVIVE","expiresInSeconds":0,"correlation":{"workId":"work","factorySessionId":"factory"}}`), &explicit); err != nil {
		t.Fatal(err)
	}
	result = mapper.SendRequest(explicit)
	expiry := 0
	want = agentmessages.SendRequest{
		RequestID: "request", Body: "secret", BodySecret: true, To: &agentmessages.Address{WorkerSessionID: "worker"},
		Delivery: "INTERRUPT", IfEnded: "HOLD", ReplyIfEnded: "REVIVE", ExpiresInSeconds: &expiry,
		Correlation: agentmessages.Correlation{WorkID: "work", FactorySessionID: "factory"},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("explicit values were changed before service validation: %+v", result)
	}
	explicit.To.WorkerSessionId = "changed"
	*explicit.ExpiresInSeconds = 60
	*explicit.Correlation.WorkId = "changed"
	if !reflect.DeepEqual(result, want) {
		t.Fatal("mapped input aliases transport values")
	}
}

func TestMessageResponsePreservesDurableFactsAndNulls(t *testing.T) {
	t.Parallel()
	input := agentmessages.Message{
		MessageID: "msg-reply", ThreadID: "msg-parent", InReplyTo: "msg-parent",
		From:        agentmessages.Sender{Principal: "WORKER", WorkerSessionID: "lead", FactorySessionID: "factory"},
		To:          agentmessages.Recipient{Kind: "WORKER_SESSION", WorkerSessionID: "worker", FactorySessionID: "factory"},
		Correlation: agentmessages.Correlation{WorkID: "work"},
		Body:        "[REDACTED]", BodySHA256: "safe-hash", BodyRedactionCount: 1,
		Delivery: "QUEUE", IfEnded: "REVIVE", ReplyIfEnded: "HOLD", Status: agentmessages.Replied,
		Reason: "REVIVE_UNSUPPORTED", RepliedByMessageID: "msg-next", Hop: 2,
		SentAt:    time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC),
	}
	mapped := (Mapper{}).Message(input)
	encoded, err := json.Marshal(mapped)
	if err != nil {
		t.Fatal(err)
	}
	var client generatedclient.AgentMessage
	if err := json.Unmarshal(encoded, &client); err != nil {
		t.Fatal(err)
	}
	want := generatedclient.AgentMessage{
		MessageId: input.MessageID, ThreadId: input.ThreadID, InReplyTo: &input.InReplyTo,
		RepliedByMessageId: &input.RepliedByMessageID, Body: input.Body, BodySha256: input.BodySHA256,
		BodyRedactionCount: 1, Status: "REPLIED", Reason: &input.Reason, Hop: 2,
		SentAt: input.SentAt, ExpiresAt: input.ExpiresAt, Delivery: "QUEUE", IfEnded: "REVIVE", ReplyIfEnded: "HOLD",
	}
	want.From.Principal = "WORKER"
	want.From.WorkerSessionId = "lead"
	want.From.FactorySessionId = &input.From.FactorySessionID
	want.To.Kind = "WORKER_SESSION"
	want.To.WorkerSessionId = "worker"
	want.To.FactorySessionId = &input.To.FactorySessionID
	want.Correlation = &struct {
		FactorySessionId *string `json:"factorySessionId,omitempty"`
		WorkId           *string `json:"workId,omitempty"`
	}{WorkId: &input.Correlation.WorkID}
	if !reflect.DeepEqual(client, want) {
		t.Fatalf("generated client lost durable facts: %+v", client)
	}
	*mapped.From.FactorySessionId = "changed"
	*mapped.Correlation.WorkId = "changed"
	if input.From.FactorySessionID != "factory" || input.Correlation.WorkID != "work" {
		t.Fatal("response aliases service values")
	}
}

func TestPageRepresentsEmptyInboxAsArray(t *testing.T) {
	t.Parallel()
	mapped := (Mapper{}).Page(agentmessages.Page{})
	encoded, err := json.Marshal(mapped)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"messages":[],"nextToken":null}` {
		t.Fatalf("empty page = %s", encoded)
	}
	page := agentmessages.Page{Messages: []agentmessages.Message{{MessageID: "second"}, {MessageID: "first"}}, NextToken: "signed-cursor"}
	mapped = (Mapper{}).Page(page)
	if !reflect.DeepEqual([]string{mapped.Messages[0].MessageId, mapped.Messages[1].MessageId}, []string{"second", "first"}) || *mapped.NextToken != page.NextToken {
		t.Fatal("mapping changed store order or cursor")
	}
}
