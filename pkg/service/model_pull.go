package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	modelPullOutcomePulled         = "PULLED"
	modelPullOutcomeAlreadyPresent = "ALREADY_PRESENT"
	defaultModelAssetBaseURL       = "https://huggingface.co"
	defaultModelAssetAPIBaseURL    = "https://huggingface.co/api"
)

type modelAssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (apisurface.ModelPullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) error
	ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) (localModelCacheLayout, error)
}

type huggingFaceModelAssetPuller struct {
	cacheDir   string
	baseURL    string
	apiBaseURL string
	client     *http.Client
	goos       string
	goarch     string
}

type modelAssetSpec struct {
	ModelName         string
	AllowedOSArch     map[string]struct{}
	Repository        string
	RequiredFilenames []string
	ProviderLocality  string
}

type modelAssetManifest struct {
	Revision string
	Files    []modelAssetRemoteFile
}

type modelAssetRemoteFile struct {
	Path   string
	Bytes  int64
	SHA256 string
	URL    string
}

type huggingFaceModelResponse struct {
	Sha      string                    `json:"sha"`
	Siblings []huggingFaceModelSibling `json:"siblings"`
}

type huggingFaceModelSibling struct {
	Rfilename string               `json:"rfilename"`
	Size      int64                `json:"size"`
	LFS       *huggingFaceModelLFS `json:"lfs"`
}

type huggingFaceModelLFS struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

func (fs *FactoryService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	models := buildModelCatalog(runtimeCfg)
	key := canonicalModelName(modelName)
	if key == "" {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := models[key]
	if !ok {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	if entry.summary.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: model %q is not a local model", apisurface.ErrModelPullUnsupported, modelName)
	}
	return fs.modelAssetPuller().PullModel(ctx, runtimeCfg, modelName)
}

func (fs *FactoryService) modelAssetPuller() modelAssetPuller {
	if fs != nil && fs.modelAssets != nil {
		return fs.modelAssets
	}
	cacheDir := ""
	if fs != nil && fs.cfg != nil {
		cacheDir = strings.TrimSpace(fs.cfg.ModelCacheDir)
	}
	puller := newHuggingFaceModelAssetPuller(cacheDir)
	if fs != nil {
		fs.modelAssets = puller
	}
	return puller
}

func newHuggingFaceModelAssetPuller(cacheDir string) *huggingFaceModelAssetPuller {
	return &huggingFaceModelAssetPuller{
		cacheDir:   strings.TrimSpace(cacheDir),
		baseURL:    defaultModelAssetBaseURL,
		apiBaseURL: defaultModelAssetAPIBaseURL,
		client:     &http.Client{},
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	}
}

func (p *huggingFaceModelAssetPuller) PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (apisurface.ModelPullResult, error) {
	spec, err := p.resolveSpec(runtimeCfg, modelName)
	if err != nil {
		return apisurface.ModelPullResult{}, err
	}
	manifest, err := p.fetchManifest(ctx, spec)
	if err != nil {
		return apisurface.ModelPullResult{}, err
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return apisurface.ModelPullResult{}, err
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("prepare model cache %q: %w", cachePath, err)
	}

	outcome := modelPullOutcomeAlreadyPresent
	downloaded := make([]apisurface.ModelPullDownloadedFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		current, fileOutcome, err := p.ensureCachedFile(ctx, cachePath, file)
		if err != nil {
			return apisurface.ModelPullResult{}, err
		}
		if fileOutcome == modelPullOutcomePulled {
			outcome = modelPullOutcomePulled
		}
		downloaded = append(downloaded, current)
	}
	sort.Slice(downloaded, func(i, j int) bool {
		return downloaded[i].Path < downloaded[j].Path
	})

	return apisurface.ModelPullResult{
		ModelName:        spec.ModelName,
		ProviderLocality: spec.ProviderLocality,
		Outcome:          outcome,
		CachePath:        cachePath,
		Revision:         manifest.Revision,
		DownloadedFiles:  downloaded,
	}, nil
}

func (p *huggingFaceModelAssetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) error {
	if worker == nil || strings.TrimSpace(worker.ModelLocality) != interfaces.ModelLocalityLocal {
		return nil
	}
	spec, err := p.resolveSpec(runtimeCfg, worker.Model)
	if err != nil {
		if errors.Is(err, apisurface.ErrModelPullUnsupported) {
			return fmt.Errorf("%w: %s", apisurface.ErrModelNotAvailable, strings.TrimSpace(worker.Model))
		}
		return err
	}
	manifest, err := p.fetchManifest(ctx, spec)
	if err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrModelNotAvailable, err)
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, file := range manifest.Files {
		target := filepath.Join(cachePath, filepath.FromSlash(file.Path))
		info, statErr := os.Stat(target)
		switch {
		case statErr == nil && !info.IsDir():
		case errors.Is(statErr, os.ErrNotExist):
			missing = append(missing, file.Path)
		case statErr != nil:
			return fmt.Errorf("inspect local model cache %q: %w", target, statErr)
		default:
			missing = append(missing, file.Path)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: required assets missing in managed cache %q (%s)", apisurface.ErrModelNotAvailable, cachePath, strings.Join(missing, ", "))
	}
	return nil
}

func (p *huggingFaceModelAssetPuller) ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) (localModelCacheLayout, error) {
	if worker == nil {
		return localModelCacheLayout{}, fmt.Errorf("local model worker is required")
	}
	if err := p.EnsureModelAvailable(ctx, runtimeCfg, worker); err != nil {
		return localModelCacheLayout{}, err
	}
	spec, err := p.resolveSpec(runtimeCfg, worker.Model)
	if err != nil {
		return localModelCacheLayout{}, err
	}
	manifest, err := p.fetchManifest(ctx, spec)
	if err != nil {
		return localModelCacheLayout{}, err
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return localModelCacheLayout{}, err
	}
	files := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		files = append(files, filepath.Join(cachePath, filepath.FromSlash(file.Path)))
	}
	return localModelCacheLayout{
		ModelName: spec.ModelName,
		CachePath: cachePath,
		Revision:  manifest.Revision,
		Files:     files,
	}, nil
}

func (p *huggingFaceModelAssetPuller) resolveSpec(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (modelAssetSpec, error) {
	key := canonicalModelName(modelName)
	spec, ok := builtInModelAssetSpecs()[key]
	if !ok {
		return modelAssetSpec{}, fmt.Errorf("%w: no managed asset source for model %q", apisurface.ErrModelPullUnsupported, modelName)
	}
	if _, ok := spec.AllowedOSArch[p.goos+"/"+p.goarch]; !ok {
		return modelAssetSpec{}, fmt.Errorf("%w: model %q is not supported on %s/%s", apisurface.ErrModelPullUnsupported, modelName, p.goos, p.goarch)
	}
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return modelAssetSpec{}, fmt.Errorf("runtime config is not available")
	}
	for _, resource := range runtimeCfg.FactoryConfig().Resources {
		if strings.TrimSpace(resource.Type) != interfaces.ResourceTypeModel {
			continue
		}
		if canonicalModelName(resource.Model) == key {
			return spec, nil
		}
	}
	return modelAssetSpec{}, fmt.Errorf("%w: model %q has no matching MODEL resource declaration", apisurface.ErrModelPullUnsupported, modelName)
}

func builtInModelAssetSpecs() map[string]modelAssetSpec {
	return map[string]modelAssetSpec{
		canonicalModelName("OMNIVOICE_Q4_K_M"): {
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Repository:       "Serveurperso/OmniVoice-GGUF",
			RequiredFilenames: []string{
				"omnivoice-base-Q4_K_M.gguf",
				"omnivoice-tokenizer-Q4_K_M.gguf",
			},
			AllowedOSArch: map[string]struct{}{
				"darwin/arm64":  {},
				"darwin/amd64":  {},
				"linux/amd64":   {},
				"linux/arm64":   {},
				"windows/amd64": {},
				"windows/arm64": {},
			},
		},
	}
}

func (p *huggingFaceModelAssetPuller) fetchManifest(ctx context.Context, spec modelAssetSpec) (modelAssetManifest, error) {
	apiURL := strings.TrimRight(p.apiBaseURL, "/") + "/models/" + spec.Repository
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return modelAssetManifest{}, fmt.Errorf("build model-manifest request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return modelAssetManifest{}, fmt.Errorf("pull model manifest for %q: %w", spec.ModelName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return modelAssetManifest{}, fmt.Errorf("pull model manifest for %q failed (%d): %s", spec.ModelName, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload huggingFaceModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return modelAssetManifest{}, fmt.Errorf("decode model manifest for %q: %w", spec.ModelName, err)
	}
	revision := strings.TrimSpace(payload.Sha)
	if revision == "" {
		revision = "main"
	}

	files := make([]modelAssetRemoteFile, 0, len(spec.RequiredFilenames))
	siblingByPath := make(map[string]huggingFaceModelSibling, len(payload.Siblings))
	for _, sibling := range payload.Siblings {
		siblingByPath[strings.TrimSpace(sibling.Rfilename)] = sibling
	}
	for _, filename := range spec.RequiredFilenames {
		sibling, ok := siblingByPath[filename]
		if !ok {
			return modelAssetManifest{}, fmt.Errorf("pull model manifest for %q is missing required file %q", spec.ModelName, filename)
		}
		size := sibling.Size
		sha := ""
		if sibling.LFS != nil {
			if sibling.LFS.Size > 0 {
				size = sibling.LFS.Size
			}
			sha = strings.ToLower(strings.TrimSpace(sibling.LFS.Oid))
		}
		files = append(files, modelAssetRemoteFile{
			Path:   filename,
			Bytes:  size,
			SHA256: sha,
			URL: strings.TrimRight(p.baseURL, "/") + "/" + spec.Repository + "/resolve/" +
				url.PathEscape(revision) + "/" + strings.ReplaceAll(filename, " ", "%20") + "?download=true",
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return modelAssetManifest{Revision: revision, Files: files}, nil
}

func (p *huggingFaceModelAssetPuller) cachePath(spec modelAssetSpec, revision string) (string, error) {
	root := strings.TrimSpace(p.cacheDir)
	if root == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve managed model cache directory: %w", err)
		}
		root = filepath.Join(homeDir, ".agent-factory", "models")
	}
	return filepath.Join(root, spec.ModelName, revision), nil
}

func (p *huggingFaceModelAssetPuller) ensureCachedFile(ctx context.Context, cachePath string, remote modelAssetRemoteFile) (apisurface.ModelPullDownloadedFile, string, error) {
	targetPath := filepath.Join(cachePath, filepath.FromSlash(remote.Path))
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		if remote.SHA256 == "" {
			return apisurface.ModelPullDownloadedFile{Path: remote.Path, Bytes: info.Size()}, modelPullOutcomeAlreadyPresent, nil
		}
		existingSHA, shaErr := fileSHA256(targetPath)
		if shaErr != nil {
			return apisurface.ModelPullDownloadedFile{}, "", fmt.Errorf("checksum cached model file %q: %w", targetPath, shaErr)
		}
		if existingSHA == remote.SHA256 {
			return apisurface.ModelPullDownloadedFile{Path: remote.Path, Bytes: info.Size(), SHA256: existingSHA}, modelPullOutcomeAlreadyPresent, nil
		}
		if removeErr := os.Remove(targetPath); removeErr != nil {
			return apisurface.ModelPullDownloadedFile{}, "", fmt.Errorf("remove stale cached model file %q: %w", targetPath, removeErr)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return apisurface.ModelPullDownloadedFile{}, "", fmt.Errorf("inspect model cache file %q: %w", targetPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return apisurface.ModelPullDownloadedFile{}, "", fmt.Errorf("prepare model cache directory for %q: %w", targetPath, err)
	}
	if err := p.downloadFile(ctx, remote, targetPath); err != nil {
		return apisurface.ModelPullDownloadedFile{}, "", err
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return apisurface.ModelPullDownloadedFile{}, "", fmt.Errorf("stat downloaded model file %q: %w", targetPath, err)
	}
	return apisurface.ModelPullDownloadedFile{Path: remote.Path, Bytes: info.Size(), SHA256: remote.SHA256}, modelPullOutcomePulled, nil
}

func (p *huggingFaceModelAssetPuller) downloadFile(ctx context.Context, remote modelAssetRemoteFile, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL, nil)
	if err != nil {
		return fmt.Errorf("build model download request for %q: %w", remote.Path, err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("download model asset %q: %w", remote.Path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download model asset %q failed (%d): %s", remote.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmpPath := targetPath + ".partial"
	output, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("write model asset %q: %w", remote.Path, err)
	}
	hasher := sha256.New()
	writer := io.MultiWriter(output, hasher)
	_, copyErr := io.Copy(writer, resp.Body)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write model asset %q: %w", remote.Path, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize model asset %q: %w", remote.Path, closeErr)
	}
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if remote.SHA256 != "" && !strings.EqualFold(gotSHA, remote.SHA256) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download model asset %q failed checksum verification: expected %s, got %s", remote.Path, remote.SHA256, gotSHA)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("commit model asset %q into managed cache: %w", remote.Path, err)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
