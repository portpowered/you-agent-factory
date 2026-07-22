package factorysessionexecution

import (
	"encoding/json"
	"testing"
)

func TestMaterializeEventReadStream_OwnsFiniteClosedLifecycleAndDetachedHistory(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"id":"event-1","type":"factory.session.started","sequence":1,"payload":{"value":"original"}}`)
	stream := MaterializeEventReadStream(EventReadResult{Events: []json.RawMessage{
		raw,
		json.RawMessage(`{not-json}`),
	}})
	if stream == nil || len(stream.History) != 1 || stream.History[0].Id != "event-1" {
		t.Fatalf("stream = %#v, want one canonical history event", stream)
	}
	if _, open := <-stream.Events; open {
		t.Fatal("finite durable event stream live channel is open")
	}
	raw[len(raw)-3] = 'X'
	if got := string(stream.History[0].Payload); got != `{"value":"original"}` {
		t.Fatalf("detached payload = %q, want original payload", got)
	}
}

func TestMaterializeEventReadStream_EmptyResultStillReturnsClosedStream(t *testing.T) {
	t.Parallel()
	stream := MaterializeEventReadStream(EventReadResult{})
	if stream == nil || len(stream.History) != 0 {
		t.Fatalf("stream = %#v, want empty materialized stream", stream)
	}
	if _, open := <-stream.Events; open {
		t.Fatal("empty durable event stream live channel is open")
	}
}
