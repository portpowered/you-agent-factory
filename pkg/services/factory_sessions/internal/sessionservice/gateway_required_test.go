package service_test

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

func TestService_StreamMethods_RequireGateway(t *testing.T) {
	t.Parallel()

	var gateway *factorysessionservice.Service

	if _, err := gateway.SubscribeSessionResponseStream("sess", "dispatch-1", 0); err == nil {
		t.Fatal("SubscribeSessionResponseStream = nil, want gateway required")
	}
	if _, err := gateway.SessionResponseStreamDispatchIDs("sess"); err == nil {
		t.Fatal("SessionResponseStreamDispatchIDs = nil, want gateway required")
	}
	if gateway.InferenceProgressPublisherFactory(nil) != nil {
		t.Fatal("InferenceProgressPublisherFactory = non-nil, want nil without gateway")
	}
	if gateway.DispatchCompletionObserverFactory() != nil {
		t.Fatal("DispatchCompletionObserverFactory = non-nil, want nil without gateway")
	}

	gateway.CloseSessionResponseStreams(&factorysessions.LiveSession{ID: "sess"})
	if gateway.JavaScriptCheckpointStore(&factorysessions.LiveSession{ID: "sess"}) != nil {
		t.Fatal("JavaScriptCheckpointStore = non-nil, want nil without gateway")
	}
}

func TestServiceConstructionRequiresResponseStreamRegistry(t *testing.T) {
	t.Parallel()

	if gateway := factorysessionservice.New(&openTestHost{}, nil); gateway != nil {
		t.Fatalf("New without response-stream registry = %#v, want nil", gateway)
	}
}
