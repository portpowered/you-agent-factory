package http

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestArtifactListRequestFromAPI_MapsSessionScope(t *testing.T) {
	t.Parallel()

	request, err := ArtifactListRequestFromAPI(ArtifactListInput{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("ArtifactListRequestFromAPI: %v", err)
	}
	if request.RecordingID != recordings.RecordingID("session-1") {
		t.Fatalf("request = %#v, want recording id session-1", request)
	}
}

func TestArtifactListRequestFromAPI_RejectsEmptySessionBeforeRoot(t *testing.T) {
	t.Parallel()

	if _, err := ArtifactListRequestFromAPI(ArtifactListInput{SessionID: "   "}); err == nil {
		t.Fatal("ArtifactListRequestFromAPI must reject empty session id before root invocation")
	}
}

func TestArtifactGetRequestFromAPI_MapsPortableArtifactReadRequest(t *testing.T) {
	t.Parallel()

	request, err := ArtifactGetRequestFromAPI(ArtifactGetInput{
		SessionID:  "session-1",
		ArtifactID: "artifact-1",
	})
	if err != nil {
		t.Fatalf("ArtifactGetRequestFromAPI: %v", err)
	}
	if request.RecordingID != recordings.RecordingID("session-1") ||
		request.Reference != recordings.RecordingArtifactReference("artifact-1") {
		t.Fatalf("request = %#v, want portable artifact read for session-1/artifact-1", request)
	}
}

func TestArtifactGetRequestFromAPI_RejectsMalformedInputsBeforeRoot(t *testing.T) {
	t.Parallel()

	if _, err := ArtifactGetRequestFromAPI(ArtifactGetInput{
		SessionID:  "session-1",
		ArtifactID: "   ",
	}); err == nil {
		t.Fatal("ArtifactGetRequestFromAPI must reject empty artifact id before root invocation")
	}
}

func TestArtifactStatesFromWorldStatePayload_ExtractsDetachedArtifactProjections(t *testing.T) {
	t.Parallel()

	capturedAt := time.Unix(1_700_000_000, 0).UTC()
	payload, err := json.Marshal(interfaces.FactoryWorldState{
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:          "artifact-1",
			Kind:        "FINAL_RESULT",
			Visibility:  "PUBLIC",
			Label:       "result",
			ContentHash: "sha256:abc",
			SizeBytes:   12,
			CapturedAt:  capturedAt,
		}},
		JavaScriptRuntime: &interfaces.FactorySessionJavaScriptRuntimeState{
			Artifacts: []interfaces.FactorySessionArtifactState{{
				ID:         "artifact-2",
				Kind:       "LOG",
				Visibility: "PUBLIC",
			}},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	artifacts, err := ArtifactStatesFromWorldStatePayload(string(payload))
	if err != nil {
		t.Fatalf("ArtifactStatesFromWorldStatePayload: %v", err)
	}
	if len(artifacts) != 2 || artifacts[0].ID != "artifact-1" || artifacts[1].ID != "artifact-2" {
		t.Fatalf("artifacts = %#v, want artifact-1 and artifact-2", artifacts)
	}
}

func TestArtifactListResponseToAPI_EncodesDetachedArtifactSummaries(t *testing.T) {
	t.Parallel()

	capturedAt := time.Unix(1_700_000_000, 0).UTC()
	response := ArtifactListResponseToAPI("session-1", []interfaces.FactorySessionArtifactState{{
		ID:          "artifact-1",
		Kind:        "FINAL_RESULT",
		Visibility:  "PUBLIC",
		Label:       "result",
		ContentHash: "sha256:abc",
		SizeBytes:   12,
		CapturedAt:  capturedAt,
	}})
	if response.SessionId != "session-1" ||
		len(response.Artifacts) != 1 ||
		response.Artifacts[0].Id != "artifact-1" ||
		response.Artifacts[0].Kind != factoryapi.FactoryArtifactKindFINALRESULT {
		t.Fatalf("response = %#v, want encoded artifact summary", response)
	}
}

func TestArtifactDetailResponseToAPI_EncodesDetachedArtifactDetail(t *testing.T) {
	t.Parallel()

	dispatchID := "dispatch-1"
	response := ArtifactDetailResponseToAPI("session-1", interfaces.FactorySessionArtifactState{
		ID:         "artifact-1",
		Kind:       "LOG",
		Visibility: "PUBLIC",
		Label:      "log",
		CaptureMetadata: map[string]string{
			"sourceDispatchId": dispatchID,
		},
	})
	if response.SessionId != "session-1" ||
		response.Id != "artifact-1" ||
		response.Kind != factoryapi.FactoryArtifactKindLOG ||
		response.DispatchId == nil ||
		*response.DispatchId != dispatchID {
		t.Fatalf("response = %#v, want encoded artifact detail", response)
	}
}
