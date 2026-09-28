package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

type closeRegistryOwnerFake struct {
	registry sessionregistry.Service
	order    *[]string
	failID   string
	failErr  error
}

func (owner *closeRegistryOwnerFake) BuildSessionProjectionContext(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return factorysessions.ProjectionContext{FactorySessionID: session.ID}, nil
}

func (owner *closeRegistryOwnerFake) PrepareOwnedSessionClose(_ context.Context, sessionID string) error {
	*owner.order = append(*owner.order, sessionID)
	if sessionID == owner.failID {
		return owner.failErr
	}
	return nil
}

func (owner *closeRegistryOwnerFake) RetireOwnedSession(sessionID string) error {
	owner.registry.Remove(sessionID)
	return nil
}

func TestAssemblyCloseDrainsCanonicalRegistryAndRetainsFailedSession(t *testing.T) {
	state := newWorkResolverSessionState()
	registry := state.Registry()
	var order []string
	failure := errors.New("close first session")
	owner := &closeRegistryOwnerFake{registry: registry, order: &order, failID: "session-first", failErr: failure}
	for _, id := range []string{"session-first", "session-second", "session-third"} {
		registry.Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Owner: owner}}, false)
	}
	assembly := &Assembly{state: state, registry: registry}
	if err := assembly.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Close error = %v, want %v", err, failure)
	}
	if !reflect.DeepEqual(order, []string{"session-third", "session-second", "session-first"}) {
		t.Fatalf("shutdown order = %v", order)
	}
	if registry.Get("session-first") == nil || registry.Get("session-second") != nil || registry.Get("session-third") != nil {
		t.Fatalf("shutdown lost retryable failure or retained successful sessions: %v", registry.IDs())
	}
	owner.failID = ""
	if err := assembly.Close(context.Background()); err != nil || registry.Count() != 0 {
		t.Fatalf("retry close error = %v, remaining sessions = %v", err, registry.IDs())
	}
}
