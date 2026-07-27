package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

type registryBackedLiveRuntimeHost struct {
	liveRuntimeEffectHost
	registry  *sessionregistry.Registry
	openMu    sync.Mutex
	nextID    atomic.Int32
	openCalls atomic.Int32
}

// OpenLiveSessionForTarget mirrors production open semantics: each successful
// activation allocates a new session identity without logical-key dedupe.
func (h *registryBackedLiveRuntimeHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	h.openMu.Lock()
	defer h.openMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	h.openCalls.Add(1)
	time.Sleep(2 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sessionID := fmt.Sprintf("sess-gateway-open-%d", h.nextID.Add(1))
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Target: target.Ref,
	}
	h.registry.Upsert(session, h.registry.Count() == 0)
	h.sessionIDs = h.registry.IDs()
	return sessionID, nil
}

func (h *registryBackedLiveRuntimeHost) ListLiveSessionIDs() []string {
	return h.registry.IDs()
}

func (h *registryBackedLiveRuntimeHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	return h.registry.Get(sessionID)
}

func (h *registryBackedLiveRuntimeHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if session := h.registry.Get(sessionID); session != nil {
		return session, nil
	}
	return nil, factorysessions.ErrNotFound
}

func (h *registryBackedLiveRuntimeHost) StopLiveSession(sessionID string) error {
	h.stopCalls++
	h.registry.Remove(sessionID)
	h.sessionIDs = h.registry.IDs()
	return nil
}

func newRegistryBackedLiveRuntimeGateway(t *testing.T) (*factorysessionservice.Service, *registryBackedLiveRuntimeHost) {
	t.Helper()
	host := &registryBackedLiveRuntimeHost{
		registry: sessionregistry.New(),
		liveRuntimeEffectHost: liveRuntimeEffectHost{
			openTestHost: openTestHost{
				targets: []factorysessions.Target{{
					Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
					FactoryDir: "/tmp/factory",
					FolderPath: "/tmp",
				}},
			},
		},
	}
	return newLiveRuntimeCompositionGateway(t, host), host
}

func TestService_ConcurrentOpenThroughLiveRuntimeOwnerAllocatesDistinctActivations(t *testing.T) {
	t.Parallel()

	gateway, host := newRegistryBackedLiveRuntimeGateway(t)
	ctx := context.Background()

	const workers = 32
	ids := make([]string, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for index := range workers {
		go func(slot int) {
			defer wg.Done()
			result, err := gateway.OpenFactorySessionFromFolder(ctx, "/tmp", nil, false, false)
			if err != nil {
				t.Errorf("OpenFactorySessionFromFolder: %v", err)
				return
			}
			if result == nil {
				t.Error("OpenFactorySessionFromFolder returned nil result")
				return
			}
			ids[slot] = result.SessionID
		}(index)
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers)
	for index, id := range ids {
		if id == "" {
			t.Fatalf("open[%d] returned empty identity", index)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("open[%d] = %q, want distinct identity per activation", index, id)
		}
		seen[id] = struct{}{}
	}
	if host.registry.Count() != workers {
		t.Fatalf("registry count = %d, want %d distinct activations", host.registry.Count(), workers)
	}
	if host.openCalls.Load() != workers {
		t.Fatalf("activation calls = %d, want %d", host.openCalls.Load(), workers)
	}
}

func TestService_ConcurrentResolveAndCloseLeavesDeterminateRegistryState(t *testing.T) {
	t.Parallel()

	gateway, host := newRegistryBackedLiveRuntimeGateway(t)
	ctx := context.Background()
	opened, err := gateway.OpenFactorySessionFromFolder(ctx, "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	sessionID := opened.SessionID

	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if gateway.ResolveFactorySession(sessionID) != nil {
				_ = gateway.CloseFactorySession(ctx, sessionID)
				return
			}
			_, _ = gateway.GetFactorySession(ctx, sessionID)
		}()
	}
	wg.Wait()

	if gateway.ResolveFactorySession(sessionID) != nil {
		if host.registry.Count() != 1 {
			t.Fatalf("active registry count = %d, want 1", host.registry.Count())
		}
		return
	}
	if host.registry.Count() != 0 {
		t.Fatalf("inactive registry count = %d, want 0", host.registry.Count())
	}
	_, err = gateway.GetFactorySession(ctx, sessionID)
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("Get after concurrent close = %v, want ErrNotFound", err)
	}
}

type cancellableRegistryBackedHost struct {
	registryBackedLiveRuntimeHost
}

func (h *cancellableRegistryBackedHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	h.openMu.Lock()
	defer h.openMu.Unlock()
	time.Sleep(10 * time.Millisecond)
	return h.registryBackedLiveRuntimeHost.OpenLiveSessionForTarget(ctx, target)
}

func TestService_OpenFactorySessionCancellationDoesNotPublishActiveRegistryEntry(t *testing.T) {
	t.Parallel()

	host := &cancellableRegistryBackedHost{
		registryBackedLiveRuntimeHost: registryBackedLiveRuntimeHost{
			registry: sessionregistry.New(),
			liveRuntimeEffectHost: liveRuntimeEffectHost{
				openTestHost: openTestHost{
					targets: []factorysessions.Target{{
						Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
						FactoryDir: "/tmp/factory",
						FolderPath: "/tmp",
					}},
				},
			},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gateway.OpenFactorySessionFromFolder(ctx, "/tmp", nil, false, false)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenFactorySessionFromFolder error = %v, want context canceled", err)
	}
	if host.registry.Count() != 0 {
		t.Fatalf("registry count after cancelled open = %d, want 0", host.registry.Count())
	}
}
