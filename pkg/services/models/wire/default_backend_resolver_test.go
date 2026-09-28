package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestBackendArtifactResolverPrefersAvailableWindowsCUDAArchive(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "internal", "artifacts", "testdata", "windows-cuda-variant-manifest.json"))
	if err != nil {
		t.Fatalf("read CUDA manifest fixture: %v", err)
	}
	manifest, err := artifacts.Decode(data)
	if err != nil {
		t.Fatalf("decode CUDA manifest fixture: %v", err)
	}
	resolve := backendArtifactResolver(manifest)
	platform := models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true}
	for _, testCase := range []struct {
		backend, accelerator, target string
	}{
		{"localai-llamacpp", "cuda", "windows-amd64-cuda"},
		{"localai-whisper", "cpu", "windows-amd64"},
		{"localai-vibevoice", "cpu", "windows-amd64"},
	} {
		selection, err := resolve(context.Background(), ResolvedHostConfiguration{
			Backend: testCase.backend, Platform: platform,
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		}, false)
		if err != nil {
			t.Fatalf("resolve %s: %v", testCase.backend, err)
		}
		if selection.Accelerator != testCase.accelerator || selection.Name == "" ||
			!strings.Contains(selection.Name, testCase.target) {
			t.Fatalf("selection for %s = %#v, want %s", testCase.backend, selection, testCase.target)
		}
	}
}

func TestBackendArtifactResolverPrefersAvailableLinuxCUDAArchive(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "internal", "artifacts", "testdata", "windows-cuda-variant-manifest.json"))
	if err != nil {
		t.Fatalf("read CUDA manifest fixture: %v", err)
	}
	marker := []byte(`"id": "localai-llamacpp/windows-amd64-cuda"`)
	start := bytes.Index(data, marker)
	if start < 0 {
		t.Fatal("CUDA archive missing from manifest fixture")
	}
	// Move only the fixture's CUDA archive to Linux, retaining its Linux CPU
	// archive so the resolver has two valid choices for this backend.
	cudaArchive := bytes.ReplaceAll(data[start:], []byte("windows-amd64-cuda"), []byte("linux-amd64-cuda"))
	cudaArchive = bytes.ReplaceAll(cudaArchive, []byte(`"operatingSystem": "windows"`), []byte(`"operatingSystem": "linux"`))
	cudaArchive = bytes.ReplaceAll(cudaArchive, []byte(".zip"), []byte(".tar.gz"))
	data = append(data[:start:start], cudaArchive...)
	manifest, err := artifacts.Decode(data)
	if err != nil {
		t.Fatalf("decode Linux CUDA manifest fixture: %v", err)
	}
	resolve := backendArtifactResolver(manifest)
	selection, err := resolve(context.Background(), ResolvedHostConfiguration{
		Backend: "localai-llamacpp",
		Platform: models.AssetHostPlatform{
			OperatingSystem: "linux", Architecture: "amd64", CUDAAvailable: true,
		},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}, false)
	if err != nil {
		t.Fatalf("resolve Linux CUDA archive: %v", err)
	}
	if selection.Accelerator != "cuda" || !strings.Contains(selection.Name, "linux-amd64-cuda") {
		t.Fatalf("selection = %#v, want Linux CUDA archive", selection)
	}
}

func TestDefaultBackendArtifactResolverFallsBackToPublishedWindowsCPU(t *testing.T) {
	t.Parallel()
	resolve, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("construct default resolver: %v", err)
	}
	selection, err := resolve(context.Background(), ResolvedHostConfiguration{
		Backend: "localai-llamacpp",
		Platform: models.AssetHostPlatform{
			OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true,
		},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}, false)
	if err != nil {
		t.Fatalf("resolve Windows backend: %v", err)
	}
	if selection.Accelerator != "cpu" || strings.Contains(selection.Name, "windows-amd64-cuda") {
		t.Fatalf("published archive selection = %#v, want CPU fallback", selection)
	}
}

func TestDefaultBackendArtifactResolverFallsBackToPublishedLinuxCPU(t *testing.T) {
	t.Parallel()
	resolve, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("construct default resolver: %v", err)
	}
	selection, err := resolve(context.Background(), ResolvedHostConfiguration{
		Backend: "localai-llamacpp",
		Platform: models.AssetHostPlatform{
			OperatingSystem: "linux", Architecture: "amd64", CUDAAvailable: true,
		},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}, false)
	if err != nil {
		t.Fatalf("resolve Linux backend: %v", err)
	}
	if selection.Accelerator != "cpu" || strings.Contains(selection.Name, "linux-amd64-cuda") {
		t.Fatalf("published archive selection = %#v, want CPU fallback", selection)
	}
}

func TestNewDefaultBackendArtifactResolverSelectsPinnedMatrix(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	for _, test := range []struct {
		name     string
		backend  string
		platform models.AssetHostPlatform
	}{
		{name: "llamacpp linux", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "llamacpp mac", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "darwin", Architecture: "arm64"}},
		{name: "llamacpp windows", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"}},
		{name: "whisper linux", backend: "localai-whisper", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "vibevoice linux", backend: "localai-vibevoice", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection, err := resolver(context.Background(), ResolvedHostConfiguration{
				Backend: test.backend, Platform: test.platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			}, false)
			if err != nil {
				t.Fatalf("resolve %s: %v", test.backend, err)
			}
			if selection.Name == "" || selection.Location == "" || selection.Bytes <= 0 || len(selection.SHA256) != 64 {
				t.Fatalf("selection = %#v, want detached pinned archive facts", selection)
			}
		})
	}
}

func TestNewDefaultBackendArtifactResolverPreservesAcceleratorRequest(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	for _, operatingSystem := range []string{"windows", "linux"} {
		t.Run(operatingSystem, func(t *testing.T) {
			platform := models.AssetHostPlatform{
				OperatingSystem: operatingSystem, Architecture: "amd64", Accelerator: "cuda",
			}
			request := ResolvedHostConfiguration{
				Backend: "localai-llamacpp", Platform: platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			}
			_, err := resolver(context.Background(), request, false)
			if !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
				t.Fatalf("resolve explicit CUDA error = %v, want ErrIncompatibleAccelerator", err)
			}
			var failure *artifacts.Failure
			if !errors.As(err, &failure) || failure.Detail != "the matching artifact does not declare this accelerator" {
				t.Fatalf("resolve explicit CUDA error = %v, want selection against the CPU-only manifest", err)
			}

			request.Platform.Accelerator = ""
			selection, err := resolver(context.Background(), request, false)
			if err != nil {
				t.Fatalf("resolve omitted accelerator: %v", err)
			}
			request.Platform.Accelerator = "cpu"
			cpuSelection, err := resolver(context.Background(), request, false)
			if err != nil {
				t.Fatalf("resolve explicit CPU: %v", err)
			}
			if selection != cpuSelection || selection.Name == "" {
				t.Fatalf("omitted accelerator selection = %#v, want explicit CPU selection %#v", selection, cpuSelection)
			}
		})
	}
}

func TestNewDefaultBackendArtifactResolverRejectsIncompatibleRequests(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	base := ResolvedHostConfiguration{
		Backend: "localai-vibevoice", Platform: models.AssetHostPlatform{
			OperatingSystem: "linux", Architecture: "amd64",
		}, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}
	tests := []struct {
		name string
		edit func(*ResolvedHostConfiguration)
		want error
	}{
		{name: "protocol", edit: func(request *ResolvedHostConfiguration) { request.ProtocolVersion = "localai-backend-v0" }, want: artifacts.ErrIncompatibleProtocol},
		{name: "platform", edit: func(request *ResolvedHostConfiguration) { request.Platform.OperatingSystem = "freebsd" }, want: artifacts.ErrUnsupportedPlatform},
		{name: "backend", edit: func(request *ResolvedHostConfiguration) { request.Backend = "localai-unknown" }, want: artifacts.ErrUnknownBackend},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.edit(&request)
			_, err := resolver(context.Background(), request, false)
			if !errors.Is(err, test.want) {
				t.Fatalf("resolve error = %v, want %v", err, test.want)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver(cancelled, base, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolve error = %v, want context.Canceled", err)
	}
}

func TestNewDefaultHostCompatibilityCheckerUsesPinnedArtifactMatrix(t *testing.T) {
	t.Parallel()

	checker, err := NewDefaultHostCompatibilityChecker()
	if err != nil {
		t.Fatalf("NewDefaultHostCompatibilityChecker: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Configuration: ResolvedHostConfiguration{
			Backend:   "localai-llamacpp",
			ModelName: "llm",
			Platform:  models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		},
	}); err != nil {
		t.Fatalf("supported pinned host: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Configuration: ResolvedHostConfiguration{
			Backend:   "localai-llamacpp",
			ModelName: "llm",
			Platform:  models.AssetHostPlatform{OperatingSystem: "freebsd", Architecture: "amd64"},
		},
	}); err == nil {
		t.Fatal("unsupported pinned host unexpectedly passed compatibility")
	}
}

func TestGalleryBackendArtifactResolverSelectsCUDAWithoutCPUFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		backend      string
		expectedCUDA string
		expectedPath string
	}{
		{"localai-llamacpp", "cuda12-llama-cpp", "/cache/backends/cuda12-llama-cpp"},
		{"localai-whisper", "cuda12-whisper", "/cache/backends/cuda12-whisper"},
		{"localai-vibevoice", "cuda12-vibevoice-cpp", "/cache/backends/cuda12-vibevoice-cpp"},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			t.Parallel()
			var installed string
			resolver, err := NewGalleryBackendArtifactResolver(func(_ context.Context, name string, offline bool) (string, error) {
				installed = name
				if offline {
					t.Fatal("online request marked offline")
				}
				return "/cache/backends/" + name, nil
			})
			if err != nil {
				t.Fatalf("construct gallery resolver: %v", err)
			}
			selection, err := resolver(context.Background(), modelseffects.ResolvedHostConfiguration{
				Backend: tt.backend, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", CUDAAvailable: true},
			}, false)
			if err != nil {
				t.Fatalf("resolve CUDA backend %s: %v", tt.backend, err)
			}
			if installed != tt.expectedCUDA || selection.InstalledPath != tt.expectedPath || selection.Accelerator != "cuda" {
				t.Fatalf("installed = %q, selection = %#v, want installed = %q, path = %q", installed, selection, tt.expectedCUDA, tt.expectedPath)
			}
		})
	}
}

func TestGalleryBackendArtifactResolverRejectsUnsupportedAccelerator(t *testing.T) {
	t.Parallel()
	called := false
	resolver, err := NewGalleryBackendArtifactResolver(func(context.Context, string, bool) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatalf("construct gallery resolver: %v", err)
	}
	_, err = resolver(context.Background(), modelseffects.ResolvedHostConfiguration{
		Backend: "localai-whisper", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "metal"},
	}, false)
	if !errors.Is(err, artifacts.ErrIncompatibleAccelerator) || called {
		t.Fatalf("error = %v, installer called = %t", err, called)
	}
}

func TestGalleryBackendNameUsesExplicitCPUVariant(t *testing.T) {
	t.Parallel()
	name, err := galleryBackendName("localai-vibevoice", "cpu")
	if err != nil || name != "cpu-vibevoice-cpp" {
		t.Fatalf("gallery name = %q, error = %v, want cpu-vibevoice-cpp", name, err)
	}
}

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

func TestPublishedBackendResolverKeepsOlderWindowsCUDAWhenLaterReleaseIsBroken(t *testing.T) {
	t.Parallel()
	olderManifest := windowsCUDAPublicationFixture(t)
	olderTag := publicationTag(t, olderManifest)
	brokenTag := "localai-backends-v1-" + strings.Repeat("c", 64)
	newerManifest := bytes.ReplaceAll(olderManifest,
		[]byte(strings.TrimPrefix(olderTag, "localai-backends-v1-")), []byte(strings.Repeat("b", 64)))
	newerTag := publicationTag(t, newerManifest)

	newerManifest = publicationWithoutArchive(t, newerManifest, "localai-llamacpp/windows-amd64-cuda")
	index := publicationReleaseIndex(t, []publicationFixture{
		{newerTag, newerManifest}, {olderTag, olderManifest}, {brokenTag, olderManifest},
	})
	requests := 0
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		switch request.URL.String() {
		case backendReleasesURL:
			return publicationResponse(index), nil
		case backendReleaseBase + newerTag + "/manifest.json":
			return publicationResponse(newerManifest), nil
		case backendReleaseBase + olderTag + "/manifest.json":
			return publicationResponse(olderManifest), nil
		case backendReleaseBase + brokenTag + "/manifest.json":
			return publicationResponse([]byte("{")), nil
		default:
			return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
		}
	}))
	if err != nil {
		t.Fatalf("construct published resolver: %v", err)
	}
	selection, err := resolver(context.Background(), publishedWindowsRequest("localai-llamacpp"), false)
	if err != nil {
		t.Fatalf("resolve published Windows CUDA archive: %v", err)
	}
	if selection.Accelerator != "cuda" || !strings.Contains(selection.Name, "windows-amd64-cuda") ||
		!strings.Contains(selection.Location, olderTag) {
		t.Fatalf("published selection = %#v, want older Windows CUDA archive from %s", selection, olderTag)
	}
	assertPublicationRequests(t, requests, 4)
}

func TestPublishedBackendResolverSelectsCUDAFromDifferentReleasesAcrossRequests(t *testing.T) {
	t.Parallel()
	olderManifest := windowsCUDAPublicationFixture(t)
	olderTag := publicationTag(t, olderManifest)
	newerManifest := bytes.ReplaceAll(olderManifest,
		[]byte(strings.TrimPrefix(olderTag, "localai-backends-v1-")), []byte(strings.Repeat("b", 64)))
	newerTag := publicationTag(t, newerManifest)

	newerManifest = publicationWithWhisperCUDA(t, newerManifest)
	index := publicationReleaseIndex(t, []publicationFixture{{newerTag, newerManifest}, {olderTag, olderManifest}})
	requests := 0
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		switch request.URL.String() {
		case backendReleasesURL:
			return publicationResponse(index), nil
		case backendReleaseBase + newerTag + "/manifest.json":
			return publicationResponse(newerManifest), nil
		case backendReleaseBase + olderTag + "/manifest.json":
			return publicationResponse(olderManifest), nil
		default:
			return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
		}
	}))
	if err != nil {
		t.Fatalf("construct published resolver: %v", err)
	}
	for _, test := range []struct {
		backend string
		tag     string
	}{{"localai-llamacpp", olderTag}, {"localai-whisper", newerTag}} {
		selection, err := resolver(context.Background(), publishedWindowsRequest(test.backend), false)
		if err != nil || selection.Accelerator != "cuda" ||
			!strings.Contains(selection.Name, test.backend+"-windows-amd64-cuda") ||
			!strings.Contains(selection.Location, "/"+test.tag+"/") {
			t.Fatalf("%s selection = %#v, %v, want CUDA archive from %s", test.backend, selection, err, test.tag)
		}
	}
	assertPublicationRequests(t, requests, 3)
}

type publicationFixture struct {
	tag      string
	manifest []byte
}

func publicationReleaseIndex(t *testing.T, fixtures []publicationFixture) []byte {
	t.Helper()
	var releases []json.RawMessage
	for _, fixture := range fixtures {
		var entry []json.RawMessage
		if err := json.Unmarshal(publicationIndex(t, fixture.tag, fixture.manifest), &entry); err != nil {
			t.Fatalf("decode publication index: %v", err)
		}
		releases = append(releases, entry...)
	}
	index, err := json.Marshal(releases)
	if err != nil {
		t.Fatalf("encode publication index: %v", err)
	}
	return index
}

func decodePublicationFixture(t *testing.T, data []byte) (map[string]json.RawMessage, []json.RawMessage) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode publication fixture: %v", err)
	}
	var archives []json.RawMessage
	if err := json.Unmarshal(document["artifacts"], &archives); err != nil {
		t.Fatalf("decode publication archives: %v", err)
	}
	return document, archives
}

func encodePublicationFixture(t *testing.T, document map[string]json.RawMessage, archives []json.RawMessage) []byte {
	t.Helper()
	var err error
	document["artifacts"], err = json.Marshal(archives)
	if err != nil {
		t.Fatalf("encode publication archives: %v", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode publication: %v", err)
	}
	return data
}

func publicationArchiveID(t *testing.T, archive json.RawMessage) string {
	t.Helper()
	var identity struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(archive, &identity); err != nil {
		t.Fatalf("decode archive identity: %v", err)
	}
	return identity.ID
}

func publicationWithoutArchive(t *testing.T, data []byte, omittedID string) []byte {
	t.Helper()
	document, archives := decodePublicationFixture(t, data)
	kept := make([]json.RawMessage, 0, len(archives))
	for _, archive := range archives {
		if publicationArchiveID(t, archive) != omittedID {
			kept = append(kept, archive)
		}
	}
	if len(kept) != len(archives)-1 {
		t.Fatalf("expected one %s archive in publication fixture", omittedID)
	}
	return encodePublicationFixture(t, document, kept)
}

func publicationWithWhisperCUDA(t *testing.T, data []byte) []byte {
	t.Helper()
	document, archives := decodePublicationFixture(t, data)
	var whisperCPU json.RawMessage
	for _, archive := range archives {
		if publicationArchiveID(t, archive) == "localai-whisper/windows-amd64" {
			whisperCPU = archive
			break
		}
	}
	if whisperCPU == nil {
		t.Fatal("expected Whisper CPU archive in publication fixture")
	}
	whisperCUDA := whisperCUDAArchive(t, whisperCPU)
	withoutLlama := publicationWithoutArchive(t, data, "localai-llamacpp/windows-amd64-cuda")
	document, archives = decodePublicationFixture(t, withoutLlama)
	return encodePublicationFixture(t, document, append(archives, whisperCUDA))
}

func whisperCUDAArchive(t *testing.T, cpu json.RawMessage) json.RawMessage {
	t.Helper()
	var archive map[string]json.RawMessage
	if err := json.Unmarshal(cpu, &archive); err != nil {
		t.Fatalf("decode Whisper archive: %v", err)
	}
	archive["id"] = json.RawMessage(`"localai-whisper/windows-amd64-cuda"`)
	archive["target"] = json.RawMessage(`{"id":"windows-amd64-cuda","operatingSystem":"windows","architecture":"amd64","accelerators":["cuda"]}`)
	var artifact map[string]json.RawMessage
	if err := json.Unmarshal(archive["artifact"], &artifact); err != nil {
		t.Fatalf("decode Whisper archive artifact: %v", err)
	}
	for _, field := range []string{"name", "location"} {
		artifact[field] = bytes.Replace(artifact[field], []byte("windows-amd64"), []byte("windows-amd64-cuda"), 1)
	}
	var err error
	archive["artifact"], err = json.Marshal(artifact)
	if err != nil {
		t.Fatalf("encode Whisper CUDA artifact: %v", err)
	}
	encoded, err := json.Marshal(archive)
	if err != nil {
		t.Fatalf("encode Whisper CUDA archive: %v", err)
	}
	return encoded
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
	manifests, err := fetchPublishedBackendManifests(context.Background(), &http.Client{Timeout: 15 * time.Second}, baseline.ProtocolRevision())
	if err != nil {
		t.Fatalf("fetch current publication: %v", err)
	}
	selection, err := backendArtifactResolver(manifests[0])(context.Background(), publishedWindowsRequest("localai-llamacpp"), false)
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
