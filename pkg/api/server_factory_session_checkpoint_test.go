package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestFactorySessionsAPI_GetFactorySessionResult_OmitsRawCheckpointBody(t *testing.T) {
	hash := "sha256:checkpoint-body"
	size := int64(128)
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{
		Id:    "ckpt-1",
		Label: stringPointerForAPITest("after-plan"),
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
	}}
	srv := newTestServer(&testutil.MockFactory{
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
	srv := newTestServer(&testutil.MockFactory{
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
	srv := newTestServer(&testutil.MockFactory{
		GetFactorySessionResultErr: apisurface.ErrFactorySessionResultUnavailable,
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-petri/result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
