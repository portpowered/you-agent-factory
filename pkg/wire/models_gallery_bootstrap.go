package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	localAIReleaseURL       = "https://api.github.com/repos/mudler/LocalAI/releases/latest"
	localAIReleaseTimeout   = 10 * time.Minute
	localAIResponseMaxBytes = 2 << 20
	localAIBinaryMaxBytes   = 1 << 30
)

type localAIReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type localAIRelease struct {
	Tag    string                `json:"tag_name"`
	Assets []localAIReleaseAsset `json:"assets"`
}

type localAIHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// resolveLocalAIBinary returns an explicit override, a binary on PATH, or a
// verified binary from the latest official Linux amd64 release. cacheDir is
// injectable for callers and tests; an empty value uses the user cache.
func resolveLocalAIBinary(ctx context.Context, client localAIHTTPDoer, cacheDir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if command := strings.TrimSpace(os.Getenv(localAIBinaryEnvironment)); command != "" {
		return command, nil
	}
	if command, err := exec.LookPath("local-ai"); err == nil {
		return command, nil
	}
	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("resolve LocalAI binary cache: %w", err)
		}
		cacheDir = filepath.Join(userCache, "you", "localai")
	}
	return downloadLocalAIBinary(ctx, client, cacheDir, localAIReleaseURL)
}

func downloadLocalAIBinary(ctx context.Context, client localAIHTTPDoer, cacheDir, releaseURL string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, localAIReleaseTimeout)
	defer cancel()
	release, err := fetchLocalAIRelease(ctx, client, releaseURL)
	if err != nil {
		return "", err
	}
	binary, checksums, err := selectLocalAIReleaseAssets(release)
	if err != nil {
		return "", err
	}
	wantDigest, err := fetchLocalAIAssetDigest(ctx, client, checksums.URL, binary.Name)
	if err != nil {
		return "", err
	}
	versionDir := filepath.Join(cacheDir, release.Tag)
	path := filepath.Join(versionDir, binary.Name)
	if matchesLocalAIFile(path, binary.Size, wantDigest) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return path, nil
	}
	return stageLocalAIBinary(ctx, client, versionDir, path, binary, wantDigest)
}

func fetchLocalAIRelease(ctx context.Context, client localAIHTTPDoer, releaseURL string) (localAIRelease, error) {
	metadata, err := readLocalAIURL(ctx, client, releaseURL, localAIResponseMaxBytes)
	if err != nil {
		return localAIRelease{}, fmt.Errorf("fetch latest LocalAI release: %w", err)
	}
	var release localAIRelease
	if err := json.Unmarshal(metadata, &release); err != nil {
		return localAIRelease{}, fmt.Errorf("decode latest LocalAI release: %w", err)
	}
	if release.Tag == "" || filepath.Base(release.Tag) != release.Tag || strings.ContainsAny(release.Tag, `/\\`) {
		return localAIRelease{}, fmt.Errorf("invalid LocalAI release tag %q", release.Tag)
	}
	return release, nil
}

func selectLocalAIReleaseAssets(release localAIRelease) (localAIReleaseAsset, localAIReleaseAsset, error) {
	assetName := "local-ai-" + release.Tag + "-linux-amd64"
	checksumsName := "LocalAI-" + release.Tag + "-checksums.txt"
	var binary, checksums localAIReleaseAsset
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			binary = asset
		case checksumsName:
			checksums = asset
		}
	}
	if binary.URL == "" || checksums.URL == "" || binary.Size <= 0 || binary.Size > localAIBinaryMaxBytes {
		return localAIReleaseAsset{}, localAIReleaseAsset{}, fmt.Errorf("LocalAI release %q lacks a bounded Linux amd64 binary or checksums asset", release.Tag)
	}
	return binary, checksums, nil
}

func fetchLocalAIAssetDigest(ctx context.Context, client localAIHTTPDoer, checksumsURL, assetName string) (string, error) {
	checksumData, err := readLocalAIURL(ctx, client, checksumsURL, localAIResponseMaxBytes)
	if err != nil {
		return "", fmt.Errorf("fetch LocalAI release checksums: %w", err)
	}
	return localAIAssetDigest(string(checksumData), assetName)
}

func stageLocalAIBinary(ctx context.Context, client localAIHTTPDoer, versionDir, path string, binary localAIReleaseAsset, wantDigest string) (string, error) {
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		return "", fmt.Errorf("create LocalAI binary cache: %w", err)
	}
	staged, err := os.CreateTemp(versionDir, ".local-ai-*")
	if err != nil {
		return "", fmt.Errorf("stage LocalAI binary: %w", err)
	}
	defer os.Remove(staged.Name())
	defer staged.Close()
	if err := writeLocalAIBinary(ctx, client, staged, binary, wantDigest); err != nil {
		return "", err
	}
	if err := staged.Chmod(0o700); err != nil {
		return "", fmt.Errorf("make LocalAI binary executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close staged LocalAI binary: %w", err)
	}
	if err := os.Rename(staged.Name(), path); err != nil {
		if matchesLocalAIFile(path, binary.Size, wantDigest) {
			return path, nil
		}
		return "", fmt.Errorf("install LocalAI binary: %w", err)
	}
	return path, nil
}

func writeLocalAIBinary(ctx context.Context, client localAIHTTPDoer, staged *os.File, binary localAIReleaseAsset, wantDigest string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, binary.URL, nil)
	if err != nil {
		return fmt.Errorf("prepare LocalAI binary download: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download LocalAI binary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download LocalAI binary: HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(response.Body, binary.Size+1))
	if err != nil {
		return fmt.Errorf("write LocalAI binary: %w", err)
	}
	if n != binary.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), wantDigest) {
		return fmt.Errorf("LocalAI binary size or SHA256 does not match release checksums")
	}
	return nil
}

func readLocalAIURL(ctx context.Context, client localAIHTTPDoer, url string, maxBytes int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func localAIAssetDigest(checksums, assetName string) (string, error) {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		digest := fields[0]
		if len(digest) != sha256.Size*2 {
			break
		}
		if _, err := hex.DecodeString(digest); err == nil {
			return digest, nil
		}
		break
	}
	return "", fmt.Errorf("LocalAI release checksums lack SHA256 for %q", assetName)
}

func matchesLocalAIFile(path string, size int64, digest string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return false
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), digest)
}

func provideModelAssetHTTP(edges serviceedges.Edges) modelswire.AssetHTTPDoer {
	if selected := edges.ModelAssetHTTPClient; selected != nil {
		return modelswire.AssetHTTPDoer(selected)
	}
	return newModelAssetHTTPClient()
}

func provideModelAssetMakeDirectories(edges serviceedges.Edges) modelswire.AssetMakeDirectories {
	if selected := edges.ModelAssetMakeDirectories; selected != nil {
		return modelswire.AssetMakeDirectories(selected)
	}
	return os.MkdirAll
}

func provideModelAssetInspectPath(edges serviceedges.Edges) modelswire.AssetInspectPath {
	if selected := edges.ModelAssetInspectPath; selected != nil {
		return modelswire.AssetInspectPath(selected)
	}
	return os.Stat
}

func provideModelAssetResolveHomeDirectory(edges serviceedges.Edges) modelswire.AssetResolveHomeDirectory {
	if selected := edges.ModelAssetResolveHomeDirectory; selected != nil {
		return modelswire.AssetResolveHomeDirectory(selected)
	}
	return os.UserHomeDir
}

func provideModelAssetWriteFile(edges serviceedges.Edges) modelswire.AssetWriteFile {
	if selected := edges.ModelAssetWriteFile; selected != nil {
		return modelswire.AssetWriteFile(selected)
	}
	return os.WriteFile
}

func provideModelAssetRenamePath(edges serviceedges.Edges) modelswire.AssetRenamePath {
	if selected := edges.ModelAssetRenamePath; selected != nil {
		return modelswire.AssetRenamePath(selected)
	}
	return os.Rename
}

func provideModelAssetRemovePath(edges serviceedges.Edges) modelswire.AssetRemovePath {
	if selected := edges.ModelAssetRemovePath; selected != nil {
		return modelswire.AssetRemovePath(selected)
	}
	return os.Remove
}

func provideModelAssetReadFile(edges serviceedges.Edges) modelswire.AssetReadFile {
	if selected := edges.ModelAssetReadFile; selected != nil {
		return modelswire.AssetReadFile(selected)
	}
	return os.ReadFile
}

func provideModelAssetReadDirectory(edges serviceedges.Edges) modelswire.AssetReadDirectory {
	if selected := edges.ModelAssetReadDirectory; selected != nil {
		return modelswire.AssetReadDirectory(selected)
	}
	return os.ReadDir
}

func provideModelAssetCreateFile(edges serviceedges.Edges) modelswire.AssetCreateFile {
	if selected := edges.ModelAssetCreateFile; selected != nil {
		return modelswire.AssetCreateFile(selected)
	}
	return func(path string) (io.WriteCloser, error) { return os.Create(path) }
}

func provideModelAssetOpenFile(edges serviceedges.Edges) modelswire.AssetOpenFile {
	if selected := edges.ModelAssetOpenFile; selected != nil {
		return modelswire.AssetOpenFile(selected)
	}
	return func(path string) (io.ReadCloser, error) { return os.Open(path) }
}

func provideModelAssetResolveEnvironment(edges serviceedges.Edges) modelswire.AssetResolveEnvironment {
	if selected := edges.ModelAssetResolveEnvironment; selected != nil {
		return modelswire.AssetResolveEnvironment(selected)
	}
	return os.Getenv
}

func provideModelAssetEndpoints(edges serviceedges.Edges) models.RuntimeAssetEndpoints {
	return modelswire.NormalizeAssetEndpoints(edges.ModelAssetEndpoints)
}

func provideModelAssetCoordination(edges serviceedges.Edges) (modelswire.AssetStagingCoordination, error) {

	var coordination modelswire.AssetStagingCoordination
	var err error
	if factory := edges.ModelAssetStagingCoordinationFactory; factory != nil {
		coordination, err = factory()
	} else {
		coordination, err = platformlocking.New(platformlocking.LocalFileSystem{})
	}
	if err != nil {
		return nil, fmt.Errorf("construct Models asset staging coordination: %w", err)
	}
	return coordination, nil
}

func provideModelRuntimeTempFile(edges serviceedges.Edges) modelswire.RuntimeCreateTempFile {

	if selected := edges.ModelRuntimeCreateTempFile; selected != nil {
		return adaptModelRuntimeTempFile(selected)
	}
	return func(dir, pattern string) (modelswire.RuntimeTempFile, error) { return os.CreateTemp(dir, pattern) }
}
