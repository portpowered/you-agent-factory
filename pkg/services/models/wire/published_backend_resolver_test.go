package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

type backendPublicationDoer func(*http.Request) (*http.Response, error)

func (do backendPublicationDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func TestPublishedBackendResolverSelectsCurrentWindowsCUDAAndKeepsOfflineSelection(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	oldTag := publicationTag(t, manifest)
	manifest = bytes.ReplaceAll(manifest, []byte(strings.TrimPrefix(oldTag, "localai-backends-v1-")), []byte(strings.Repeat("b", 64)))
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	requests := 0
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return publicationResponse(nil), fmt.Errorf("unexpected publication request")
	}))
	if err != nil {
		t.Fatalf("construct published resolver: %v", err)
	}
	llama := publishedWindowsRequest("localai-llamacpp")
	assertPublishedResolution(t, resolver, llama, true, "cpu", "windows-amd64")
	assertPublicationRequests(t, requests, 0)
	assertPublishedResolution(t, resolver, llama, false, "cuda", "windows-amd64-cuda")
	assertPublicationRequests(t, requests, 2)
	current, err := resolver(context.Background(), llama, false)
	if err != nil || !strings.Contains(current.Location, tag) {
		t.Fatalf("current publication location = %q, %v, want release %s", current.Location, err, tag)
	}
	assertPublishedResolution(t, resolver, publishedWindowsRequest("localai-whisper"), false, "cpu", "windows-amd64")
	assertPublicationRequests(t, requests, 2)
	assertPublishedResolution(t, resolver, llama, true, "cuda", "windows-amd64-cuda")
	assertPublicationRequests(t, requests, 2)
}

func assertPublishedResolution(t *testing.T, resolver BackendArtifactResolver, request ResolvedHostConfiguration, offline bool, accelerator, target string) {
	t.Helper()
	selection, err := resolver(context.Background(), request, offline)
	if err != nil || selection.Accelerator != accelerator || !strings.Contains(selection.Name, target) {
		t.Fatalf("published selection = %#v, %v, want accelerator %s and target %s", selection, err, accelerator, target)
	}
}

func assertPublicationRequests(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("publication requests = %d, want %d", got, want)
	}
}

func TestPublishedBackendResolverFallsBackWhenPublicationUnavailable(t *testing.T) {
	t.Parallel()
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(*http.Request) (*http.Response, error) {
		return publicationResponse(nil), fmt.Errorf("publication unavailable")
	}))
	if err != nil {
		t.Fatalf("construct published resolver: %v", err)
	}
	selection, err := resolver(context.Background(), publishedWindowsRequest("localai-llamacpp"), false)
	if err != nil || selection.Accelerator != "cpu" {
		t.Fatalf("unavailable publication selection = %#v, %v, want CPU fallback", selection, err)
	}
}

func TestPublishedBackendResolverRejectsIncompleteRelease(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	tag := publicationTag(t, manifest)
	index := []byte(fmt.Sprintf(`[{"tag_name":%q,"assets":[{"name":"manifest.json"}]}]`, tag))
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		return publicationResponse(manifest), nil
	}))
	if err != nil {
		t.Fatalf("construct published resolver: %v", err)
	}
	assertPublishedResolution(t, resolver, publishedWindowsRequest("localai-llamacpp"), false, "cpu", "windows-amd64")
}

func TestFetchPublishedBackendManifestLive(t *testing.T) {
	if os.Getenv("YOU_LOCALAI_PUBLICATION_LIVE") != "1" {
		t.Skip("opt in to the published backend manifest check")
	}
	baseline, err := artifacts.DefaultManifest()
	if err != nil {
		t.Fatalf("decode baseline manifest: %v", err)
	}
	manifest, err := fetchPublishedBackendManifest(context.Background(), &http.Client{Timeout: 15 * time.Second}, baseline.ProtocolRevision())
	if err != nil {
		t.Fatalf("fetch current publication: %v", err)
	}
	selection, err := backendArtifactResolver(manifest)(context.Background(), publishedWindowsRequest("localai-llamacpp"), false)
	if err != nil || selection.Name == "" || selection.Location == "" || len(selection.SHA256) != 64 {
		t.Fatalf("current publication selection = %#v, %v", selection, err)
	}
}

func windowsCUDAPublicationFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "internal", "artifacts", "testdata", "windows-cuda-variant-manifest.json"))
	if err != nil {
		t.Fatalf("read CUDA publication fixture: %v", err)
	}
	return data
}

func publicationTag(t *testing.T, data []byte) string {
	t.Helper()
	var document struct {
		Publication struct {
			ReleaseTag string `json:"releaseTag"`
		} `json:"publication"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode publication tag: %v", err)
	}
	return document.Publication.ReleaseTag
}

func publicationIndex(t *testing.T, tag string, manifest []byte) []byte {
	t.Helper()
	var document struct {
		Artifacts []backendArchiveEntry `json:"artifacts"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatalf("decode publication archives: %v", err)
	}
	assets := []map[string]string{{"name": "manifest.json"}}
	for _, entry := range document.Artifacts {
		assets = append(assets, map[string]string{"name": entry.Artifact.Name})
	}
	index, err := json.Marshal([]map[string]any{{"tag_name": tag, "assets": assets}})
	if err != nil {
		t.Fatalf("encode publication index: %v", err)
	}
	return index
}

func publicationResponse(data []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data))}
}

func publishedWindowsRequest(backend string) modelseffects.ResolvedHostConfiguration {
	return modelseffects.ResolvedHostConfiguration{
		Backend: backend, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true},
	}
}
