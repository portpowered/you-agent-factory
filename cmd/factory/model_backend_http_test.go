package main

import (
	"context"
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
		edges.ModelEmbeddingBackend != nil || edges.ModelHostProcessLauncher != nil ||
		edges.ModelHostProtocolNegotiator != nil {
		t.Fatal("model backend fixture edges = enabled, want empty production composition")
	}
}

func TestEnvironmentModelBackendMapsEmbeddingWire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		if request.URL.Path != "/embed" {
			http.Error(writer, "unexpected embedding path", http.StatusBadRequest)
			return
		}
		var input environmentEmbeddingRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if input.Text != "Find similar work" || input.Parameters["normalize"] != true {
			http.Error(writer, "unexpected embedding request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(environmentEmbeddingResponse{
			Embeddings: []float64{0.1, 0.2, 0.3, 0.4},
		})
	}))
	defer server.Close()

	backend := environmentModelBackend{client: server.Client(), endpoint: server.URL}
	response, err := backend.embed(context.Background(), models.EmbeddingBackendRequest{
		Text:       "Find similar work",
		Parameters: map[string]any{"normalize": true},
	})
	if err != nil {
		t.Fatalf("environment backend embed: %v", err)
	}
	if len(response.Embeddings) != 4 || response.Embeddings[0] != 0.1 || response.Embeddings[3] != 0.4 {
		t.Fatalf("environment backend response = %#v, want one embedding vector", response)
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
