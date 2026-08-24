package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestModelBackendEdgesFromEnvironmentRemainDisabledByDefault(t *testing.T) {
	t.Setenv(modelBackendEndpointEnvironment, "")

	edges := modelBackendEdgesFromEnvironment()
	if edges.ModelInvocationBackend != nil || edges.ModelAssetHTTPClient != nil ||
		edges.ModelHostProcessLauncher != nil || edges.ModelHostProtocolNegotiator != nil {
		t.Fatal("model backend fixture edges = enabled, want empty production composition")
	}
}

func TestEnvironmentModelBackendMapsGenericEmbeddingWire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var input environmentGenericRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if input.ModelName != "embed" || input.Operation != string(models.OperationEMBED) || len(input.Inputs) != 1 ||
			input.Inputs[0].Name != "text" {
			http.Error(writer, "unexpected generic request", http.StatusBadRequest)
			return
		}
		text, err := base64.StdEncoding.DecodeString(input.Inputs[0].ContentBase64)
		if err != nil || string(text) != "Find similar work" {
			http.Error(writer, "unexpected text input", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(environmentGenericResponse{Outputs: []environmentGenericOutput{{
			Name: "embedding", Modality: string(models.ModalityJSON), ContentType: "application/json",
			MediaType: "application/json", Content: `[0.1,0.2,0.3,0.4]`,
		}}})
	}))
	defer server.Close()

	backend := environmentModelBackend{client: server.Client(), endpoint: server.URL}
	content, artifacts, err := backend.invoke(context.Background(), models.InvokeModelRequest{
		Model: models.ModelReference{NameOrURI: "embed"}, Operation: models.OperationEMBED,
		Inputs: []models.InferenceInput{{
			Name: "text", Modality: models.ModalityText, ContentType: "text/plain",
			MediaType: "text/plain", Content: "Find similar work",
		}},
	})
	if err != nil {
		t.Fatalf("environment backend invoke: %v", err)
	}
	if len(artifacts) != 0 || len(content) != 1 || content[0].Name != "embedding" ||
		content[0].Modality != models.ModalityJSON || content[0].Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("environment backend result = %#v artifacts=%#v, want one embedding JSON output", content, artifacts)
	}
}

func TestEnvironmentBackendArtifactIsPinnedFixtureIdentity(t *testing.T) {
	selection, err := environmentBackendArtifact(context.Background(), serviceedges.ModelBackendArtifactSelectionRequest{
		Backend: "localai-llamacpp", ProtocolVersion: "localai-backend-v1",
	})
	if err != nil {
		t.Fatalf("environment backend artifact: %v", err)
	}
	if selection.Name != environmentEmbedBackendName || selection.Location != environmentEmbedBackendLocation ||
		selection.Bytes != environmentEmbedBackendBytes || selection.SHA256 != environmentEmbedBackendSHA256 {
		t.Fatalf("environment backend selection = %#v, want pinned EMBED fixture identity", selection)
	}
}
