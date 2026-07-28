package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestListFactorySessionArtifacts_RejectsInvalidScopeBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			invoked = true
			return recordings.RecordingStatusResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListFactorySessionArtifacts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/%20/artifacts", nil),
		"   ",
	)

	if invoked {
		t.Fatal("invalid artifact read scope must be rejected before Recordings root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_EncodesFakeRootArtifactSummaries(t *testing.T) {
	t.Parallel()

	capturedAt := time.Unix(1_700_000_000, 0).UTC()
	worldPayload, err := json.Marshal(interfaces.FactoryWorldState{
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:          "artifact-1",
			Kind:        "FINAL_RESULT",
			Visibility:  "PUBLIC",
			Label:       "result",
			ContentHash: "sha256:abc",
			SizeBytes:   12,
			CapturedAt:  capturedAt,
		}},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != recordings.RecordingID("session-1") {
				t.Fatalf("RecordingStatusRequest = %#v, want session-1", request)
			}
			return recordings.RecordingStatusResult{
				Status: recordings.RecordingStatusFacts{
					RecordingID: request.RecordingID,
					Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-1"},
				},
			}, nil
		},
		buildPortableArtifact: func(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
			return recordings.BuildPortableArtifactResult{
				Artifact: recordings.PortableArtifact{
					Summary: recordings.PortableArtifactSummary{
						Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
					},
					Events: []recordings.CanonicalEvent{{
						Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-1"},
						FactoryTick: 3,
						Cursor: recordings.CanonicalEventCursor{
							StreamGenerationID: "generation-1",
							Sequence:           0,
						},
					}},
				},
			}, nil
		},
		reconstructWorldState: func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			if request.Scope.FactorySessionID != "session-1" || len(request.Events) != 1 {
				t.Fatalf("ReconstructWorldStateRequest = %#v, want one scoped event", request)
			}
			return recordings.ReconstructWorldStateResult{
				WorldState: recordings.WorldStateView{Payload: string(worldPayload)},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListFactorySessionArtifacts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil),
		"session-1",
	)

	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"id":"artifact-1"`) ||
		!strings.Contains(recorder.Body.String(), `"sessionId":"session-1"`) {
		t.Fatalf("response = %d %s, want encoded artifact list", recorder.Code, recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_MapsMissingTargetToNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListFactorySessionArtifacts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil),
		"session-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want typed not found", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "pkg/services/recordings") {
		t.Fatalf("response leaked internal package path: %s", recorder.Body.String())
	}
}

func TestListFactorySessionArtifacts_MapsValidationErrorToBadRequest(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, recordings.ErrInvalidProjectionInput
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListFactorySessionArtifacts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil),
		"session-1",
	)

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadRequest ||
		!strings.Contains(body, `"code":"BAD_REQUEST"`) ||
		!strings.Contains(body, `"family":"BAD_REQUEST"`) ||
		!strings.Contains(body, `"message":"invalid artifact read request"`) {
		t.Fatalf("response = %d %s, want typed bad request ErrorResponse", recorder.Code, body)
	}
}

func TestListFactorySessionArtifacts_SanitizesUnmappedRootFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, errors.New("pkg/services/recordings/internal/service: boom")
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListFactorySessionArtifacts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts", nil),
		"session-1",
	)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"message":"failed to read factory session artifacts"`) {
		t.Fatalf("response = %d %s, want sanitized internal ErrorResponse", recorder.Code, body)
	}
	if strings.Contains(body, "pkg/services/recordings") || strings.Contains(body, "boom") {
		t.Fatalf("response leaked internal failure details: %s", body)
	}
}

func TestGetFactorySessionArtifact_RejectsInvalidArtifactIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			invoked = true
			return recordings.RecordingStatusResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetFactorySessionArtifact(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts/%20", nil),
		"session-1",
		"   ",
	)

	if invoked {
		t.Fatal("invalid artifact id must be rejected before Recordings root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactorySessionArtifact_EncodesFakeRootArtifactDetail(t *testing.T) {
	t.Parallel()

	worldPayload, err := json.Marshal(interfaces.FactoryWorldState{
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-1",
			Kind:       "LOG",
			Visibility: "PUBLIC",
			Label:      "log",
		}},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	adapter := NewAdapter(artifactReadRootFake(string(worldPayload)))
	recorder := httptest.NewRecorder()

	adapter.GetFactorySessionArtifact(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts/artifact-1", nil),
		"session-1",
		"artifact-1",
	)

	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"id":"artifact-1"`) ||
		!strings.Contains(recorder.Body.String(), `"kind":"LOG"`) {
		t.Fatalf("response = %d %s, want encoded artifact detail", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactorySessionArtifact_MapsMissingArtifactToNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(artifactReadRootFake(`{"artifacts":[]}`))
	recorder := httptest.NewRecorder()

	adapter.GetFactorySessionArtifact(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/artifacts/missing", nil),
		"session-1",
		"missing",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want typed not found", recorder.Code, recorder.Body.String())
	}
}

func artifactReadRootFake(worldPayload string) *rootFake {
	return &rootFake{
		queryRecordingStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{
				Status: recordings.RecordingStatusFacts{
					RecordingID: request.RecordingID,
					Scope:       recordings.CanonicalEventScope{FactorySessionID: string(request.RecordingID)},
				},
			}, nil
		},
		buildPortableArtifact: func(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
			return recordings.BuildPortableArtifactResult{
				Artifact: recordings.PortableArtifact{
					Summary: recordings.PortableArtifactSummary{
						Scope: recordings.CanonicalEventScope{FactorySessionID: string(request.RecordingID)},
					},
					Events: []recordings.CanonicalEvent{{
						Scope: recordings.CanonicalEventScope{FactorySessionID: string(request.RecordingID)},
					}},
				},
			}, nil
		},
		reconstructWorldState: func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			return recordings.ReconstructWorldStateResult{
				WorldState: recordings.WorldStateView{Payload: worldPayload},
			}, nil
		},
	}
}
