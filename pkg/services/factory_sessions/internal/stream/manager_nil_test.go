package stream_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
)

func TestManager_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var manager *stream.Manager
	session := &livesession.LiveSession{ID: "sess-nil"}

	if _, err := manager.Subscribe("sess-nil", "dispatch-1", 0); err == nil {
		t.Fatal("Subscribe = nil, want manager required")
	}
	if _, err := manager.DispatchIDs("sess-nil"); err == nil {
		t.Fatal("DispatchIDs = nil, want manager required")
	}
	manager.CloseAll(session)
	if manager.CloseDispatch(session, "dispatch-1") {
		t.Fatal("CloseDispatch = true, want false for nil manager")
	}
	if manager.InferenceProgressPublisherFactory(nil) != nil {
		t.Fatal("InferenceProgressPublisherFactory = non-nil, want nil")
	}
	if manager.DispatchCompletionObserverFactory() != nil {
		t.Fatal("DispatchCompletionObserverFactory = non-nil, want nil")
	}
}

func TestManager_Subscribe_RequiresSession(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{session: nil}
	manager := newTestManager(host)

	_, err := manager.Subscribe("missing", "dispatch-1", 0)
	if err == nil {
		t.Fatal("Subscribe = nil, want missing session")
	}
}

func TestManager_DispatchIDs_ReturnsNilForMissingStreams(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &livesession.LiveSession{ID: "sess-no-streams"},
	}
	host.streams = nil
	manager := newTestManager(host)

	ids, err := manager.DispatchIDs("sess-no-streams")
	if err != nil {
		t.Fatalf("DispatchIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("dispatch IDs = %#v, want empty", ids)
	}
}

func TestManager_InferenceProgressPublisher_RecoversFromMissingSession(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{session: nil}
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("missing")
	if publisher == nil {
		t.Fatal("publisher = nil, want no-op publisher")
	}
	publisher(progressFragment("dispatch-1", "ignored"))
}

func TestManager_DispatchCompletionObserver_NoopsForMissingSession(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{session: nil}
	manager := newTestManager(host)
	observer := manager.DispatchCompletionObserverFactory()("missing")
	observer("dispatch-1")
}
