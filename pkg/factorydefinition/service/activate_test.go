package factorydefinition

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

type activateTrackingHost struct {
	stubDefinitionHost
	swappedName string
	sessionID   string
}

func (h *activateTrackingHost) RunSessionID() string { return "session-alpha" }

func (h *activateTrackingHost) SessionForActivation(string) *factorysessions.LiveSession {
	return &factorysessions.LiveSession{
		ID: "session-alpha",
		SessionState: factorysessions.SessionState{
			FactoryDir: h.persistRootDir,
			FolderPath: h.persistRootDir,
		},
	}
}

func (h *activateTrackingHost) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return h.persistRootDir, h.persistRootDir
}

func (h *activateTrackingHost) SwapPersistedNamedFactoryRuntime(
	_ context.Context,
	sessionID string,
	_ *factorysessions.LiveSession,
	_, _, _, name string,
) error {
	h.sessionID = sessionID
	h.swappedName = name
	return nil
}

func TestService_ActivateNamedFactory_SwapsPersistedNamedFactory(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if _, err := config.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	host := &activateTrackingHost{stubDefinitionHost: stubDefinitionHost{persistRootDir: rootDir}}
	if err := New(host).ActivateNamedFactory(context.Background(), "alpha"); err != nil {
		t.Fatalf("ActivateNamedFactory: %v", err)
	}
	if host.swappedName != "alpha" {
		t.Fatalf("swapped factory name = %q, want alpha", host.swappedName)
	}
	if host.sessionID != "session-alpha" {
		t.Fatalf("swap session id = %q, want session-alpha", host.sessionID)
	}
}

func TestService_ActivateNamedFactory_ReturnsResolveErrorForMissingFactory(t *testing.T) {
	t.Parallel()

	host := &activateTrackingHost{stubDefinitionHost: stubDefinitionHost{persistRootDir: t.TempDir()}}
	err := New(host).ActivateNamedFactory(context.Background(), "missing")
	if err == nil {
		t.Fatal("ActivateNamedFactory: expected error for missing named factory")
	}
	if !factoryconfig.IsNamedFactoryNotFound(err) {
		t.Fatalf("ActivateNamedFactory error = %v, want named factory not found", err)
	}
}
