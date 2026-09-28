package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// NewDefaultBackendArtifactResolver constructs the production selector for
// the checked-in pinned publication. The manifest is decoded and validated at
// composition time; the returned effect only performs deterministic
// capability selection and returns detached archive facts.
func NewDefaultBackendArtifactResolver() (BackendArtifactResolver, error) {
	manifest, err := artifacts.DefaultManifest()
	if err != nil {
		return nil, fmt.Errorf("decode default backend artifact manifest: %w", err)
	}
	return backendArtifactResolver(manifest), nil
}

func backendArtifactResolver(manifest artifacts.Manifest) BackendArtifactResolver {
	return func(ctx context.Context, request ResolvedHostConfiguration, _ bool) (BackendArtifactSelection, error) {
		if err := ctx.Err(); err != nil {
			return BackendArtifactSelection{}, err
		}
		if request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
			return BackendArtifactSelection{}, fmt.Errorf(
				"%w: requested protocol %q",
				artifacts.ErrIncompatibleProtocol,
				request.ProtocolVersion,
			)
		}
		accelerator := defaultBackendAccelerator(request.Platform)
		preferCUDA := request.Platform.Accelerator == "" && accelerator == "cuda"
		selection := artifacts.SelectionRequest{
			Backend:          request.Backend,
			OperatingSystem:  request.Platform.OperatingSystem,
			Architecture:     request.Platform.Architecture,
			ProtocolRevision: manifest.ProtocolRevision(),
			Accelerator:      accelerator,
		}
		descriptor, err := manifest.Select(selection)
		if preferCUDA && errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
			selection.Accelerator = "cpu"
			descriptor, err = manifest.Select(selection)
		}
		if err != nil {
			return BackendArtifactSelection{}, err
		}
		return BackendArtifactSelection{
			Name:        descriptor.Artifact.Name,
			Location:    descriptor.Artifact.Location,
			Bytes:       descriptor.Artifact.SizeBytes,
			SHA256:      descriptor.Artifact.SHA256,
			Accelerator: selection.Accelerator,
		}, nil
	}
}

func defaultBackendAccelerator(platform models.AssetHostPlatform) string {
	if platform.Accelerator != "" {
		return platform.Accelerator
	}
	if platform.CUDAAvailable && platform.Architecture == "amd64" &&
		(platform.OperatingSystem == "linux" || platform.OperatingSystem == "windows") {
		return "cuda"
	}
	if platform.OperatingSystem == "darwin" && platform.Architecture == "arm64" {
		return "metal"
	}
	if (platform.OperatingSystem == "linux" || platform.OperatingSystem == "windows") &&
		platform.Architecture == "amd64" {
		return "cpu"
	}
	return ""
}

const (
	backendReleasesURL   = "https://api.github.com/repos/portpowered/you-agent-factory/releases?per_page=30"
	backendReleaseBase   = "https://github.com/portpowered/you-agent-factory/releases/download/"
	backendReleaseLimit  = 1 << 20
	backendManifestLimit = 2 << 20
)

var backendReleaseTag = regexp.MustCompile(`^localai-backends-v1-[0-9a-f]{64}$`)

// NewPublishedBackendArtifactResolver checks published Windows archive
// manifests on first online use. Linux uses the current
// LocalAI gallery; this resolver keeps the checked-in publication as the
// fallback when the release index is unavailable or has no compatible
// archive. Offline calls never access the network.
func NewPublishedBackendArtifactResolver(client AssetHTTPDoer) (BackendArtifactResolver, error) {
	if client == nil {
		return nil, fmt.Errorf("backend publication HTTP client is required")
	}
	baseline, err := artifacts.DefaultManifest()
	if err != nil {
		return nil, fmt.Errorf("decode default backend artifact manifest: %w", err)
	}
	baselineResolver := backendArtifactResolver(baseline)
	var mu sync.Mutex
	checked := false
	var published []artifacts.Manifest
	return func(ctx context.Context, request ResolvedHostConfiguration, offline bool) (BackendArtifactSelection, error) {
		if err := ctx.Err(); err != nil {
			return BackendArtifactSelection{}, err
		}
		if request.Platform.OperatingSystem != "windows" || request.Platform.Architecture != "amd64" {
			return baselineResolver(ctx, request, offline)
		}
		if _, err := baselineResolver(ctx, request, offline); err != nil &&
			!(request.Platform.Accelerator == "cuda" && errors.Is(err, artifacts.ErrIncompatibleAccelerator)) {
			return BackendArtifactSelection{}, err
		}
		mu.Lock()
		if !offline && !checked {
			candidates, fetchErr := fetchPublishedBackendManifests(ctx, client, baseline.ProtocolRevision())
			if ctx.Err() != nil {
				mu.Unlock()
				return BackendArtifactSelection{}, ctx.Err()
			}
			checked = true
			if fetchErr == nil {
				published = candidates
			}
		}
		selection := selectPublishedBackend(ctx, published, request, offline)
		mu.Unlock()
		if selection.Name != "" {
			return selection, nil
		}
		return baselineResolver(ctx, request, offline)
	}, nil
}

func selectPublishedBackend(ctx context.Context, manifests []artifacts.Manifest, request ResolvedHostConfiguration, offline bool) BackendArtifactSelection {
	if request.Platform.Accelerator == "" && request.Platform.CUDAAvailable {
		if selection := selectPublishedBackendAccelerator(ctx, manifests, request, offline, "cuda"); selection.Name != "" {
			return selection
		}
	}
	if request.Platform.Accelerator == "cuda" {
		return selectPublishedBackendAccelerator(ctx, manifests, request, offline, "cuda")
	}
	return selectPublishedBackendAccelerator(ctx, manifests, request, offline, "cpu")
}

func selectPublishedBackendAccelerator(ctx context.Context, manifests []artifacts.Manifest, request ResolvedHostConfiguration, offline bool, accelerator string) BackendArtifactSelection {
	request.Platform.Accelerator = accelerator
	for _, manifest := range manifests {
		candidate, err := backendArtifactResolver(manifest)(ctx, request, offline)
		if err == nil && candidate.Accelerator == accelerator {
			return candidate
		}
	}
	return BackendArtifactSelection{}
}

type backendRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

type backendArchiveEntry struct {
	Artifact struct {
		Name string `json:"name"`
	} `json:"artifact"`
}

func fetchPublishedBackendManifests(ctx context.Context, client AssetHTTPDoer, protocol string) ([]artifacts.Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	index, err := readBackendPublication(ctx, client, backendReleasesURL, backendReleaseLimit)
	if err != nil {
		return nil, err
	}
	var releases []backendRelease
	if err := json.Unmarshal(index, &releases); err != nil {
		return nil, fmt.Errorf("decode backend publication index: %w", err)
	}
	var manifests []artifacts.Manifest
	for _, release := range releases {
		if release.Draft || release.Prerelease || !backendReleaseTag.MatchString(release.TagName) || !hasBackendManifest(release) {
			continue
		}
		data, err := readBackendPublication(ctx, client, backendReleaseBase+release.TagName+"/manifest.json", backendManifestLimit)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		manifest, err := decodePublishedBackendManifest(data, release)
		if err != nil {
			continue
		}
		if manifest.ProtocolRevision() != protocol {
			continue
		}
		manifests = append(manifests, manifest)
	}
	if len(manifests) == 0 {
		return nil, fmt.Errorf("no backend publication manifest was found")
	}
	return manifests, nil
}

func decodePublishedBackendManifest(data []byte, release backendRelease) (artifacts.Manifest, error) {
	var identity struct {
		Publication struct {
			ReleaseTag     string `json:"releaseTag"`
			Repository     string `json:"repository"`
			PinFingerprint string `json:"pinFingerprint"`
		} `json:"publication"`
		Artifacts []backendArchiveEntry `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &identity); err != nil || identity.Publication.ReleaseTag != release.TagName ||
		identity.Publication.Repository != "portpowered/you-agent-factory" ||
		"localai-backends-v1-"+identity.Publication.PinFingerprint != release.TagName {
		return artifacts.Manifest{}, fmt.Errorf("backend publication manifest does not match its release")
	}
	if !manifestAssetsPublished(identity.Artifacts, release) {
		return artifacts.Manifest{}, fmt.Errorf("backend publication is missing a declared archive")
	}
	return artifacts.Decode(data)
}

func manifestAssetsPublished(entries []backendArchiveEntry, release backendRelease) bool {
	if len(entries) == 0 {
		return false
	}
	for _, entry := range entries {
		found := false
		for _, asset := range release.Assets {
			if asset.Name == entry.Artifact.Name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func hasBackendManifest(release backendRelease) bool {
	for _, asset := range release.Assets {
		if asset.Name == "manifest.json" {
			return true
		}
	}
	return false
}

func readBackendPublication(ctx context.Context, client AssetHTTPDoer, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "you-agent-factory")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("backend publication response is missing")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend publication request returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit || strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("backend publication response is empty or oversized")
	}
	return data, nil
}
