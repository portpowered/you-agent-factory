package service

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
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
)

const assetSourceMaxAttempts = 3

type remoteManifest struct {
	revision string
	files    []remoteFile
}

type remoteFile struct {
	modelName  string
	repository string
	revision   string
	path       string
	bytes      int64
	sha256     string
	url        string
}

type upstreamModel struct {
	SHA      string            `json:"sha"`
	Siblings []upstreamSibling `json:"siblings"`
}

type upstreamSibling struct {
	Path string       `json:"rfilename"`
	Size int64        `json:"size"`
	LFS  *upstreamLFS `json:"lfs"`
}

type upstreamLFS struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

func (s *service) PrepareModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (result models.PrepareModelAssetsResult, resultErr error) {
	if err := request.Validate(); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	expected := append([]models.AssetRequirement(nil), request.Artifacts...)
	expected = append(expected, request.BackendArtifacts...)
	s.beginActivePull(request.Scope, request.Name, expected)
	defer func() {
		s.finishActivePull(request.Scope, request.Name, resultErr)
	}()
	result, resultErr = s.prepareModelAssets(ctx, request)
	if resultErr != nil && !errors.Is(resultErr, context.Canceled) &&
		!errors.Is(resultErr, context.DeadlineExceeded) {
		stage := pullsupport.PullStageForError(resultErr)
		if stage == "" {
			stage = models.PullStageAssembly
		}
		resultErr = pullsupport.WrapPullStage(
			stage, request.Name, "prepare model assets", "", resultErr,
		)
	}
	return result, resultErr
}

func (s *service) prepareModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	if shouldPrepareGenericAssets(request) {
		return s.prepareGenericAssets(ctx, request)
	}
	if err := request.Validate(); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	scope, err := s.resolveScope(ctx, request.Scope)
	if err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	spec, source, err := s.resolveSource(scope.Runtime, request.Name)
	if err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	s.updateActivePull(request.Scope, request.Name, assetRequirementsForSpec(spec), "")
	if err := assetContextError(ctx); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}

	if snapshot, available, inspectErr := s.inspectVerifiedCache(
		ctx, scope.CacheDirectory, spec, source,
	); inspectErr != nil {
		return models.PrepareModelAssetsResult{Asset: snapshot.Clone()}, inspectErr
	} else if available && snapshot.Integrity == models.AssetIntegrityVerified {
		return models.PrepareModelAssetsResult{
			Asset: snapshot.Clone(), Outcome: models.AssetPreparationAlreadyAvailable,
		}, nil
	}

	manifest, err := s.fetchManifest(ctx, spec)
	if err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	s.updateActivePull(request.Scope, request.Name, requirementsFromManifest(manifest), manifest.revision)
	if snapshot, available, inspectErr := s.inspectManifestCache(
		ctx, scope.CacheDirectory, spec, source, manifest,
	); inspectErr != nil {
		return models.PrepareModelAssetsResult{Asset: snapshot.Clone()}, inspectErr
	} else if available {
		return models.PrepareModelAssetsResult{
			Asset: snapshot.Clone(), Outcome: models.AssetPreparationAlreadyAvailable,
		}, nil
	}
	snapshot, err := s.publishManifest(ctx, scope.CacheDirectory, spec, source, manifest)
	if err != nil {
		return models.PrepareModelAssetsResult{Asset: snapshot.Clone()}, err
	}
	return models.PrepareModelAssetsResult{
		Asset: snapshot.Clone(), Outcome: models.AssetPreparationPrepared,
	}, nil
}

func assetRequirementsForSpec(spec assetSpec) []models.AssetRequirement {
	result := make([]models.AssetRequirement, 0, len(spec.requiredArtifacts))
	for _, name := range spec.requiredArtifacts {
		result = append(result, models.AssetRequirement{Name: name})
	}
	return result
}

func requirementsFromManifest(manifest remoteManifest) []models.AssetRequirement {
	result := make([]models.AssetRequirement, 0, len(manifest.files))
	for _, file := range manifest.files {
		result = append(result, models.AssetRequirement{
			Name: file.path, Bytes: file.bytes, SHA256: file.sha256,
		})
	}
	return result
}

// inspectManifestCache reconciles complete caches written by the retired
// puller whose local metadata may not contain every current integrity fact.
// The upstream manifest supplies those facts, allowing a verified cache hit
// without downloading or rewriting already-published asset files.
func (s *service) inspectManifestCache(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
	manifest remoteManifest,
) (models.AssetSnapshot, bool, error) {
	root, err := s.modelCacheRoot(cacheDirectory, spec.modelName)
	if err != nil {
		return missingSnapshot(spec.modelName, source), false, err
	}
	artifacts := make([]models.AssetArtifact, 0, len(manifest.files))
	for _, remote := range manifest.files {
		if err := assetContextError(ctx); err != nil {
			return unavailableSnapshot(
				spec.modelName, source, manifest.revision, artifacts,
			), false, err
		}
		artifact, verifyErr := s.verifyCachedFile(
			ctx,
			filepath.Join(root, manifest.revision, filepath.FromSlash(remote.path)),
			metadataFile{Path: remote.path, Bytes: remote.bytes, SHA256: remote.sha256},
		)
		if errors.Is(verifyErr, os.ErrNotExist) {
			return unavailableSnapshot(
				spec.modelName, source, manifest.revision, artifacts,
			), false, nil
		}
		if verifyErr != nil {
			diagnostics := append(artifacts, artifact)
			return failedSnapshot(
				spec.modelName, source, manifest.revision, diagnostics,
			), false, verifyErr
		}
		artifacts = append(artifacts, artifact)
	}
	snapshot := availableSnapshot(spec.modelName, source, manifest.revision, artifacts)
	snapshot.Integrity = models.AssetIntegrityVerified
	return snapshot, true, nil
}

func (s *service) inspectVerifiedCache(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
) (models.AssetSnapshot, bool, error) {
	root, err := s.modelCacheRoot(cacheDirectory, spec.modelName)
	if err != nil {
		return missingSnapshot(spec.modelName, source), false, err
	}
	metadata, found, err := s.readMetadata(ctx, filepath.Join(root, metadataFileName))
	if err != nil || !found {
		return missingSnapshot(spec.modelName, source), false, err
	}

	revision := strings.TrimSpace(metadata.Revision)
	metadataByName := make(map[string]metadataFile, len(metadata.Files))
	for _, file := range metadata.Files {
		metadataByName[filepath.ToSlash(strings.TrimSpace(file.Path))] = file
	}
	artifacts := make([]models.AssetArtifact, 0, len(spec.requiredArtifacts))
	verified := true
	for _, name := range spec.requiredArtifacts {
		if err := assetContextError(ctx); err != nil {
			return unavailableSnapshot(spec.modelName, source, revision, artifacts), false, err
		}
		expected, ok := metadataByName[name]
		if !ok {
			return unavailableSnapshot(spec.modelName, source, revision, artifacts), false, nil
		}
		artifact, err := s.verifyCachedFile(
			ctx,
			filepath.Join(root, revision, filepath.FromSlash(name)),
			expected,
		)
		if errors.Is(err, os.ErrNotExist) {
			return unavailableSnapshot(spec.modelName, source, revision, artifacts), false, nil
		}
		if err != nil {
			diagnostics := append(artifacts, artifact)
			return failedSnapshot(spec.modelName, source, revision, diagnostics), false, err
		}
		if expected.Bytes <= 0 && strings.TrimSpace(expected.SHA256) == "" {
			verified = false
		}
		artifacts = append(artifacts, artifact)
	}
	snapshot := availableSnapshot(spec.modelName, source, revision, artifacts)
	if verified {
		snapshot.Integrity = models.AssetIntegrityVerified
	}
	return snapshot, true, nil
}

func (s *service) verifyCachedFile(
	ctx context.Context,
	path string,
	expected metadataFile,
) (models.AssetArtifact, error) {
	artifact := models.AssetArtifact{
		Name:   expected.Path,
		SHA256: strings.ToLower(strings.TrimSpace(expected.SHA256)),
	}
	info, err := s.inspectPath(path)
	if err != nil {
		return artifact, err
	}
	if info.IsDir() {
		return artifact, os.ErrNotExist
	}
	artifact.Bytes = info.Size()
	if expected.Bytes > 0 && info.Size() != expected.Bytes {
		return artifact, fmt.Errorf(
			"%w: asset %q size is %d, expected %d",
			models.ErrAssetIntegrityFailed, expected.Path, info.Size(), expected.Bytes,
		)
	}
	if artifact.SHA256 != "" {
		actual, hashErr := s.fileSHA256(ctx, path)
		if hashErr != nil {
			return artifact, fmt.Errorf("verify cached asset %q: %w", expected.Path, hashErr)
		}
		if actual != artifact.SHA256 {
			return artifact, fmt.Errorf(
				"%w: asset %q checksum does not match",
				models.ErrAssetIntegrityFailed, expected.Path,
			)
		}
	}
	return artifact, nil
}

func (s *service) fetchManifest(ctx context.Context, spec assetSpec) (remoteManifest, error) {
	requestURL := strings.TrimRight(s.endpoints.APIBaseURL, "/") + "/models/" + spec.repository
	diagnostics := models.PullDiagnostics{
		ModelName:          spec.modelName,
		ResolvedRepository: spec.repository,
		Operation:          "fetch model asset manifest",
		RequestURL:         requestURL,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return remoteManifest{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: build model asset manifest request", models.ErrSourceFetchFailed),
		)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return remoteManifest{}, contextErr
		}
		return remoteManifest{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: fetch model asset manifest", models.ErrSourceFetchFailed),
		)
	}
	defer response.Body.Close()
	if contextErr := assetContextError(ctx); contextErr != nil {
		return remoteManifest{}, contextErr
	}
	if response.StatusCode != http.StatusOK {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return remoteManifest{}, contextErr
		}
		diagnostics.UpstreamStatusCode = response.StatusCode
		return remoteManifest{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: fetch model asset manifest failed", models.ErrSourceFetchFailed),
		)
	}
	var payload upstreamModel
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return remoteManifest{}, contextErr
		}
		diagnostics.Operation = "decode model asset manifest"
		return remoteManifest{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: decode model asset manifest", models.ErrSourceFetchFailed),
		)
	}
	revision := strings.TrimSpace(payload.SHA)
	if revision == "" {
		revision = "main"
	}
	byPath := make(map[string]upstreamSibling, len(payload.Siblings))
	for _, sibling := range payload.Siblings {
		byPath[strings.TrimSpace(sibling.Path)] = sibling
	}
	files := make([]remoteFile, 0, len(spec.requiredArtifacts))
	for _, name := range spec.requiredArtifacts {
		sibling, ok := byPath[name]
		if !ok {
			missing := diagnostics
			missing.File = name
			missing.Operation = "resolve manifest artifact"
			return remoteManifest{}, pullsupport.WrapPullDiagnostics(
				missing,
				pullsupport.WrapPullStage(
					models.PullStageSourceResolution, spec.modelName,
					missing.Operation, name,
					fmt.Errorf("%w: manifest is missing required asset", models.ErrModelReferenceUnknown),
				),
			)
		}
		size := sibling.Size
		checksum := ""
		if sibling.LFS != nil {
			if sibling.LFS.Size > 0 {
				size = sibling.LFS.Size
			}
			checksum = strings.ToLower(strings.TrimSpace(sibling.LFS.OID))
		}
		files = append(files, remoteFile{
			modelName: spec.modelName, repository: spec.repository, revision: revision,
			path: name, bytes: size, sha256: checksum,
			url: strings.TrimRight(s.endpoints.BaseURL, "/") + "/" + spec.repository +
				"/resolve/" + url.PathEscape(revision) + "/" +
				strings.ReplaceAll(name, " ", "%20") + "?download=true",
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return remoteManifest{revision: revision, files: files}, nil
}

func (s *service) publishManifest(
	ctx context.Context,
	cacheDirectory string,
	spec assetSpec,
	source models.SourceMetadata,
	manifest remoteManifest,
) (snapshot models.AssetSnapshot, resultErr error) {
	root, err := s.modelCacheRoot(cacheDirectory, spec.modelName)
	if err != nil {
		return missingSnapshot(spec.modelName, source), err
	}
	stagePath := filepath.Join(root, manifest.revision+".partial")
	finalPath := filepath.Join(root, manifest.revision)
	metadataPath := filepath.Join(root, metadataFileName)
	metadataStagePath := metadataPath + ".partial"
	if err := s.ensureStageAbsent(stagePath); err != nil {
		return missingSnapshot(spec.modelName, source), err
	}
	if err := s.makeDirectory(stagePath, 0o755); err != nil {
		return missingSnapshot(spec.modelName, source),
			interruptedAssetError("prepare asset staging directory", err)
	}

	created := make([]string, 0, len(manifest.files))
	promoted := false
	success := false
	defer func() {
		if !success {
			base := stagePath
			if promoted {
				base = finalPath
			}
			if cleanupErr := s.cleanupAttempt(base, metadataStagePath, created); cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()

	artifacts := make([]models.AssetArtifact, 0, len(manifest.files))
	for _, remote := range manifest.files {
		if err := assetContextError(ctx); err != nil {
			return failedSnapshot(spec.modelName, source, manifest.revision, artifacts), err
		}
		created = append(created, remote.path)
		artifact, err := s.downloadFile(ctx, stagePath, remote)
		if err != nil {
			return failedSnapshot(spec.modelName, source, manifest.revision, artifacts), err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := assetContextError(ctx); err != nil {
		return failedSnapshot(spec.modelName, source, manifest.revision, artifacts), err
	}
	if err := s.writeMetadata(metadataStagePath, spec, manifest); err != nil {
		return failedSnapshot(spec.modelName, source, manifest.revision, artifacts),
			interruptedAssetError("stage asset metadata", err)
	}
	if err := assetContextError(ctx); err != nil {
		return failedSnapshot(spec.modelName, source, manifest.revision, artifacts), err
	}
	promoted, err = s.promoteAttempt(
		ctx, stagePath, finalPath, metadataStagePath, metadataPath,
	)
	if err != nil {
		return failedSnapshot(spec.modelName, source, manifest.revision, artifacts),
			err
	}
	success = true
	snapshot = availableSnapshot(spec.modelName, source, manifest.revision, artifacts)
	snapshot.Integrity = models.AssetIntegrityVerified
	return snapshot, nil
}

func (s *service) cleanupAttempt(basePath, metadataStagePath string, created []string) error {
	paths := make([]string, 0, len(created)+2)
	directories := make(map[string]struct{})
	for index := len(created) - 1; index >= 0; index-- {
		relative := filepath.FromSlash(created[index])
		paths = append(paths, filepath.Join(basePath, relative))
		for directory := filepath.Dir(relative); directory != "."; directory = filepath.Dir(directory) {
			directories[filepath.Join(basePath, directory)] = struct{}{}
		}
	}
	nested := make([]string, 0, len(directories))
	for directory := range directories {
		nested = append(nested, directory)
	}
	sort.Slice(nested, func(i, j int) bool { return len(nested[i]) > len(nested[j]) })
	paths = append(paths, nested...)
	paths = append(paths, basePath, metadataStagePath)

	var cleanupErr error
	for _, path := range paths {
		if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(
				cleanupErr,
				fmt.Errorf("clean up asset preparation path %q: %w", filepath.Base(path), err),
			)
		}
	}
	return cleanupErr
}

func (s *service) promoteAttempt(
	ctx context.Context,
	stagePath string,
	finalPath string,
	metadataStagePath string,
	metadataPath string,
) (bool, error) {
	if err := s.renamePath(stagePath, finalPath); err != nil {
		return false, pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "publish verified asset revision", "",
			interruptedAssetError("publish verified asset revision", err),
		)
	}
	if err := assetContextError(ctx); err != nil {
		return true, err
	}
	if err := s.renamePath(metadataStagePath, metadataPath); err != nil {
		return true, pullsupport.WrapPullStage(
			models.PullStageCacheInstallation, "", "publish verified asset metadata", "",
			interruptedAssetError("publish verified asset metadata", err),
		)
	}
	return true, nil
}

func (s *service) ensureStageAbsent(path string) error {
	_, err := s.inspectPath(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("inspect asset staging directory: %w", err)
	default:
		return fmt.Errorf("%w: an asset preparation attempt is already staged", models.ErrAssetPreparationInterrupted)
	}
}

func (s *service) downloadFile(
	ctx context.Context,
	stagePath string,
	remote remoteFile,
) (models.AssetArtifact, error) {
	diagnostics := models.PullDiagnostics{
		ModelName:          remote.modelName,
		ResolvedRepository: remote.repository,
		Revision:           remote.revision,
		File:               remote.path,
		Operation:          "download asset",
		RequestURL:         remote.url,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.url, nil)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: build asset request", models.ErrSourceFetchFailed),
		)
	}
	response, err := s.doWithRetry(request)
	if err != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return models.AssetArtifact{}, contextErr
		}
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: download asset", models.ErrSourceFetchFailed),
		)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		diagnostics.UpstreamStatusCode = response.StatusCode
	}
	if err := validateDownloadResponse(ctx, response, remote.path); err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(diagnostics, err)
	}

	target := filepath.Join(stagePath, filepath.FromSlash(remote.path))
	if err := s.makeDirectory(filepath.Dir(target), 0o755); err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			interruptedAssetError("prepare asset directory", err),
		)
	}
	output, err := s.createFile(target)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			interruptedAssetError("create staged asset", err),
		)
	}
	written, checksum, err := copyStagedAsset(ctx, output, response.Body, remote.path)
	if err != nil {
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(diagnostics, err)
	}
	if remote.bytes > 0 && written != remote.bytes {
		diagnostics.Operation = "verify downloaded asset"
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: asset size does not match", models.ErrAssetIntegrityFailed),
		)
	}
	if remote.sha256 != "" && checksum != remote.sha256 {
		diagnostics.Operation = "verify downloaded asset"
		return models.AssetArtifact{}, pullsupport.WrapPullDiagnostics(
			diagnostics,
			fmt.Errorf("%w: asset checksum does not match", models.ErrAssetIntegrityFailed),
		)
	}
	if remote.sha256 == "" {
		checksum = ""
	}
	return models.AssetArtifact{Name: remote.path, Bytes: written, SHA256: checksum}, nil
}

func validateDownloadResponse(ctx context.Context, response *http.Response, assetPath string) error {
	if contextErr := assetContextError(ctx); contextErr != nil {
		return contextErr
	}
	if response.StatusCode == http.StatusOK {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 512))
	if contextErr := assetContextError(ctx); contextErr != nil {
		return contextErr
	}
	return fmt.Errorf(
		"%w: download asset %q failed (%d)",
		models.ErrSourceFetchFailed,
		assetPath,
		response.StatusCode,
	)
}

func copyStagedAsset(
	ctx context.Context,
	output io.WriteCloser,
	input io.Reader,
	assetPath string,
) (int64, string, error) {
	hasher := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(output, hasher),
		contextAssetReader{ctx: ctx, input: input},
	)
	closeErr := output.Close()
	if copyErr != nil {
		if contextErr := assetContextError(ctx); contextErr != nil {
			return 0, "", contextErr
		}
		return 0, "", interruptedAssetError("write staged asset "+assetPath, copyErr)
	}
	if closeErr != nil {
		return 0, "", interruptedAssetError("close staged asset "+assetPath, closeErr)
	}
	if contextErr := assetContextError(ctx); contextErr != nil {
		return 0, "", contextErr
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *service) writeMetadata(path string, spec assetSpec, manifest remoteManifest) error {
	metadata := cacheMetadata{
		ModelName: spec.modelName,
		Revision:  manifest.revision,
		Files:     make([]metadataFile, 0, len(manifest.files)),
	}
	for _, file := range manifest.files {
		metadata.Files = append(metadata.Files, metadataFile{
			Path: file.path, Bytes: file.bytes, SHA256: file.sha256,
		})
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode asset metadata: %w", err)
	}
	if err := s.writeFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write staged asset metadata: %w", err)
	}
	return nil
}

func (s *service) fileSHA256(ctx context.Context, path string) (string, error) {
	input, err := s.openFile(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, contextAssetReader{ctx: ctx, input: input}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *service) doWithRetry(request *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= assetSourceMaxAttempts; attempt++ {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		response, err := s.client.Do(request.Clone(request.Context()))
		if err == nil {
			if response == nil {
				return nil, fmt.Errorf("empty asset source response")
			}
			if response.Body == nil {
				response.Body = http.NoBody
			}
			if shouldRetryResponse(response) && attempt < assetSourceMaxAttempts {
				lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
				_ = response.Body.Close()
				continue
			}
			return response, nil
		}
		lastErr = err
		if attempt == assetSourceMaxAttempts || !shouldRetryError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func interruptedAssetError(operation string, cause error) error {
	return &assetPreparationInterruption{operation: operation, cause: cause}
}

type assetPreparationInterruption struct {
	operation string
	cause     error
}

func (failure *assetPreparationInterruption) Error() string {
	if failure == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", models.ErrAssetPreparationInterrupted, failure.operation)
}

func (failure *assetPreparationInterruption) Unwrap() error {
	if failure == nil {
		return nil
	}
	return errors.Join(models.ErrAssetPreparationInterrupted, failure.cause)
}

type contextAssetReader struct {
	ctx   context.Context
	input io.Reader
}

func (reader contextAssetReader) Read(buffer []byte) (int, error) {
	if err := assetContextError(reader.ctx); err != nil {
		return 0, err
	}
	return reader.input.Read(buffer)
}

func shouldRetryResponse(response *http.Response) bool {
	return response != nil &&
		(response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500)
}

func shouldRetryError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func failedSnapshot(
	modelName string,
	source models.SourceMetadata,
	revision string,
	artifacts []models.AssetArtifact,
) models.AssetSnapshot {
	snapshot := unavailableSnapshot(modelName, source, revision, artifacts)
	snapshot.Readiness = models.AssetReadinessFailed
	snapshot.Integrity = models.AssetIntegrityFailed
	return snapshot
}
