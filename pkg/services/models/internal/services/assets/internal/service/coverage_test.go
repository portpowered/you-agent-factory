package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestGenericCacheFindsVerifiedCandidate(t *testing.T) {
	scopes := newScopes(t, "generic-cache-candidates")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("candidate inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	body := []byte("verified payload")
	source := genericTestHFSource()
	artifact := genericArtifact{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
	}}
	root := t.TempDir()
	identity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{artifact})
	candidates := genericCandidatePaths(root, assetKindModel, source, identity, artifact.requirement.Name)
	if len(candidates) < 3 {
		t.Fatalf("candidate paths = %#v, want content, legacy, and HF paths", candidates)
	}
	if err := os.MkdirAll(candidates[0], 0o755); err != nil {
		t.Fatalf("create directory candidate: %v", err)
	}
	corrupt := append([]byte(nil), body...)
	corrupt[0] ^= 1
	if err := os.MkdirAll(filepath.Dir(candidates[1]), 0o755); err != nil {
		t.Fatalf("create legacy candidate parent: %v", err)
	}
	if err := os.WriteFile(candidates[1], corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt candidate: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(candidates[2]), 0o755); err != nil {
		t.Fatalf("create HF candidate parent: %v", err)
	}
	if err := os.WriteFile(candidates[2], body, 0o644); err != nil {
		t.Fatalf("write verified candidate: %v", err)
	}
	found, ok, err := service.findGenericArtifact(
		context.Background(), assetKindModel, source, identity, artifact, []string{root},
	)
	if err != nil || !ok || found.path != candidates[2] || found.artifact.SHA256 != sha256Hex(body) {
		t.Fatalf("candidate result = %#v, %t, %v; want verified HF candidate", found, ok, err)
	}
}

func TestGenericCacheRejectsUnverifiedCandidates(t *testing.T) {
	body := []byte("verified payload")
	scopes := newScopes(t, "generic-cache-unverified-candidates")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("candidate inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericTestHFSource()
	noDigestArtifact := genericArtifact{requirement: models.AssetRequirement{Name: "weights.bin", Bytes: int64(len(body))}}
	noDigestRoot := t.TempDir()
	noDigestIdentity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{noDigestArtifact})
	noDigestCandidate := genericCandidatePaths(noDigestRoot, assetKindModel, source, noDigestIdentity, "weights.bin")[2]
	if err := os.MkdirAll(filepath.Dir(noDigestCandidate), 0o755); err != nil {
		t.Fatalf("create no-digest parent: %v", err)
	}
	if err := os.WriteFile(noDigestCandidate, body, 0o644); err != nil {
		t.Fatalf("write no-digest candidate: %v", err)
	}
	if _, ok, err := service.findGenericArtifact(
		context.Background(), assetKindModel, source, noDigestIdentity,
		noDigestArtifact, []string{noDigestRoot},
	); err != nil || ok {
		t.Fatalf("no-digest HF candidate = %t, %v, want rejected cache", ok, err)
	}

	localSource := genericSource{kind: genericSourceLocal, safe: "local://path", localPath: "model.bin"}
	sizeArtifact := genericArtifact{requirement: models.AssetRequirement{
		Name: "model.bin", Bytes: int64(len(body) + 1), SHA256: sha256Hex(body),
	}}
	sizeRoot := t.TempDir()
	sizeIdentity := genericArtifactIdentityHash(assetKindModel, localSource, []genericArtifact{sizeArtifact})
	sizeCandidate := genericCandidatePaths(sizeRoot, assetKindModel, localSource, sizeIdentity, "model.bin")[0]
	if err := os.MkdirAll(filepath.Dir(sizeCandidate), 0o755); err != nil {
		t.Fatalf("create size candidate parent: %v", err)
	}
	if err := os.WriteFile(sizeCandidate, body, 0o644); err != nil {
		t.Fatalf("write size candidate: %v", err)
	}
	if _, ok, err := service.findGenericArtifact(
		context.Background(), assetKindModel, localSource, sizeIdentity,
		sizeArtifact, []string{sizeRoot},
	); err != nil || ok {
		t.Fatalf("size-mismatched candidate = %t, %v, want rejected cache", ok, err)
	}
}

func TestGenericCacheFindCandidateHandlesReadAndCancellation(t *testing.T) {
	scopes := newScopes(t, "generic-cache-candidate-cancellation")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("candidate inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericTestHFSource()
	artifact := genericArtifact{requirement: models.AssetRequirement{Name: "weights.bin"}}
	root := t.TempDir()
	identity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{artifact})
	candidate := genericCandidatePaths(root, assetKindModel, source, identity, artifact.requirement.Name)[0]
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		t.Fatalf("create candidate parent: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	service.openFile = func(string) (io.ReadCloser, error) { return nil, errors.New("cache read failed") }
	if _, ok, err := service.findGenericArtifact(
		context.Background(), assetKindModel, source, identity, artifact, []string{root},
	); err != nil || ok {
		t.Fatalf("cache read failure = %t, %v, want skipped unreadable candidate", ok, err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	service.openFile = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	if _, _, err := service.findGenericArtifact(
		cancelled, assetKindModel, source, identity, artifact, []string{root},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled candidate inspection = %v, want context.Canceled", err)
	}
}

func TestGenericCacheRejectsUnsupportedAndMissingLocalSources(t *testing.T) {
	scopes := newScopes(t, "generic-cache-local-failures")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("local failure test used HTTP")
	}), func(string) string { return "" })
	requirement := models.AssetRequirement{Name: "weights.bin", Bytes: int64(len("payload"))}
	progress := newAssetProgress(context.Background(), "voice", requirement.Name, requirement.Bytes)
	if _, err := service.stageGenericArtifact(
		context.Background(), genericSource{kind: genericSourceKind("unknown")}, "", "", requirement, progress,
	); !errors.Is(err, models.ErrAssetSourceUnsupported) {
		t.Fatalf("unsupported generic source error = %v, want ErrAssetSourceUnsupported", err)
	}
	if _, err := service.copyLocalArtifact(
		context.Background(), filepath.Join(t.TempDir(), "missing.bin"), filepath.Join(t.TempDir(), "target.bin"), requirement, progress,
	); !errors.Is(err, models.ErrAssetSourceMissing) {
		t.Fatalf("missing local artifact error = %v, want ErrAssetSourceMissing", err)
	}
}

func TestGenericCacheClassifiesStagingAndRemoteFailures(t *testing.T) {
	scopes := newScopes(t, "generic-cache-remote-failures")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("opaque upstream failure"))}, nil
	}), func(string) string { return "" })
	requirement := models.AssetRequirement{Name: "weights.bin", Bytes: int64(len("payload"))}
	progress := newAssetProgress(context.Background(), "voice", requirement.Name, requirement.Bytes)
	localPath := filepath.Join(t.TempDir(), "local.bin")
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write local source: %v", err)
	}
	service.createFile = func(string) (io.WriteCloser, error) { return nil, errors.New("staging disk full") }
	if _, err := service.copyLocalArtifact(
		context.Background(), localPath, filepath.Join(t.TempDir(), "target.bin"), requirement, progress,
	); !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("local staging failure = %v, want interruption", err)
	}

	remote := genericTestHFSource()
	remote.modelName = "voice"
	service.createFile = func(path string) (io.WriteCloser, error) { return os.Create(path) }
	if _, err := service.downloadGenericArtifact(
		context.Background(), remote, filepath.Join(t.TempDir(), "weights.bin"), requirement, progress,
	); !errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("remote status failure = %v, want ErrSourceFetchFailed", err)
	}
	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload"))}, nil
	})
	service.createFile = func(string) (io.WriteCloser, error) { return nil, errors.New("staging disk full") }
	if _, err := service.downloadGenericArtifact(
		context.Background(), remote, filepath.Join(t.TempDir(), "weights.bin"), requirement, progress,
	); !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("remote staging failure = %v, want interruption", err)
	}
	service.createFile = func(path string) (io.WriteCloser, error) { return os.Create(path) }
	wrongSize := requirement
	wrongSize.Bytes++
	if _, err := service.downloadGenericArtifact(
		context.Background(), remote, filepath.Join(t.TempDir(), "weights.bin"), wrongSize, progress,
	); !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("remote size failure = %v, want ErrAssetIntegrityFailed", err)
	}
	digestRequirement := requirement
	digestRequirement.SHA256 = sha256Hex([]byte("expected payload"))
	if _, err := verifiedGenericArtifact(digestRequirement, digestRequirement.Bytes, "wrong-digest"); !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("digest verification failure = %v, want ErrAssetIntegrityFailed", err)
	}
}

func TestGenericCacheRejectsConflictingDiscoveredMetadata(t *testing.T) {
	scopes := newScopes(t, "generic-cache-conflicting-metadata")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("conflicting cache metadata used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericTestHFSource()
	root := t.TempDir()
	snapshot := filepath.Join(root, assetContentDirectory, assetKindModel, "discovered")
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create discovered snapshot: %v", err)
	}
	metadata, err := json.Marshal(genericCacheMetadata{
		Kind: assetKindModel, SourceKey: genericSourceIdentity(source),
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", Bytes: 1}},
	})
	if err != nil {
		t.Fatalf("marshal discovered metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, assetMetadataName), metadata, 0o644); err != nil {
		t.Fatalf("write discovered metadata: %v", err)
	}
	requested := []genericArtifact{{requirement: models.AssetRequirement{Name: "weights.bin", Bytes: 2}}}
	_, err = service.acquireGenericCache(
		context.Background(), assetKindModel, models.AssetArtifactKindModel,
		source, requested, []string{root}, false,
	)
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("conflicting discovered metadata error = %v, want integrity failure", err)
	}
}

func TestGenericCacheFollowerCancellationDoesNotWaitForLeader(t *testing.T) {
	body := []byte("shared cache payload")
	manifest, err := json.Marshal(map[string]any{
		"sha": genericTestRevision,
		"siblings": []map[string]any{{
			"rfilename": "weights.bin", "size": len(body),
			"lfs": map[string]any{"oid": sha256Hex(body), "size": len(body)},
		}},
	})
	if err != nil {
		t.Fatalf("marshal shared manifest: %v", err)
	}
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasPrefix(request.URL.Path, "/models/") {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(manifest))}, nil
		}
		close(downloadStarted)
		<-releaseDownload
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	scopes := newScopes(t, "generic-cache-follower-cancel")
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	source := genericTestHFSource()
	artifact := []genericArtifact{{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
	}}}
	root := t.TempDir()
	leaderResult := make(chan error, 1)
	go func() {
		_, leaderErr := service.acquireGenericCache(context.Background(), assetKindModel, models.AssetArtifactKindModel, source, artifact, []string{root}, false)
		leaderResult <- leaderErr
	}()
	<-downloadStarted
	followerJoined := make(chan struct{})
	service.cacheJoinObserver = func() { close(followerJoined) }
	followerContext, cancel := context.WithCancel(context.Background())
	followerResult := make(chan error, 1)
	go func() {
		_, followerErr := service.acquireGenericCache(followerContext, assetKindModel, models.AssetArtifactKindModel, source, artifact, []string{root}, false)
		followerResult <- followerErr
	}()
	<-followerJoined
	cancel()
	if err := <-followerResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled follower error = %v, want context.Canceled", err)
	}
	close(releaseDownload)
	if err := <-leaderResult; err != nil {
		t.Fatalf("leader cache result = %v, want successful publication", err)
	}
}

func TestGenericSnapshotPathRejectsMixedRoots(t *testing.T) {
	if got := genericSnapshotPath(nil); got != "" {
		t.Fatalf("genericSnapshotPath(nil) = %q, want empty path", got)
	}
	if got := genericSnapshotPath([]string{
		filepath.Join(t.TempDir(), "snapshot", "one.bin"),
		filepath.Join(t.TempDir(), "snapshot", "two.bin"),
	}); got != "" {
		t.Fatalf("genericSnapshotPath(mixed roots) = %q, want empty path", got)
	}
}

func TestGenericCacheRemoveTreeClassifiesFailures(t *testing.T) {
	scopes := newScopes(t, "generic-cache-cleanup-failures")
	service := newGenericService(t, scopes, nil, func(string) string { return "" })
	filePath := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("write removable file: %v", err)
	}
	if err := service.removeTree(filePath); err != nil {
		t.Fatalf("removeTree(file) = %v", err)
	}
	readFailure := errors.New("directory read failed")
	readService := newGenericService(t, scopes, nil, func(string) string { return "" })
	readService.readDirectory = func(string) ([]os.DirEntry, error) { return nil, readFailure }
	if err := readService.removeTree(t.TempDir()); !errors.Is(err, readFailure) {
		t.Fatalf("removeTree(read failure) = %v, want read cause", err)
	}
	nestedRoot := t.TempDir()
	nestedFile := filepath.Join(nestedRoot, "nested.bin")
	if err := os.WriteFile(nestedFile, []byte("asset"), 0o644); err != nil {
		t.Fatalf("write nested removable file: %v", err)
	}
	removeFailure := errors.New("nested remove failed")
	removeService := newGenericService(t, scopes, nil, func(string) string { return "" })
	removeService.removePath = func(path string) error {
		if path == nestedFile {
			return removeFailure
		}
		return os.Remove(path)
	}
	if err := removeService.removeTree(nestedRoot); !errors.Is(err, removeFailure) {
		t.Fatalf("removeTree(nested failure) = %v, want nested cause", err)
	}
	inspectFailure := errors.New("cache stat failed")
	inspectService := newGenericService(t, scopes, nil, func(string) string { return "" })
	inspectService.inspectPath = func(string) (os.FileInfo, error) { return nil, inspectFailure }
	if err := inspectService.removeTree("cache"); !errors.Is(err, inspectFailure) {
		t.Fatalf("removeTree(stat failure) = %v, want stat cause", err)
	}
}

func TestGenericCacheStageSetupFailuresRemainTyped(t *testing.T) {
	scopes := newScopes(t, "generic-cache-stage-failures")
	stageFailure := errors.New("staging directory unavailable")
	clearService := newGenericService(t, scopes, nil, func(string) string { return "" })
	clearService.inspectPath = func(string) (os.FileInfo, error) { return nil, stageFailure }
	if err := clearService.prepareGenericStage("base", "stage"); !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, stageFailure) {
		t.Fatalf("prepareGenericStage(clear failure) = %v, want typed stage failure", err)
	}
	baseService := newGenericService(t, scopes, nil, func(string) string { return "" })
	baseService.makeDirectory = func(string, os.FileMode) error { return stageFailure }
	if err := baseService.prepareGenericStage(t.TempDir(), filepath.Join(t.TempDir(), "stage")); !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, stageFailure) {
		t.Fatalf("prepareGenericStage(base failure) = %v, want typed stage failure", err)
	}
	stageService := newGenericService(t, scopes, nil, func(string) string { return "" })
	makeCalls := 0
	stageService.makeDirectory = func(string, os.FileMode) error {
		makeCalls++
		if makeCalls == 2 {
			return stageFailure
		}
		return nil
	}
	if err := stageService.prepareGenericStage(t.TempDir(), filepath.Join(t.TempDir(), "stage")); !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, stageFailure) {
		t.Fatalf("prepareGenericStage(stage failure) = %v, want typed stage failure", err)
	}
}

func TestGenericCacheMetadataPublicationFailureIsTyped(t *testing.T) {
	scopes := newScopes(t, "generic-cache-metadata-failure")
	service := newGenericService(t, scopes, nil, func(string) string { return "" })
	localPath := filepath.Join(t.TempDir(), "weights.bin")
	body := []byte("local payload")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local publication source: %v", err)
	}
	service.writeFile = func(string, []byte, os.FileMode) error { return errors.New("metadata write failed") }
	source := genericSource{kind: genericSourceLocal, safe: "local://path", localPath: localPath}
	artifact := genericArtifact{
		requirement: models.AssetRequirement{Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body)},
		localPath:   localPath,
	}
	_, err := service.publishGenericCache(
		context.Background(), assetKindModel, models.AssetArtifactKindModel,
		source, []genericArtifact{artifact}, nil, []genericArtifact{artifact}, []string{t.TempDir()},
	)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !strings.Contains(err.Error(), "stage asset metadata") {
		t.Fatalf("metadata publication failure = %v, want typed metadata stage failure", err)
	}
}

func TestAssetStagePresenceErrors(t *testing.T) {
	scopes := newScopes(t, "asset-stage-presence")
	service := newPreparationTestService(scopes, nil, models.RuntimeAssetEndpoints{}, nil)
	missingStage := filepath.Join(t.TempDir(), "missing.partial")
	if err := service.ensureStageAbsent(missingStage); err != nil {
		t.Fatalf("ensureStageAbsent(missing) = %v, want nil", err)
	}
	presentStage := filepath.Join(t.TempDir(), "present.partial")
	if err := os.MkdirAll(presentStage, 0o755); err != nil {
		t.Fatalf("create present stage: %v", err)
	}
	if err := service.ensureStageAbsent(presentStage); !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("ensureStageAbsent(present) = %v, want staged-attempt interruption", err)
	}
	service.inspectPath = func(string) (os.FileInfo, error) { return nil, errors.New("stat unavailable") }
	if err := service.ensureStageAbsent(missingStage); err == nil || !strings.Contains(err.Error(), "inspect asset staging directory") {
		t.Fatalf("ensureStageAbsent(stat failure) = %v, want inspection diagnostic", err)
	}
}

func TestAssetPromoteAttemptClassifiesRevisionRenameFailure(t *testing.T) {
	scopes := newScopes(t, "asset-promote-rename-failure")
	service := newPreparationTestService(scopes, nil, models.RuntimeAssetEndpoints{}, nil)
	renameCause := errors.New("revision rename failed")
	service.renamePath = func(string, string) error { return renameCause }
	if promoted, err := service.promoteAttempt(context.Background(), "stage", "final", "metadata.partial", "metadata"); promoted || !errors.Is(err, renameCause) || !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("revision rename result = (%t, %v), want interruption and cause", promoted, err)
	}
}

func TestAssetPromoteAttemptReturnsCancellationAfterRevision(t *testing.T) {
	scopes := newScopes(t, "asset-promote-cancellation")
	service := newPreparationTestService(scopes, nil, models.RuntimeAssetEndpoints{}, nil)
	root := t.TempDir()
	stage := filepath.Join(root, "stage.partial")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("create cancellable stage: %v", err)
	}
	final := filepath.Join(root, "revision")
	ctx, cancel := context.WithCancel(context.Background())
	service.renamePath = func(oldPath, newPath string) error {
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
		cancel()
		return nil
	}
	if promoted, err := service.promoteAttempt(ctx, stage, final, "metadata.partial", "metadata"); !promoted || !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("cancelled promotion result = (%t, %v), want promoted cancellation", promoted, err)
	}
}

func TestAssetPromoteAttemptClassifiesMetadataRenameFailure(t *testing.T) {
	scopes := newScopes(t, "asset-promote-metadata-failure")
	service := newPreparationTestService(scopes, nil, models.RuntimeAssetEndpoints{}, nil)
	root := t.TempDir()
	stage := filepath.Join(root, "stage.partial")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("create metadata stage: %v", err)
	}
	metadataStage := filepath.Join(root, "metadata.partial")
	if err := os.WriteFile(metadataStage, []byte("metadata"), 0o644); err != nil {
		t.Fatalf("create staged metadata: %v", err)
	}
	metadataCause := errors.New("metadata rename failed")
	renameCount := 0
	service.renamePath = func(oldPath, newPath string) error {
		renameCount++
		if renameCount == 2 {
			return metadataCause
		}
		return os.Rename(oldPath, newPath)
	}
	if promoted, err := service.promoteAttempt(context.Background(), stage, filepath.Join(root, "revision"), metadataStage, filepath.Join(root, "metadata")); !promoted || !errors.Is(err, metadataCause) || !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("metadata rename result = (%t, %v), want interruption and cause", promoted, err)
	}
}

func TestAssetMetadataAndHashFailuresRemainActionable(t *testing.T) {
	scopes := newScopes(t, "asset-metadata-hash-failures")
	service := newPreparationTestService(scopes, nil, models.RuntimeAssetEndpoints{}, nil)
	manifest := remoteManifest{revision: "rev-1", files: []remoteFile{{path: "weights.bin", bytes: 7, sha256: "digest"}}}
	service.writeFile = func(string, []byte, os.FileMode) error { return errors.New("metadata disk full") }
	if err := service.writeMetadata("metadata", assetSpec{modelName: "voice"}, manifest); err == nil || !strings.Contains(err.Error(), "write staged asset metadata") {
		t.Fatalf("metadata write failure = %v, want actionable write diagnostic", err)
	}
	metadataPath := filepath.Join(t.TempDir(), "metadata.json")
	service.writeFile = os.WriteFile
	if err := service.writeMetadata(metadataPath, assetSpec{modelName: "voice"}, manifest); err != nil {
		t.Fatalf("metadata write success = %v", err)
	}
	if body, err := os.ReadFile(metadataPath); err != nil || !strings.Contains(string(body), "weights.bin") {
		t.Fatalf("metadata body = %q, %v, want serialized artifact", body, err)
	}
	hashPath := filepath.Join(t.TempDir(), "weights.bin")
	if err := os.WriteFile(hashPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write hash fixture: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.fileSHA256(ctx, hashPath); !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("cancelled file hash = %v, want ErrAssetCancelled", err)
	}
}

func genericTestHFSource() genericSource {
	return genericSource{
		kind: genericSourceHF, owner: "owner", repository: "repo",
		revision: genericTestRevision, safe: "hf://owner/repo@" + genericTestRevision,
	}
}
