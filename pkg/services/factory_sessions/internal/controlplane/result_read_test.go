package controlplane_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
)

type resultReadTestHost struct {
	readTestHost
	checkpoint factoryruntime.JavaScriptCheckpointStore
	factoryCfg *interfaces.FactoryConfig
}

type resultProjectionRole struct{}

func (resultProjectionRole) ProjectSessionResults(
	input factoryruntime.SessionResultInput,
) factoryruntime.SessionResultProjection {
	return factoryruntime.SessionResultProjection{Live: factoryruntime.LiveSessionResult{
		SessionID: input.SessionID,
		Status:    input.Status,
	}}
}

func (h *resultReadTestHost) JavaScriptCheckpointStore(_ *factorysessions.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	if h.checkpoint == nil {
		h.checkpoint = resultReadCheckpointStore{}
	}
	return h.checkpoint
}

type resultReadCheckpointStore struct{}

func (resultReadCheckpointStore) Put(interfaces.JavaScriptCheckpointRecord) {}
func (resultReadCheckpointStore) List() []interfaces.JavaScriptCheckpointRecord {
	return nil
}
func (resultReadCheckpointStore) Get(string) (interfaces.JavaScriptCheckpointRecord, bool) {
	return interfaces.JavaScriptCheckpointRecord{}, false
}

var _ factoryruntime.JavaScriptCheckpointStore = resultReadCheckpointStore{}

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

	result, err := controlplane.GetLiveFactorySessionResult(
		context.Background(), host, resultProjectionRole{}, "sess-js",
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionResult: %v", err)
	}
	if result.SessionID != "sess-js" {
		t.Fatalf("session id = %q, want sess-js", result.SessionID)
	}
}

func TestGetLiveFactorySessionResult_RequiresInjectedProjection(t *testing.T) {
	t.Parallel()

	host := &resultReadTestHost{
		readTestHost: readTestHost{sessions: map[string]*factorysessions.LiveSession{
			"sess-js": {ID: "sess-js"},
		}},
		factoryCfg: &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
		}},
	}

	_, err := controlplane.GetLiveFactorySessionResult(context.Background(), host, nil, "sess-js")
	if err == nil || err.Error() != "Factory Runtime session result projection is required" {
		t.Fatalf("GetLiveFactorySessionResult() error = %v, want required injected projection", err)
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
