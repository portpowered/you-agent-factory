package apiserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func newMockFactorySessionTestServer(f *testutil.MockFactory) *api.Server {
	logger, _ := zap.NewDevelopment()
	return api.NewServer(f, 8080, logger)
}

func stringPointerForFactorySessionTest(value string) *string {
	return &value
}

func TestFactorySessionsAPI_GetFactorySession(t *testing.T) {
	phase := "review"
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
			Target: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindNamed,
				Name: stringPointerForFactorySessionTest("beta"),
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

func TestFactorySessionsAPI_GetFactorySessionResult_OmitsRawCheckpointBody(t *testing.T) {
	hash := "sha256:checkpoint-body"
	size := int64(128)
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{
		Id:    "ckpt-1",
		Label: stringPointerForFactorySessionTest("after-plan"),
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
	}}
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionResult: factoryapi.FactorySessionResult{
			SessionId: "session-js",
			Status:    factoryapi.FactorySessionStatusIDLE,
			ResultArtifactRef: &factoryapi.FactoryArtifactRef{
				Id:         "artifact-ckpt-1",
				Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
				Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			},
			CheckpointRefs: &checkpoints,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-js/result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-js/result status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"rawBody", "storagePath", "vmState", "/tmp/checkpoints"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("session result leaked %q: %s", forbidden, body)
		}
	}
	var response factoryapi.FactorySessionResult
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&response); err != nil {
		t.Fatalf("decode session result: %v", err)
	}
	if response.ResultArtifactRef == nil || response.ResultArtifactRef.Visibility != factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT {
		t.Fatalf("result artifact ref = %#v", response.ResultArtifactRef)
	}
}

func TestFactorySessionsAPI_GetFactorySessionPartialResult_ReturnsCheckpointRefs(t *testing.T) {
	phase := "review"
	hash := "sha256:checkpoint-body"
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{
		Id: "ckpt-1",
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
		},
	}}
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionPartialResult: factoryapi.FactorySessionPartialResult{
			SessionId: "session-js",
			Phase:     phase,
			PartialResultArtifactRef: &factoryapi.FactoryArtifactRef{
				Id:         "artifact-ckpt-1",
				Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
				Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			},
			CheckpointRefs: &checkpoints,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-js/partial-result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-js/partial-result status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySessionPartialResult
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode partial result: %v", err)
	}
	if response.Phase != phase {
		t.Fatalf("phase = %q, want %q", response.Phase, phase)
	}
	if response.PartialResultArtifactRef == nil {
		t.Fatal("expected partial result artifact ref")
	}
}

func TestFactorySessionsAPI_GetFactorySessionResult_NotFoundForUnavailableSession(t *testing.T) {
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		GetFactorySessionResultErr: apisurface.ErrFactorySessionResultUnavailable,
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-petri/result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
