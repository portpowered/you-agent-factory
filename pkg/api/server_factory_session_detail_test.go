package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestFactorySessionsAPI_GetFactorySession(t *testing.T) {
	phase := "review"
	srv := newTestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
			Target: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindNamed,
				Name: stringPointerForAPITest("beta"),
			},
			Runtime: factoryapi.FactorySessionRuntime{
				OrchestratorKind: factoryapi.JAVASCRIPT,
				Status:           factoryapi.FactorySessionStatusIDLE,
				Progress: factoryapi.FactorySessionProgress{
					FactoryState:  "UNKNOWN",
					Categories:    factoryapi.StatusCategories{},
					InFlightCount: 0,
					TotalTokens:   0,
				},
				Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
				Lifecycle: factoryapi.FactorySessionLifecycle{
					StartedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
				},
				Javascript: &factoryapi.FactorySessionJavaScriptProjection{
					Phase:               &phase,
					Phases:              []string{"plan", "review"},
					ScriptStatus:        factoryapi.FactorySessionJavaScriptScriptStatusIDLE,
					ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-beta status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySession
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode factory session response: %v", err)
	}
	if response.Runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestrator kind = %q, want JAVASCRIPT", response.Runtime.OrchestratorKind)
	}
	if response.Runtime.Javascript == nil || response.Runtime.Javascript.Phase == nil || *response.Runtime.Javascript.Phase != "review" {
		t.Fatalf("javascript projection = %#v", response.Runtime.Javascript)
	}
}
