package controlplane_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
)

type resultReadTestHost struct {
	readTestHost
	checkpoint *factorysessions.JavaScriptCheckpointStore
	factoryCfg *interfaces.FactoryConfig
}

func (h *resultReadTestHost) JavaScriptCheckpointStore(_ *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.checkpoint == nil {
		h.checkpoint = factorysessions.NewJavaScriptCheckpointStore()
	}
	return h.checkpoint
}

func (h *resultReadTestHost) BuildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{
		Session:    session,
		FactoryCfg: h.factoryCfg,
	}, nil
}

func TestGetLiveFactorySessionResult_ReturnsJavaScriptProjection(t *testing.T) {
	t.Parallel()

	session := &factorysessions.LiveSession{ID: "sess-js"}
	host := &resultReadTestHost{
		readTestHost: readTestHost{
			sessions: map[string]*factorysessions.LiveSession{"sess-js": session},
		},
		factoryCfg: &interfaces.FactoryConfig{
			Name: "workflow",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
	}

	result, err := controlplane.GetLiveFactorySessionResult(context.Background(), host, "sess-js")
	if err != nil {
		t.Fatalf("GetLiveFactorySessionResult: %v", err)
	}
	if result.SessionID != "sess-js" {
		t.Fatalf("session id = %q, want sess-js", result.SessionID)
	}
}

func TestGetLiveFactorySessionPartialResult_ReturnsJavaScriptProjection(t *testing.T) {
	t.Parallel()

	session := &factorysessions.LiveSession{ID: "sess-js-partial"}
	host := &resultReadTestHost{
		readTestHost: readTestHost{
			sessions: map[string]*factorysessions.LiveSession{"sess-js-partial": session},
		},
		factoryCfg: &interfaces.FactoryConfig{
			Name: "workflow",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
	}

	result, err := controlplane.GetLiveFactorySessionPartialResult(context.Background(), host, "sess-js-partial")
	if err != nil {
		t.Fatalf("GetLiveFactorySessionPartialResult: %v", err)
	}
	if result.SessionID != "sess-js-partial" {
		t.Fatalf("session id = %q, want sess-js-partial", result.SessionID)
	}
}
