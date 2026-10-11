package agentmessages

import (
	"strings"
	"testing"
)

func TestObservationTopicsPreserveExactRecipientIdentity(t *testing.T) {
	t.Parallel()
	recipients := []Recipient{
		{FactorySessionID: "factory-a", WorkerSessionID: "legacy-worker"},
		{FactorySessionID: "factory-b", WorkerSessionID: "legacy-worker"},
		{FactorySessionID: "factory-a", WorkerSessionID: "another-worker"},
		{FactorySessionID: "a/b", WorkerSessionID: "c"},
		{FactorySessionID: "a", WorkerSessionID: "b/c"},
		{WorkerSessionID: "legacy-worker"},
		{FactorySessionID: strings.Repeat("工", 200), WorkerSessionID: strings.Repeat("w", 200)},
	}
	seen := make(map[string]bool)
	for _, recipient := range recipients {
		topic := recipient.ObservationTopic()
		if err := topic.Validate(); err != nil {
			t.Fatalf("opaque recipient did not produce a valid Events topic: %v", err)
		}
		if !strings.HasPrefix(string(topic), "messages/") || len(topic) != len("messages/")+64 {
			t.Fatal("recipient topic lost its bounded messaging namespace")
		}
		if seen[string(topic)] {
			t.Fatal("distinct exact recipients share an observation topic")
		}
		seen[string(topic)] = true
		if recipient.ObservationTopic() != topic {
			t.Fatal("recipient topic changed between calls")
		}
		// Delivery facts do not change the addressed inbox. Continuation and
		// own-Work read permissions must be checked separately by Messaging.
		recipient.DeliveredWorkerSessionID = "later-session"
		if recipient.ObservationTopic() != topic {
			t.Fatal("delivery facts redirected the exact recipient topic")
		}
	}
}
