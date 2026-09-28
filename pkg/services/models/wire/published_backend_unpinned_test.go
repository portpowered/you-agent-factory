package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

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
