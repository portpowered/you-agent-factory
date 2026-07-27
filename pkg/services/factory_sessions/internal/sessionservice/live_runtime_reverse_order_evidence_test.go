package service_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

// Reverse-order partial-failure cleanup for scope/instance binding remains owned by
// runtimeopening.runtimeOpeningCleanup and is proven in
// pkg/services/factory_sessions/internal/runtimeopening/models_bind_test.go by
// TestRuntimeOpeningCleanupClosesModelsScopeAfterLaterResourceOnFailure and
// TestOpenRuntimeClosesModelsScopeExactlyOnceAfterLaterStepFails.
// This test proves the Sessions gateway/live_runtime packaging surface does not
// publish a live registry entry when the injected activation effect fails.
func TestService_OpenActivationFailureDoesNotPublishLiveRegistryEntry(t *testing.T) {
	t.Parallel()

	registry := sessionregistry.New()
	host := &failingOpenLiveRuntimeHost{
		registry: registry,
		liveRuntimeEffectHost: liveRuntimeEffectHost{
			openTestHost: openTestHost{
				targets: []factorysessions.Target{{
					Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
					FactoryDir: "/tmp/factory",
					FolderPath: "/tmp",
				}},
			},
		},
		openErr: errors.New("runtime opening failed after partial setup"),
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	_, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, false, false)
	if err == nil || !errors.Is(err, host.openErr) {
		t.Fatalf("OpenFactorySessionFromFolder error = %v, want %v", err, host.openErr)
	}
	if registry.Count() != 0 {
		t.Fatalf("registry count after failed open = %d, want 0", registry.Count())
	}
}

type failingOpenLiveRuntimeHost struct {
	liveRuntimeEffectHost
	registry *sessionregistry.Registry
	openErr  error
}

func (h *failingOpenLiveRuntimeHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", h.openErr
}

func (h *failingOpenLiveRuntimeHost) ListLiveSessionIDs() []string {
	return h.registry.IDs()
}

func (h *failingOpenLiveRuntimeHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	return h.registry.Get(sessionID)
}

func (h *failingOpenLiveRuntimeHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if session := h.registry.Get(sessionID); session != nil {
		return session, nil
	}
	return nil, factorysessions.ErrNotFound
}
