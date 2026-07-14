package stream_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/stream"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestManager_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var manager *stream.Manager
	session := &factorysessions.LiveSession{ID: "sess-nil"}

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
	if manager.JavaScriptCheckpointStore(session) != nil {
		t.Fatal("JavaScriptCheckpointStore = non-nil, want nil")
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
	manager := stream.NewManager(host)

	_, err := manager.Subscribe("missing", "dispatch-1", 0)
	if err == nil {
		t.Fatal("Subscribe = nil, want missing session")
	}
}

func TestManager_DispatchIDs_ReturnsNilForMissingStreams(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-no-streams"},
	}
	host.streams = nil
	manager := stream.NewManager(host)

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
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("missing")
	if publisher == nil {
		t.Fatal("publisher = nil, want no-op publisher")
	}
	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "ignored"))
}

func TestManager_DispatchCompletionObserver_NoopsForMissingSession(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{session: nil}
	manager := stream.NewManager(host)
	observer := manager.DispatchCompletionObserverFactory()("missing")
	observer("dispatch-1")
}
