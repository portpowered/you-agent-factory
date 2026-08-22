package workersessions_test

import (
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestTopic_IsDeterministicAndValid(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	if topic != "worker-session/worker-1/events" {
		t.Fatalf("Topic(%q) = %q, want worker-session/worker-1/events", "worker-1", topic)
	}
	if err := topic.Validate(); err != nil {
		t.Fatalf("Topic(%q).Validate() error = %v, want nil", "worker-1", err)
	}
	if second := workersessions.Topic("worker-1"); topic != second {
		t.Fatalf("Topic() is not deterministic for the same id: %q != %q", topic, second)
	}
	if other := workersessions.Topic("worker-2"); topic == other {
		t.Fatalf("Topic() collided across distinct ids: %q == %q", topic, other)
	}
}
