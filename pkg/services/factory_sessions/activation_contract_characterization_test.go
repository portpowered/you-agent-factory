package factorysessions

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// peerDefinitionActivationFake exercises the published activation root slice
// through DefinitionActivationGateway. It compiles against only the Sessions
// root package and never imports factory_sessions/internal types.
type peerDefinitionActivationFake struct {
	holdingLock int32
	idle        bool
	sessionID   string
}

func newPeerDefinitionActivationFake(sessionID string, idle bool) *peerDefinitionActivationFake {
	return &peerDefinitionActivationFake{sessionID: sessionID, idle: idle}
}

var _ DefinitionActivationGateway = (*peerDefinitionActivationFake)(nil)

func (fake *peerDefinitionActivationFake) RunSessionID() string { return fake.sessionID }

func (fake *peerDefinitionActivationFake) SessionForActivation(sessionID string) *factorydefinitions.DefinitionSession {
	return &factorydefinitions.DefinitionSession{ID: sessionID, IsDefault: true}
}

func (fake *peerDefinitionActivationFake) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{ID: sessionID, IsDefault: true}, nil
}

func (fake *peerDefinitionActivationFake) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return "/persist"
}

func (fake *peerDefinitionActivationFake) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "/persist", "/persist/factory"
}

func (fake *peerDefinitionActivationFake) SaveNow() time.Time { return time.Unix(0, 0).UTC() }

func (fake *peerDefinitionActivationFake) WithActivationLock(fn func() error) error {
	if !atomic.CompareAndSwapInt32(&fake.holdingLock, 0, 1) {
		return errors.New("activation lock already held")
	}
	defer atomic.StoreInt32(&fake.holdingLock, 0)
	return fn()
}

func (fake *peerDefinitionActivationFake) RequireIdleRuntimeForSession(context.Context, string) error {
	if fake.idle {
		return nil
	}
	return factorydefinitions.ErrFactoryActivationRequiresIdle
}

func (fake *peerDefinitionActivationFake) RequireIdleBeforeNamedFactoryActivation(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
) error {
	if fake.idle {
		return nil
	}
	return factorydefinitions.ErrFactoryActivationRequiresIdle
}

func (fake *peerDefinitionActivationFake) ActivateSessionEditableFactory(
	context.Context,
	*factorydefinitions.DefinitionSession,
	string,
	string,
	string,
	string,
	string,
) error {
	return nil
}

func (fake *peerDefinitionActivationFake) SwapPersistedNamedFactoryRuntime(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
	string,
	string,
	string,
	string,
) error {
	return nil
}

func TestDefinitionActivationRootContract_SerializesLockAndIdleOutcomes(t *testing.T) {
	t.Parallel()

	busyGateway := newPeerDefinitionActivationFake("session-alpha", false)
	var gateway DefinitionActivationGateway = busyGateway

	if err := gateway.RequireIdleRuntimeForSession(context.Background(), "session-alpha"); !errors.Is(err, factorydefinitions.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("RequireIdleRuntimeForSession() error = %v, want %v", err, factorydefinitions.ErrFactoryActivationRequiresIdle)
	}

	idleGateway := newPeerDefinitionActivationFake("session-alpha", true)
	gateway = idleGateway
	if err := gateway.WithActivationLock(func() error {
		return gateway.WithActivationLock(func() error { return nil })
	}); err == nil {
		t.Fatal("nested WithActivationLock() error = nil, want serialized lock rejection")
	}
	if err := gateway.RequireIdleRuntimeForSession(context.Background(), "session-alpha"); err != nil {
		t.Fatalf("RequireIdleRuntimeForSession() error = %v, want idle acceptance", err)
	}
}
