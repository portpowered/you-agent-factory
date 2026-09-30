package factorysessionexecution

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestDurableLiveChangeRejectsAnotherProjectsRuntime(t *testing.T) {
	const sessionID = "dur-sess-project-one"
	service := &JavaScriptRuntimeService{sessions: map[string]*runtimeSessionState{
		sessionID: {session: SessionReadResult{SessionID: sessionID}, projectRoot: "/project-one"},
	}}
	_, err := service.ApplyLiveChangeWithRuntime(context.Background(), sessionID, factorysessions.LiveChangeRequest{}, nil, "/project-two")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("cross-project live change error = %v, want session not found", err)
	}
}
