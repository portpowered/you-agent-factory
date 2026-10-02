package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestPublishedBackendResolverRetriesDiscoveryAfterFailure(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	requests := 0
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return publicationResponse(nil), fmt.Errorf("transient publication failure")
		}
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
	request := publishedWindowsRequest("localai-llamacpp")
	assertPublishedResolution(t, resolver, request, false, "cpu", "windows-amd64")
	assertPublicationRequests(t, requests, 1)
	assertPublishedResolution(t, resolver, request, false, "cuda", "windows-amd64-cuda")
	assertPublicationRequests(t, requests, 3)
	assertPublishedResolution(t, resolver, request, false, "cuda", "windows-amd64-cuda")
	assertPublicationRequests(t, requests, 3)
}

func TestPublishedBackendResolverUnpinsWindowsCapabilities(t *testing.T) {
	t.Parallel()
	older := windowsCUDAPublicationFixture(t)
	olderTag := publicationTag(t, older)
	newer := bytes.ReplaceAll(older, []byte(strings.TrimPrefix(olderTag, "localai-backends-v1-")), []byte(strings.Repeat("d", 64)))
	newer = publicationWithoutArchive(t, newer, "localai-llamacpp/windows-amd64-cuda")
	newerTag := publicationTag(t, newer)
	index := publicationReleaseIndex(t, []publicationFixture{{newerTag, newer}, {olderTag, older}})
	var requests atomic.Int32
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch request.URL.String() {
		case backendReleasesURL:
			return publicationResponse(index), nil
		case backendReleaseBase + newerTag + "/manifest.json":
			return publicationResponse(newer), nil
		case backendReleaseBase + olderTag + "/manifest.json":
			return publicationResponse(older), nil
		default:
			return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-llamacpp")

	assertUnpinnedSelection(t, resolver, request, true, "cpu", "")
	if got := requests.Load(); got != 0 {
		t.Fatalf("offline requests = %d, want 0", got)
	}
	assertUnpinnedSelection(t, resolver, request, false, "cuda", olderTag)
	if got := requests.Load(); got != 3 {
		t.Fatalf("online requests = %d, want 3", got)
	}

	request.Platform.Accelerator = "cuda"
	assertUnpinnedSelection(t, resolver, request, false, "cuda", olderTag)
	assertUnpinnedSelection(t, resolver, request, true, "cuda", olderTag)
	request.Platform.Accelerator = "cpu"
	assertUnpinnedSelection(t, resolver, request, false, "cpu", newerTag)
	assertUnpinnedSelection(t, resolver, request, true, "cpu", newerTag)
	request.Platform.Accelerator = ""
	request.Platform.CUDAAvailable = false
	assertUnpinnedSelection(t, resolver, request, false, "cpu", newerTag)
	if got := requests.Load(); got != 3 {
		t.Fatalf("cached publication requests = %d, want 3", got)
	}
}

func TestPublishedBackendResolverSelectsValidatedLinuxArchives(t *testing.T) {
	t.Parallel()
	baseline := windowsCUDAPublicationFixture(t)
	oldTag := publicationTag(t, baseline)
	manifest := bytes.ReplaceAll(baseline, []byte(strings.TrimPrefix(oldTag, "localai-backends-v1-")), []byte(strings.Repeat("a", 64)))
	manifest = publicationWithLinuxCUDA(t, manifest)
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	var requests atomic.Int32
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch request.URL.String() {
		case backendReleasesURL:
			return publicationResponse(index), nil
		case backendReleaseBase + tag + "/manifest.json":
			return publicationResponse(manifest), nil
		default:
			return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedLinuxRequest("localai-llamacpp")
	assertUnpinnedSelection(t, resolver, request, true, "cpu", oldTag)
	if got := requests.Load(); got != 0 {
		t.Fatalf("offline requests = %d, want 0", got)
	}
	assertUnpinnedSelection(t, resolver, request, false, "cuda", tag)
	request.Platform.Accelerator = "cuda"
	assertUnpinnedSelection(t, resolver, request, false, "cuda", tag)
	assertUnpinnedSelection(t, resolver, request, true, "cuda", tag)
	request.Platform.Accelerator = "cpu"
	assertUnpinnedSelection(t, resolver, request, false, "cpu", tag)
	request.Platform.Accelerator = ""
	request.Platform.CUDAAvailable = false
	assertUnpinnedSelection(t, resolver, request, false, "cpu", tag)
	if got := requests.Load(); got != 2 {
		t.Fatalf("publication requests = %d, want 2", got)
	}
}

func TestPublishedBackendResolverLinuxCUDARequiresPublishedArchive(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	tag := publicationTag(t, manifest)
	var requests atomic.Int32
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(publicationIndex(t, tag, manifest)), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedLinuxRequest("localai-llamacpp")
	request.Platform.Accelerator = "cuda"
	if _, err := resolver(context.Background(), request, false); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("explicit Linux CUDA error = %v, want incompatible accelerator", err)
	}
	if _, err := resolver(context.Background(), request, true); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("offline Linux CUDA error = %v, want incompatible accelerator", err)
	}
	request.Platform.Accelerator = ""
	assertUnpinnedSelection(t, resolver, request, false, "cpu", tag)
	if got := requests.Load(); got != 2 {
		t.Fatalf("publication requests = %d, want 2", got)
	}
}

func TestPublishedBackendResolverLinuxFallsBackToBaselineWhenPublicationHasNoCompatibleArchive(t *testing.T) {
	t.Parallel()
	baseline := windowsCUDAPublicationFixture(t)
	oldTag := publicationTag(t, baseline)
	manifest := publicationWithoutArchive(t, baseline, "localai-llamacpp/linux-amd64")
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedLinuxRequest("localai-llamacpp")
	request.Platform.Accelerator = "cpu"
	selection, err := resolver(context.Background(), request, false)
	if err != nil || selection.Accelerator != "cpu" || !strings.Contains(selection.Location, "/"+oldTag+"/") {
		t.Fatalf("fallback selection = %#v, error = %v, want checked-in CPU archive", selection, err)
	}
	assertUnpinnedSelection(t, resolver, request, true, "cpu", oldTag)
}

func TestPublishedBackendResolverRejectsUnsupportedLinuxAccelerator(t *testing.T) {
	t.Parallel()
	manifest := publicationWithLinuxCUDA(t, windowsCUDAPublicationFixture(t))
	tag := publicationTag(t, manifest)
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(publicationIndex(t, tag, manifest)), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedLinuxRequest("localai-llamacpp")
	request.Platform.Accelerator = "metal"
	if _, err := resolver(context.Background(), request, false); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("unsupported Linux accelerator error = %v, want incompatible accelerator", err)
	}
}

func TestPublishedBackendResolverRejectsLinuxArchiveMissingFromRelease(t *testing.T) {
	t.Parallel()
	manifest := publicationWithLinuxCUDA(t, windowsCUDAPublicationFixture(t))
	tag := publicationTag(t, manifest)
	_, archives := decodePublicationFixture(t, manifest)
	var cudaName string
	for _, archive := range archives {
		if publicationArchiveID(t, archive) == "localai-llamacpp/linux-amd64-cuda" {
			var entry backendArchiveEntry
			if err := json.Unmarshal(archive, &entry); err != nil {
				t.Fatalf("decode Linux CUDA archive: %v", err)
			}
			cudaName = entry.Artifact.Name
			break
		}
	}
	if cudaName == "" {
		t.Fatal("fixture has no Linux CUDA archive")
	}
	completeIndex := publicationIndex(t, tag, manifest)
	index := bytes.Replace(completeIndex, []byte(cudaName), []byte("missing-archive.tar.gz"), 1)
	if bytes.Equal(index, completeIndex) {
		t.Fatal("fixture release index still declares the Linux CUDA archive")
	}
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedLinuxRequest("localai-llamacpp")
	request.Platform.Accelerator = "cuda"
	if _, err := resolver(context.Background(), request, false); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("unpublished Linux CUDA error = %v, want incompatible accelerator", err)
	}
}

func publicationWithLinuxCUDA(t *testing.T, data []byte) []byte {
	t.Helper()
	document, archives := decodePublicationFixture(t, data)
	for _, archive := range archives {
		if publicationArchiveID(t, archive) != "localai-llamacpp/windows-amd64-cuda" {
			continue
		}
		linux := bytes.ReplaceAll(archive, []byte("windows-amd64-cuda"), []byte("linux-amd64-cuda"))
		linux = bytes.ReplaceAll(linux, []byte(`"operatingSystem": "windows"`), []byte(`"operatingSystem": "linux"`))
		linux = bytes.ReplaceAll(linux, []byte(".zip"), []byte(".tar.gz"))
		return encodePublicationFixture(t, document, append(archives, linux))
	}
	t.Fatal("expected Windows CUDA archive in publication fixture")
	return nil
}

func publishedLinuxRequest(backend string) ResolvedHostConfiguration {
	request := publishedWindowsRequest(backend)
	request.Platform.OperatingSystem = "linux"
	return request
}

func TestPublishedBackendResolverFirstOnlineRequestFetchesAndCaches(t *testing.T) {
	t.Parallel()
	baseline := windowsCUDAPublicationFixture(t)
	oldTag := publicationTag(t, baseline)
	manifest := bytes.ReplaceAll(baseline, []byte(strings.TrimPrefix(oldTag, "localai-backends-v1-")), []byte(strings.Repeat("e", 64)))
	tag := publicationTag(t, manifest)
	for _, test := range []struct {
		name        string
		accelerator string
		cuda        bool
		want        string
	}{
		{"implicit CPU", "", false, "cpu"},
		{"explicit CPU", "cpu", true, "cpu"},
		{"explicit CUDA", "cuda", true, "cuda"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			index := publicationIndex(t, tag, manifest)
			resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				if request.URL.String() == backendReleasesURL {
					return publicationResponse(index), nil
				}
				if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
					return publicationResponse(manifest), nil
				}
				return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
			}))
			if err != nil {
				t.Fatal(err)
			}
			request := publishedWindowsRequest("localai-llamacpp")
			request.Platform.CUDAAvailable = test.cuda
			request.Platform.Accelerator = test.accelerator
			assertUnpinnedSelection(t, resolver, request, false, test.want, tag)
			assertUnpinnedSelection(t, resolver, request, true, test.want, tag)
			if got := requests.Load(); got != 2 {
				t.Fatalf("publication requests = %d, want 2", got)
			}
		})
	}
}

func TestPublishedBackendResolverNetworkFailureKeepsExplicitCUDAError(t *testing.T) {
	t.Parallel()
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network unavailable")
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-llamacpp")
	request.Platform.Accelerator = "cuda"
	if _, err := resolver(context.Background(), request, false); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("explicit CUDA error = %v, want incompatible accelerator", err)
	}
}

func TestPublishedBackendResolverUsesBaselineWhenPublicationHasNoCompatibleArchive(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	oldTag := publicationTag(t, manifest)
	manifest = bytes.ReplaceAll(manifest, []byte(strings.TrimPrefix(oldTag, "localai-backends-v1-")), []byte(strings.Repeat("f", 64)))
	manifest = publicationWithoutArchive(t, manifest, "localai-llamacpp/windows-amd64-cuda")
	manifest = publicationWithoutArchive(t, manifest, "localai-llamacpp/windows-amd64")
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-llamacpp")
	selection, err := resolver(context.Background(), request, false)
	if err != nil || selection.Accelerator != "cpu" || !strings.Contains(selection.Location, "/"+oldTag+"/") {
		t.Fatalf("fallback selection = %#v, error = %v, want checked-in CPU archive", selection, err)
	}
}

func TestPublishedBackendResolverExplicitCUDANeverFallsBackToCPU(t *testing.T) {
	t.Parallel()
	manifest := publicationWithoutArchive(t, windowsCUDAPublicationFixture(t), "localai-llamacpp/windows-amd64-cuda")
	tag := publicationTag(t, manifest)
	var requests atomic.Int32
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(publicationIndex(t, tag, manifest)), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-llamacpp")
	request.Platform.Accelerator = "cuda"
	if _, err := resolver(context.Background(), request, false); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("explicit CUDA error = %v, want incompatible accelerator", err)
	}
	if _, err := resolver(context.Background(), request, true); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("offline explicit CUDA error = %v, want incompatible accelerator", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("publication requests = %d, want 2", got)
	}
	request.Platform.Accelerator = ""
	assertUnpinnedSelection(t, resolver, request, false, "cpu", tag)
}

func TestPublishedBackendResolverConcurrentFirstResolution(t *testing.T) {
	t.Parallel()
	manifest := windowsCUDAPublicationFixture(t)
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	var requests atomic.Int32
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.URL.String() == backendReleasesURL {
			return publicationResponse(index), nil
		}
		if request.URL.String() == backendReleaseBase+tag+"/manifest.json" {
			return publicationResponse(manifest), nil
		}
		return nil, fmt.Errorf("unexpected publication request: %s", request.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errorsFound := make(chan error, 12)
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			selection, err := resolver(context.Background(), publishedWindowsRequest("localai-llamacpp"), false)
			if err != nil || selection.Accelerator != "cuda" || !strings.Contains(selection.Location, tag) {
				errorsFound <- fmt.Errorf("selection = %#v, error = %v", selection, err)
			}
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("concurrent publication requests = %d, want 2", got)
	}
}

func assertUnpinnedSelection(t *testing.T, resolver BackendArtifactResolver, request ResolvedHostConfiguration, offline bool, accelerator, tag string) {
	t.Helper()
	selection, err := resolver(context.Background(), request, offline)
	if err != nil || selection.Accelerator != accelerator {
		t.Fatalf("selection = %#v, error = %v, want %s", selection, err, accelerator)
	}
	if tag != "" && !strings.Contains(selection.Location, "/"+tag+"/") {
		t.Fatalf("location = %q, want publication %s", selection.Location, tag)
	}
}

// TestManualPublicationManifestProbe can be run against a staged manual release
// before publication by setting LOCALAI_MANUAL_PUBLICATION_MANIFEST to its manifest.json.
func TestManualPublicationManifestProbe(t *testing.T) {
	manifestPath := os.Getenv("LOCALAI_MANUAL_PUBLICATION_MANIFEST")
	if manifestPath == "" {
		t.Skip("set LOCALAI_MANUAL_PUBLICATION_MANIFEST to probe a staged publication")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := artifacts.Decode(data)
	if err != nil {
		t.Fatalf("decode staged manifest: %v", err)
	}
	if manifest.ArtifactCount() != 10 {
		t.Fatalf("artifact count = %d, want 10", manifest.ArtifactCount())
	}
	resolve := backendArtifactResolver(manifest)
	for _, scenario := range []struct {
		backend, accelerator, target string
	}{
		{"localai-llamacpp", "cuda", "windows-amd64-cuda"},
		{"localai-whisper", "cpu", "windows-amd64"},
		{"localai-vibevoice", "cpu", "windows-amd64"},
	} {
		selection, err := resolve(context.Background(), ResolvedHostConfiguration{
			Backend: scenario.backend,
			Platform: models.AssetHostPlatform{
				OperatingSystem: "windows", Architecture: "amd64", Accelerator: scenario.accelerator,
			},
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		}, false)
		if err != nil {
			t.Fatalf("resolve %s/%s: %v", scenario.backend, scenario.accelerator, err)
		}
		if selection.Accelerator != scenario.accelerator || !strings.Contains(selection.Name, scenario.target) {
			t.Fatalf("resolve %s/%s = %#v, want %s", scenario.backend, scenario.accelerator, selection, scenario.target)
		}
	}
}

func qwenCUDAPublicationFixture(t *testing.T) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(windowsCUDAPublicationFixture(t), &document); err != nil {
		t.Fatal(err)
	}
	entry := document["artifacts"].([]any)[0].(map[string]any)
	entry["id"] = "localai-qwen3-tts-cpp/windows-amd64-cuda"
	entry["backend"] = map[string]any{"id": "localai-qwen3-tts-cpp", "source": map[string]any{
		"repository": "https://github.com/ServeurpersoCom/qwentts.cpp",
		"commit":     "d17c33d4ee2f56d15f9ca8a1bb82f7389305f838", "pinVariable": "QWEN3TTS_CPP_VERSION",
	}}
	entry["source"].(map[string]any)["path"] = "backend/go/qwen3-tts-cpp"
	entry["target"] = map[string]any{"id": "windows-amd64-cuda", "operatingSystem": "windows", "architecture": "amd64", "accelerators": []string{"cuda"}}
	document["artifacts"] = []any{entry}
	manifest, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestPublishedQwenWindowsCUDAReleasePreservesOtherBackendFallbacks(t *testing.T) {
	t.Parallel()
	manifest := qwenCUDAPublicationFixture(t)
	tag := publicationTag(t, manifest)
	index := publicationIndex(t, tag, manifest)
	resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case backendReleasesURL:
			return publicationResponse(index), nil
		case backendReleaseBase + tag + "/manifest.json":
			return publicationResponse(manifest), nil
		default:
			return nil, fmt.Errorf("unexpected publication request")
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-qwen3-tts-cpp")
	request.Platform.CUDAAvailable = true
	assertUnpinnedSelection(t, resolver, request, false, "cuda", tag)
	request.Backend = "localai-whisper"
	request.Platform.CUDAAvailable = false
	selection, err := resolver(t.Context(), request, false)
	if err != nil || selection.Accelerator != "cpu" || !strings.Contains(selection.Name, "localai-whisper") {
		t.Fatalf("Whisper fallback after Qwen publication = %#v/%v", selection, err)
	}
}

func TestPublishedQwenResolverUsesPublicationDateBeforeAPIListingOrder(t *testing.T) {
	t.Parallel()
	older := qwenCUDAPublicationFixture(t)
	oldTag := publicationTag(t, older)
	newTag := "localai-backends-v1-" + strings.Repeat("a", 64)
	newer := bytes.ReplaceAll(older, []byte(oldTag), []byte(newTag))
	newer = bytes.ReplaceAll(newer, []byte(strings.TrimPrefix(oldTag, "localai-backends-v1-")), []byte(strings.Repeat("a", 64)))
	var newDocument map[string]any
	if err := json.Unmarshal(newer, &newDocument); err != nil {
		t.Fatal(err)
	}
	newDocument["artifacts"].([]any)[0].(map[string]any)["artifact"].(map[string]any)["sha256"] = strings.Repeat("b", 64)
	newer, err := json.Marshal(newDocument)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, oldDate, newDate, wantTag string }{
		{"old listed first", "2026-10-02T04:42:07Z", "2026-10-02T08:41:57Z", newTag},
		{"invalid older date", "invalid", "2026-10-02T08:41:57Z", newTag},
		{"missing dates preserve listing", "", "", oldTag},
		{"tied dates preserve listing", "2026-10-02T08:41:57Z", "2026-10-02T08:41:57Z", oldTag},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded := publicationIndexWithDates(t, older, newer, tc.oldDate, tc.newDate)
			var fetched []string
			resolver, err := NewPublishedBackendArtifactResolver(backendPublicationDoer(func(request *http.Request) (*http.Response, error) {
				switch request.URL.String() {
				case backendReleasesURL:
					return publicationResponse(encoded), nil
				case backendReleaseBase + oldTag + "/manifest.json":
					fetched = append(fetched, oldTag)
					return publicationResponse(older), nil
				case backendReleaseBase + newTag + "/manifest.json":
					fetched = append(fetched, newTag)
					return publicationResponse(newer), nil
				default:
					return nil, fmt.Errorf("unexpected publication request")
				}
			}))
			if err != nil {
				t.Fatal(err)
			}
			request := publishedWindowsRequest("localai-qwen3-tts-cpp")
			assertUnpinnedSelection(t, resolver, request, false, "cuda", tc.wantTag)
			selection, err := resolver(t.Context(), request, false)
			if err != nil || (tc.wantTag == newTag && selection.SHA256 != strings.Repeat("b", 64)) {
				t.Fatalf("current archive checksum = %q/%v, want new publication's checksum", selection.SHA256, err)
			}
			if len(fetched) != 2 || fetched[0] != tc.wantTag {
				t.Fatalf("manifest fetch order = %v, want selected publication fetched first", fetched)
			}
			assertUnpinnedSelection(t, resolver, request, true, "cuda", tc.wantTag)
			if len(fetched) != 2 {
				t.Fatal("cached offline selection fetched manifests again")
			}
		})
	}
}

func publicationIndexWithDates(t *testing.T, older, newer []byte, oldDate, newDate string) []byte {
	t.Helper()
	var index []map[string]any
	for _, publication := range []struct {
		date string
		data []byte
	}{{oldDate, older}, {newDate, newer}} {
		var entries []map[string]any
		if err := json.Unmarshal(publicationIndex(t, publicationTag(t, publication.data), publication.data), &entries); err != nil {
			t.Fatal(err)
		}
		entries[0]["published_at"] = publication.date
		index = append(index, entries[0])
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestPinnedQwenWindowsCUDACandidateAllowsOfflineSelectionAndRejectsCPU(t *testing.T) {
	t.Parallel()
	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatal(err)
	}
	request := publishedWindowsRequest("localai-qwen3-tts-cpp")
	selection, err := resolver(t.Context(), request, true)
	if err != nil || selection.Accelerator != "cuda" || selection.Bytes != 445867793 || selection.SHA256 != "58fe20ab56dcf6f4e4e485d71138bfa2c24605c1823817a763617199c9172422" {
		t.Fatalf("pinned Qwen candidate = %#v/%v, want exact verified Windows archive", selection, err)
	}
	const archiveName = "localai-backend-localai-qwen3-tts-cpp-windows-amd64-cuda-d17c33d4ee2f56d15f9ca8a1bb82f7389305f838-dea7691547a1.zip"
	const releaseTag = "localai-backends-v1-53cf3a40d01e831eff5ba390a8f761badef03824e2b6439dfc1fd5ac0072cc87"
	if selection.Name != archiveName || selection.Location != backendReleaseBase+releaseTag+"/"+archiveName {
		t.Fatalf("pinned Qwen identity = %q/%q, want exact immutable sampler publication", selection.Name, selection.Location)
	}
	request.Platform.CUDAAvailable = false
	if _, err := resolver(t.Context(), request, true); !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
		t.Fatalf("Qwen CPU selection error = %v, want unavailable accelerator", err)
	}
	request.Platform.OperatingSystem = "darwin"
	request.Platform.Architecture = "arm64"
	if _, err := resolver(t.Context(), request, false); err == nil {
		t.Fatal("Qwen Darwin selection succeeded without a published target")
	}
}
