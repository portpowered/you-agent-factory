package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	PullOutcomePulled           = "PULLED"
	PullOutcomeAlreadyPresent   = "ALREADY_PRESENT"
	defaultModelAssetBaseURL    = "https://huggingface.co"
	defaultModelAssetAPIBaseURL = "https://huggingface.co/api"
	modelAssetMetadataFileName  = ".managed-cache.json"
	modelAssetMaxAttempts       = 3
)

type CacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

// RuntimeCacheInspection reports local managed-cache state without contacting
// upstream asset sources.
type RuntimeCacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
}

// HTTPDoer is the exact external HTTP effect used to retrieve managed-model
// manifests and assets.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Endpoints identifies the external managed-model asset source.
type Endpoints struct {
	BaseURL    string
	APIBaseURL string
}

type MakeDirectories func(string, os.FileMode) error
type InspectPath func(string) (os.FileInfo, error)
type ResolveHomeDirectory func() (string, error)
type WriteFile func(string, []byte, os.FileMode) error
type RenamePath func(string, string) error
type RemovePath func(string) error
type ReadFile func(string) ([]byte, error)
type ReadDirectory func(string) ([]os.DirEntry, error)
type CreateFile func(string) (io.WriteCloser, error)
type OpenFile func(string) (io.ReadCloser, error)

type Puller struct {
	cacheDir        string
	baseURL         string
	apiBaseURL      string
	client          HTTPDoer
	makeDirectories MakeDirectories
	inspectPath     InspectPath
	resolveHome     ResolveHomeDirectory
	writeFile       WriteFile
	renamePath      RenamePath
	removePath      RemovePath
	readFile        ReadFile
	readDirectory   ReadDirectory
	createFile      CreateFile
	openFile        OpenFile
	goos            string
	goarch          string
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

type localModelCacheMetadata struct {
	ModelName string                `json:"modelName"`
	Revision  string                `json:"revision"`
	Files     []localModelCacheFile `json:"files"`
}

type localModelCacheFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
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

func DefaultEndpoints() Endpoints {
	return Endpoints{BaseURL: defaultModelAssetBaseURL, APIBaseURL: defaultModelAssetAPIBaseURL}
}

// NewPuller constructs an inert puller with exact network and source
// boundaries supplied by the composition root.
func NewPuller(
	cacheDir string,
	goos string,
	goarch string,
	client HTTPDoer,
	endpoints Endpoints,
	makeDirectories MakeDirectories,
	inspectPath InspectPath,
	resolveHome ResolveHomeDirectory,
	writeFile WriteFile,
	renamePath RenamePath,
	removePath RemovePath,
	readFile ReadFile,
	readDirectory ReadDirectory,
	createFile CreateFile,
	openFile OpenFile,
) (*Puller, error) {
	if client == nil {
		return nil, fmt.Errorf("construct model asset puller: HTTP client is required")
	}
	if makeDirectories == nil || inspectPath == nil || resolveHome == nil ||
		writeFile == nil || renamePath == nil || removePath == nil || readFile == nil ||
		readDirectory == nil || createFile == nil || openFile == nil {
		return nil, fmt.Errorf("construct model asset puller: all cache operations are required")
	}
	baseURL := strings.TrimSpace(endpoints.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("construct model asset puller: asset base URL is required")
	}
	apiBaseURL := strings.TrimSpace(endpoints.APIBaseURL)
	if apiBaseURL == "" {
		return nil, fmt.Errorf("construct model asset puller: asset API base URL is required")
	}
	return &Puller{
		cacheDir:        strings.TrimSpace(cacheDir),
		baseURL:         baseURL,
		apiBaseURL:      apiBaseURL,
		client:          client,
		makeDirectories: makeDirectories, inspectPath: inspectPath, resolveHome: resolveHome,
		writeFile: writeFile, renamePath: renamePath, removePath: removePath,
		readFile: readFile, readDirectory: readDirectory, createFile: createFile, openFile: openFile,
		goos:   strings.TrimSpace(goos),
		goarch: strings.TrimSpace(goarch),
	}, nil
}

func (p *Puller) SetEndpointsForTest(baseURL string, apiBaseURL string, client HTTPDoer) {
	p.baseURL = strings.TrimSpace(baseURL)
	p.apiBaseURL = strings.TrimSpace(apiBaseURL)
	p.client = client
}

func (p *Puller) PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (PullResult, error) {
	spec, err := p.resolveSpec(runtimeCfg, modelName)
	if err != nil {
		return PullResult{}, err
	}
	manifest, err := p.fetchManifest(ctx, spec)
	if err != nil {
		return PullResult{}, err
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return PullResult{}, err
	}
	if err := p.makeDirectories(cachePath, 0o755); err != nil {
		return PullResult{}, fmt.Errorf("prepare model cache %q: %w", cachePath, err)
	}

	outcome := PullOutcomeAlreadyPresent
	downloaded := make([]DownloadedFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		current, fileOutcome, err := p.ensureCachedFile(ctx, cachePath, file)
		if err != nil {
			return PullResult{}, err
		}
		if fileOutcome == PullOutcomePulled {
			outcome = PullOutcomePulled
		}
		downloaded = append(downloaded, current)
	}
	sort.Slice(downloaded, func(i, j int) bool {
		return downloaded[i].Path < downloaded[j].Path
	})
	if err := p.writeLocalMetadata(spec, manifest); err != nil {
		return PullResult{}, err
	}

	return PullResult{
		ModelName:        spec.ModelName,
		ProviderLocality: spec.ProviderLocality,
		Outcome:          outcome,
		CachePath:        cachePath,
		Revision:         manifest.Revision,
		DownloadedFiles:  downloaded,
	}, nil
}

func (p *Puller) EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error {
	_, err := p.resolveModelCacheLayout(ctx, runtimeCfg, worker)
	return err
}

func (p *Puller) ResolveModelCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error) {
	return p.resolveModelCacheLayout(ctx, runtimeCfg, worker)
}

// InspectRuntimeCache inspects the local managed cache without contacting upstream sources.
func (p *Puller) InspectRuntimeCache(_ context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error) {
	spec, err := p.resolveSpec(runtimeCfg, modelName)
	if err != nil {
		if errors.Is(err, ErrPullUnsupported) {
			return RuntimeCacheInspection{}, nil
		}
		return RuntimeCacheInspection{}, err
	}
	result := RuntimeCacheInspection{
		Supported:     true,
		MissingAssets: append([]string(nil), spec.RequiredFilenames...),
	}
	manifest, found, err := p.readLocalMetadata(spec)
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	if !found {
		manifest, found, err = p.discoverLocalManifest(spec)
		if err != nil {
			return RuntimeCacheInspection{}, err
		}
	}
	if !found {
		return result, nil
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	result.CachePath = cachePath
	result.Revision = manifest.Revision
	installed := make([]string, 0, len(spec.RequiredFilenames))
	missing := make([]string, 0)
	for _, required := range spec.RequiredFilenames {
		target := filepath.Join(cachePath, filepath.FromSlash(required))
		info, statErr := p.inspectPath(target)
		if statErr == nil && !info.IsDir() {
			installed = append(installed, required)
			continue
		}
		missing = append(missing, required)
	}
	result.InstalledFileCount = len(installed)
	result.MissingAssets = missing
	result.Installed = len(missing) == 0
	if !result.Installed {
		result.PartialArtifacts = p.hasPartialArtifacts(spec, result.CachePath)
	}
	return result, nil
}

func (p *Puller) resolveModelCacheLayout(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error) {
	if worker == nil || strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
		return CacheLayout{}, nil
	}
	spec, err := p.resolveSpec(runtimeCfg, worker.Model)
	if err != nil {
		if errors.Is(err, ErrPullUnsupported) {
			return CacheLayout{}, fmt.Errorf("%w: %s", ErrNotAvailable, strings.TrimSpace(worker.Model))
		}
		return CacheLayout{}, err
	}
	manifest, err := p.resolveManifest(ctx, spec)
	if err != nil {
		return CacheLayout{}, fmt.Errorf("%w: %v", ErrNotAvailable, err)
	}
	cachePath, err := p.cachePath(spec, manifest.Revision)
	if err != nil {
		return CacheLayout{}, err
	}
	files := make([]string, 0, len(manifest.Files))
	missing := make([]string, 0)
	for _, file := range manifest.Files {
		target := filepath.Join(cachePath, filepath.FromSlash(file.Path))
		info, statErr := p.inspectPath(target)
		switch {
		case statErr == nil && !info.IsDir():
			files = append(files, target)
		case errors.Is(statErr, os.ErrNotExist):
			missing = append(missing, file.Path)
		case statErr != nil:
			return CacheLayout{}, fmt.Errorf("inspect local model cache %q: %w", target, statErr)
		default:
			missing = append(missing, file.Path)
		}
	}
	if len(missing) > 0 {
		return CacheLayout{}, fmt.Errorf("%w: required assets missing in managed cache %q (%s)", ErrNotAvailable, cachePath, strings.Join(missing, ", "))
	}
	return CacheLayout{
		ModelName: spec.ModelName,
		CachePath: cachePath,
		Revision:  manifest.Revision,
		Files:     files,
	}, nil
}

func (p *Puller) resolveManifest(ctx context.Context, spec modelAssetSpec) (modelAssetManifest, error) {
	if manifest, ok, err := p.readLocalMetadata(spec); err != nil {
		return modelAssetManifest{}, err
	} else if ok {
		return manifest, nil
	}
	if manifest, ok, err := p.discoverLocalManifest(spec); err != nil {
		return modelAssetManifest{}, err
	} else if ok {
		return manifest, nil
	}
	return p.fetchManifest(ctx, spec)
}

func (p *Puller) resolveSpec(runtimeCfg *models.RuntimeConfig, modelName string) (modelAssetSpec, error) {
	key := canonicalModelName(modelName)
	spec, ok := builtInModelAssetSpecs()[key]
	if !ok {
		return modelAssetSpec{}, fmt.Errorf("%w: no managed asset source for model %q", ErrPullUnsupported, modelName)
	}
	if _, ok := spec.AllowedOSArch[p.goos+"/"+p.goarch]; !ok {
		return modelAssetSpec{}, fmt.Errorf("%w: model %q is not supported on %s/%s", ErrPullUnsupported, modelName, p.goos, p.goarch)
	}
	if runtimeCfg == nil {
		return modelAssetSpec{}, fmt.Errorf("runtime config is not available")
	}
	for _, resource := range runtimeCfg.Resources {
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
			continue
		}
		if canonicalModelName(resource.Model) == key {
			return spec, nil
		}
	}
	return modelAssetSpec{}, fmt.Errorf("%w: model %q has no matching MODEL resource declaration", ErrPullUnsupported, modelName)
}

func builtInModelAssetSpecs() map[string]modelAssetSpec {
	return map[string]modelAssetSpec{
		canonicalModelName("OMNIVOICE_Q4_K_M"): {
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: models.RuntimeModelLocalityLocal,
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

func canonicalModelName(model string) string {
	return strings.ToUpper(strings.TrimSpace(model))
}

func (p *Puller) fetchManifest(ctx context.Context, spec modelAssetSpec) (modelAssetManifest, error) {
	apiURL := strings.TrimRight(p.apiBaseURL, "/") + "/models/" + spec.Repository
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return modelAssetManifest{}, fmt.Errorf("build model-manifest request: %w", err)
	}
	resp, err := p.doWithRetry(req, shouldRetryModelAssetResponse)
	if err != nil {
		return modelAssetManifest{}, fmt.Errorf("%w: pull model manifest for %q: %v", ErrSourceFetchFailed, spec.ModelName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return modelAssetManifest{}, fmt.Errorf("%w: pull model manifest for %q failed (%d): %s", ErrSourceFetchFailed, spec.ModelName, resp.StatusCode, strings.TrimSpace(string(body)))
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
			return modelAssetManifest{}, fmt.Errorf("%w: pull model manifest for %q is missing required file %q", ErrSourceFetchFailed, spec.ModelName, filename)
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

func (p *Puller) cachePath(spec modelAssetSpec, revision string) (string, error) {
	root, err := p.modelCacheRoot(spec)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, revision), nil
}

func (p *Puller) modelCacheRoot(spec modelAssetSpec) (string, error) {
	root := strings.TrimSpace(p.cacheDir)
	if root == "" {
		homeDir, err := p.resolveHome()
		if err != nil {
			return "", fmt.Errorf("resolve managed model cache directory: %w", err)
		}
		root = filepath.Join(homeDir, ".agent-factory", "models")
	}
	return filepath.Join(root, spec.ModelName), nil
}

func (p *Puller) metadataPath(spec modelAssetSpec) (string, error) {
	root, err := p.modelCacheRoot(spec)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, modelAssetMetadataFileName), nil
}

func (p *Puller) writeLocalMetadata(spec modelAssetSpec, manifest modelAssetManifest) error {
	metadataPath, err := p.metadataPath(spec)
	if err != nil {
		return err
	}
	if err := p.makeDirectories(filepath.Dir(metadataPath), 0o755); err != nil {
		return fmt.Errorf("prepare model metadata directory for %q: %w", spec.ModelName, err)
	}
	metadata := localModelCacheMetadata{
		ModelName: spec.ModelName,
		Revision:  manifest.Revision,
		Files:     make([]localModelCacheFile, 0, len(manifest.Files)),
	}
	for _, file := range manifest.Files {
		metadata.Files = append(metadata.Files, localModelCacheFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode managed-cache metadata for %q: %w", spec.ModelName, err)
	}
	tmpPath := metadataPath + ".partial"
	if err := p.writeFile(tmpPath, body, 0o644); err != nil {
		return fmt.Errorf("write managed-cache metadata for %q: %w", spec.ModelName, err)
	}
	if err := p.renamePath(tmpPath, metadataPath); err != nil {
		_ = p.removePath(tmpPath)
		return fmt.Errorf("commit managed-cache metadata for %q: %w", spec.ModelName, err)
	}
	return nil
}

func (p *Puller) readLocalMetadata(spec modelAssetSpec) (modelAssetManifest, bool, error) {
	metadataPath, err := p.metadataPath(spec)
	if err != nil {
		return modelAssetManifest{}, false, err
	}
	body, err := p.readFile(metadataPath)
	if errors.Is(err, os.ErrNotExist) {
		return modelAssetManifest{}, false, nil
	}
	if err != nil {
		return modelAssetManifest{}, false, fmt.Errorf("read managed-cache metadata for %q: %w", spec.ModelName, err)
	}
	var metadata localModelCacheMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return modelAssetManifest{}, false, fmt.Errorf("decode managed-cache metadata for %q: %w", spec.ModelName, err)
	}
	if strings.TrimSpace(metadata.Revision) == "" {
		return modelAssetManifest{}, false, nil
	}
	manifest := modelAssetManifest{
		Revision: metadata.Revision,
		Files:    make([]modelAssetRemoteFile, 0, len(metadata.Files)),
	}
	for _, file := range metadata.Files {
		manifest.Files = append(manifest.Files, modelAssetRemoteFile{
			Path:   file.Path,
			Bytes:  file.Bytes,
			SHA256: file.SHA256,
		})
	}
	if !manifestMatchesRequiredFiles(manifest, spec.RequiredFilenames) {
		return modelAssetManifest{}, false, nil
	}
	return manifest, true, nil
}

func (p *Puller) discoverLocalManifest(spec modelAssetSpec) (modelAssetManifest, bool, error) {
	root, err := p.modelCacheRoot(spec)
	if err != nil {
		return modelAssetManifest{}, false, err
	}
	entries, err := p.readDirectory(root)
	if errors.Is(err, os.ErrNotExist) {
		return modelAssetManifest{}, false, nil
	}
	if err != nil {
		return modelAssetManifest{}, false, fmt.Errorf("read managed model cache root for %q: %w", spec.ModelName, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		revision := strings.TrimSpace(entry.Name())
		if revision == "" {
			continue
		}
		manifest := modelAssetManifest{
			Revision: revision,
			Files:    make([]modelAssetRemoteFile, 0, len(spec.RequiredFilenames)),
		}
		allPresent := true
		for _, required := range spec.RequiredFilenames {
			target := filepath.Join(root, revision, filepath.FromSlash(required))
			info, statErr := p.inspectPath(target)
			if statErr != nil || info.IsDir() {
				allPresent = false
				break
			}
			manifest.Files = append(manifest.Files, modelAssetRemoteFile{
				Path:  required,
				Bytes: info.Size(),
			})
		}
		if allPresent {
			return manifest, true, nil
		}
	}
	return modelAssetManifest{}, false, nil
}

func manifestMatchesRequiredFiles(manifest modelAssetManifest, required []string) bool {
	if strings.TrimSpace(manifest.Revision) == "" || len(manifest.Files) == 0 {
		return false
	}
	fileSet := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		fileSet[file.Path] = struct{}{}
	}
	for _, name := range required {
		if _, ok := fileSet[name]; !ok {
			return false
		}
	}
	return true
}

func (p *Puller) ensureCachedFile(ctx context.Context, cachePath string, remote modelAssetRemoteFile) (DownloadedFile, string, error) {
	targetPath := filepath.Join(cachePath, filepath.FromSlash(remote.Path))
	if info, err := p.inspectPath(targetPath); err == nil && !info.IsDir() {
		if remote.SHA256 == "" {
			return DownloadedFile{Path: remote.Path, Bytes: info.Size()}, PullOutcomeAlreadyPresent, nil
		}
		existingSHA, shaErr := p.fileSHA256(targetPath)
		if shaErr != nil {
			return DownloadedFile{}, "", fmt.Errorf("checksum cached model file %q: %w", targetPath, shaErr)
		}
		if existingSHA == remote.SHA256 {
			return DownloadedFile{Path: remote.Path, Bytes: info.Size(), SHA256: existingSHA}, PullOutcomeAlreadyPresent, nil
		}
		if removeErr := p.removePath(targetPath); removeErr != nil {
			return DownloadedFile{}, "", fmt.Errorf("remove stale cached model file %q: %w", targetPath, removeErr)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return DownloadedFile{}, "", fmt.Errorf("inspect model cache file %q: %w", targetPath, err)
	}

	if err := p.makeDirectories(filepath.Dir(targetPath), 0o755); err != nil {
		return DownloadedFile{}, "", fmt.Errorf("prepare model cache directory for %q: %w", targetPath, err)
	}
	if err := p.downloadFile(ctx, remote, targetPath); err != nil {
		return DownloadedFile{}, "", err
	}
	info, err := p.inspectPath(targetPath)
	if err != nil {
		return DownloadedFile{}, "", fmt.Errorf("stat downloaded model file %q: %w", targetPath, err)
	}
	return DownloadedFile{Path: remote.Path, Bytes: info.Size(), SHA256: remote.SHA256}, PullOutcomePulled, nil
}

func (p *Puller) downloadFile(ctx context.Context, remote modelAssetRemoteFile, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL, nil)
	if err != nil {
		return fmt.Errorf("build model download request for %q: %w", remote.Path, err)
	}
	resp, err := p.doWithRetry(req, shouldRetryModelAssetResponse)
	if err != nil {
		return fmt.Errorf("%w: download model asset %q: %v", ErrSourceFetchFailed, remote.Path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: download model asset %q failed (%d): %s", ErrSourceFetchFailed, remote.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmpPath := targetPath + ".partial"
	output, err := p.createFile(tmpPath)
	if err != nil {
		return fmt.Errorf("write model asset %q: %w", remote.Path, err)
	}
	hasher := sha256.New()
	writer := io.MultiWriter(output, hasher)
	_, copyErr := io.Copy(writer, resp.Body)
	closeErr := output.Close()
	if copyErr != nil {
		_ = p.removePath(tmpPath)
		return fmt.Errorf("write model asset %q: %w", remote.Path, copyErr)
	}
	if closeErr != nil {
		_ = p.removePath(tmpPath)
		return fmt.Errorf("finalize model asset %q: %w", remote.Path, closeErr)
	}
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if remote.SHA256 != "" && !strings.EqualFold(gotSHA, remote.SHA256) {
		_ = p.removePath(tmpPath)
		return fmt.Errorf("%w: download model asset %q failed checksum verification: expected %s, got %s", ErrSourceFetchFailed, remote.Path, remote.SHA256, gotSHA)
	}
	if err := p.renamePath(tmpPath, targetPath); err != nil {
		_ = p.removePath(tmpPath)
		return fmt.Errorf("commit model asset %q into managed cache: %w", remote.Path, err)
	}
	return nil
}

func (p *Puller) doWithRetry(req *http.Request, shouldRetryResponse func(*http.Response) bool) (*http.Response, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("model asset HTTP client is not configured")
	}
	attempts := modelAssetMaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := p.client.Do(req.Clone(req.Context()))
		if err == nil {
			if shouldRetryResponse(resp) && attempt < attempts {
				lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
				resp.Body.Close()
				continue
			}
			return resp, nil
		}
		lastErr = err
		if attempt == attempts || !shouldRetryModelAssetError(err) {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("model asset request failed")
	}
	return nil, lastErr
}

func shouldRetryModelAssetResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode >= http.StatusInternalServerError
}

func shouldRetryModelAssetError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	return false
}

func (p *Puller) hasPartialArtifacts(spec modelAssetSpec, cachePath string) bool {
	metadataPath, err := p.metadataPath(spec)
	if err == nil {
		if _, statErr := p.inspectPath(metadataPath + ".partial"); statErr == nil {
			return true
		}
	}
	root, err := p.modelCacheRoot(spec)
	if err != nil {
		return false
	}
	dirs := []string{root}
	if strings.TrimSpace(cachePath) != "" {
		dirs = append(dirs, cachePath)
	}
	for _, dir := range dirs {
		if p.dirHasPartialArtifacts(dir) {
			return true
		}
	}
	return false
}

func (p *Puller) dirHasPartialArtifacts(dir string) bool {
	entries, err := p.readDirectory(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".partial") {
			return true
		}
		if entry.IsDir() {
			if p.dirHasPartialArtifacts(filepath.Join(dir, name)) {
				return true
			}
		}
	}
	return false
}

func (p *Puller) fileSHA256(path string) (string, error) {
	input, err := p.openFile(path)
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
