package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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
	registry   *sessionregistry.Registry
	openMu     sync.Mutex
	openCalls  atomic.Int32
}

func legacyTargetKey(target factorysessions.Target) string {
	folderPath := filepath.Clean(strings.TrimSpace(target.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	folderPath = filepath.ToSlash(folderPath)
	targetKind := strings.TrimSpace(string(target.Ref.Kind))
	targetName := strings.TrimSpace(target.Ref.Name)
	if targetKind == "" {
		targetKind = string(factorysessions.TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}

func (h *registryBackedLiveRuntimeHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	h.openMu.Lock()
	defer h.openMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if existing := h.registry.FindByLogicalSessionKeyID(legacyTargetKey(target)); existing != nil {
		h.registry.Select(existing.ID)
		if h.sessions == nil {
			h.sessions = make(map[string]*livesession.LiveSession)
		}
		h.sessions[existing.ID] = existing
		return existing.ID, nil
	}
	h.openCalls.Add(1)
	time.Sleep(2 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	const sessionID = "sess-gateway-concurrent"
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Target: target.Ref,
	}
	h.registry.Upsert(session, true)
	if h.sessions == nil {
		h.sessions = make(map[string]*livesession.LiveSession)
	}
	h.sessions[sessionID] = session
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
	delete(h.sessions, sessionID)
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

func TestService_ConcurrentOpenThroughLiveRuntimeOwnerConvergesOnOneIdentity(t *testing.T) {
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

	for index, id := range ids {
		if id != "sess-gateway-concurrent" {
			t.Fatalf("open[%d] = %q, want sess-gateway-concurrent", index, id)
		}
	}
	if host.registry.Count() != 1 {
		t.Fatalf("registry count = %d, want 1", host.registry.Count())
	}
	if host.openCalls.Load() != 1 {
		t.Fatalf("activation calls = %d, want 1", host.openCalls.Load())
	}
}

func TestService_ConcurrentResolveAndCloseLeavesDeterminateRegistryState(t *testing.T) {
	t.Parallel()

	gateway, host := newRegistryBackedLiveRuntimeGateway(t)
	ctx := context.Background()
	if _, err := gateway.OpenFactorySessionFromFolder(ctx, "/tmp", nil, false, false); err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}

	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if gateway.ResolveFactorySession("sess-gateway-concurrent") != nil {
				_ = gateway.CloseFactorySession(ctx, "sess-gateway-concurrent")
				return
			}
			_, _ = gateway.GetFactorySession(ctx, "sess-gateway-concurrent")
		}()
	}
	wg.Wait()

	if gateway.ResolveFactorySession("sess-gateway-concurrent") != nil {
		if host.registry.Count() != 1 {
			t.Fatalf("active registry count = %d, want 1", host.registry.Count())
		}
		return
	}
	if host.registry.Count() != 0 {
		t.Fatalf("inactive registry count = %d, want 0", host.registry.Count())
	}
	_, err := gateway.GetFactorySession(ctx, "sess-gateway-concurrent")
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
	if gateway.ResolveFactorySession("sess-gateway-concurrent") != nil {
		t.Fatal("cancelled open left an active registry identity")
	}
}
