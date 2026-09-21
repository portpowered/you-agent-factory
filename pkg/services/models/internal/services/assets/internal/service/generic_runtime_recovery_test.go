package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

func TestPrepareGenericAssetsRecoversStaleGenericRuntimeRevisionStage(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, true, false)
}

func TestPrepareGenericAssetsRecoversStaleGenericRuntimeMetadataStage(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, false, true)
}

func TestPrepareGenericAssetsRecoversBothStaleGenericRuntimeStages(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, true, true)
}

func assertGenericRuntimeStageRecovery(t *testing.T, revisionStage, metadataStage bool) {
	t.Helper()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-managed-runtime")
	root := removeGenericRuntimeCommit(t, fixture)
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	if revisionStage {
		writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("abandoned revision stage"))
	}
	if metadataStage {
		if err := os.WriteFile(metadataStagePath, []byte("abandoned metadata stage"), 0o644); err != nil {
			t.Fatalf("write abandoned metadata stage: %v", err)
		}
	}
	if err := os.RemoveAll(fixture.sourceRoot); err != nil {
		t.Fatalf("remove original fixture sources: %v", err)
	}
	request := fixture.request
	request.Offline = true
	prepared, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets after stale staging: %v", err)
	}
	if prepared.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("PrepareModelAssets outcome = %v, want verified cache reuse", prepared.Outcome)
	}
	assertGenericRuntimeRecoveryContents(
		t, inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name),
		fixture.modelBodies, fixture.backendBody,
	)
	assertPathAbsent(t, revisionStagePath)
	assertPathAbsent(t, metadataStagePath)
}

func TestPrepareGenericAssetsPreservesCommittedRuntimeAndUnrelatedStages(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "preserve-managed-runtime")
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	metadataPath := filepath.Join(root, metadataFileName)
	metadataBefore, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := metadataPath + ".partial"
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("abandoned revision stage"))
	if err := os.WriteFile(metadataStagePath, []byte("abandoned metadata stage"), 0o644); err != nil {
		t.Fatalf("write abandoned metadata stage: %v", err)
	}
	otherRevision := strings.Repeat("b", len(fixture.inspection.Revision))
	otherRevisionPath := filepath.Join(root, otherRevision+".partial")
	writeGenericRuntimeRevisionStage(t, otherRevisionPath, []byte("unrelated revision stage"))
	otherModelPath := filepath.Join(fixture.cacheDirectory, canonicalModelName("another-model"), fixture.inspection.Revision+".partial")
	writeGenericRuntimeRevisionStage(t, otherModelPath, []byte("other model stage"))

	request := fixture.request
	request.Offline = true
	prepared, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets with committed cache: %v", err)
	}
	if prepared.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("PrepareModelAssets outcome = %v, want already available", prepared.Outcome)
	}
	inspection := inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, fixture.modelBodies, fixture.backendBody)
	if inspection.Revision != fixture.inspection.Revision || filepath.Clean(inspection.CachePath) != filepath.Clean(fixture.inspection.CachePath) {
		t.Fatalf("committed runtime identity changed: before=%#v after=%#v", fixture.inspection, inspection)
	}
	metadataAfter, err := os.ReadFile(metadataPath)
	if err != nil || !bytes.Equal(metadataBefore, metadataAfter) {
		t.Fatalf("committed metadata changed: before=%q after=%q err=%v", metadataBefore, metadataAfter, err)
	}
	assertPathAbsent(t, revisionStagePath)
	assertPathAbsent(t, metadataStagePath)
	assertFileBody(t, filepath.Join(otherRevisionPath, "abandoned.bin"), []byte("unrelated revision stage"))
	assertFileBody(t, filepath.Join(otherModelPath, "abandoned.bin"), []byte("other model stage"))
}

func TestPrepareGenericAssetsReturnsTypedErrorWhenStaleStageCleanupFails(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-stage-cleanup-failure")
	root := removeGenericRuntimeCommit(t, fixture)
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("stale stage"))
	if err := os.WriteFile(metadataStagePath, []byte("stale metadata"), 0o644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}

	cleanupErr := errors.New("injected stale stage inspection failure")
	originalInspect := fixture.service.inspectPath
	fixture.service.inspectPath = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(revisionStagePath) {
			return nil, cleanupErr
		}
		return originalInspect(path)
	}
	request := fixture.request
	request.Offline = true
	_, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, cleanupErr) {
		t.Fatalf("PrepareModelAssets error = %v, want typed cleanup failure", err)
	}
	assertPathAbsent(t, filepath.Join(root, fixture.inspection.Revision))
	assertPathAbsent(t, filepath.Join(root, metadataFileName))
	assertFileBody(t, filepath.Join(revisionStagePath, "abandoned.bin"), []byte("stale stage"))
	assertFileBody(t, metadataStagePath, []byte("stale metadata"))
}

func TestPublishGenericRuntimeLeavesStaleStagesWhenModelLockIsUnavailable(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-stage-lock-failure")
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("stale stage"))
	if err := os.WriteFile(metadataStagePath, []byte("stale metadata"), 0o644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}

	source, err := parseGenericSource(fixture.request.Reference.NameOrURI)
	if err != nil {
		t.Fatalf("parse model source: %v", err)
	}
	paths := make([]string, 0, len(fixture.inspection.ObservedArtifacts))
	for _, artifact := range fixture.inspection.ObservedArtifacts {
		paths = append(paths, filepath.Join(fixture.inspection.CachePath, filepath.FromSlash(artifact.Name)))
	}
	lockErr := errors.New("injected model lock failure")
	fixture.service.coordination = rejectingStagingCoordination{err: lockErr}
	_, err = fixture.service.publishGenericRuntimeCache(
		context.Background(), fixture.cacheDirectory, fixture.request.Name, source,
		genericCacheResult{
			snapshotPath: fixture.inspection.CachePath,
			artifacts:    fixture.inspection.ObservedArtifacts,
			paths:        paths,
		}, genericCacheResult{}, "",
	)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, lockErr) {
		t.Fatalf("publishGenericRuntimeCache error = %v, want typed lock failure", err)
	}
	assertFileBody(t, filepath.Join(revisionStagePath, "abandoned.bin"), []byte("stale stage"))
	assertFileBody(t, metadataStagePath, []byte("stale metadata"))
}

type genericRuntimeRecoveryFixture struct {
	cacheDirectory string
	sourceRoot     string
	scope          models.RuntimeScopeRef
	service        *service
	request        models.PrepareModelAssetsRequest
	modelBodies    map[string][]byte
	backendBody    []byte
	inspection     assets.RuntimeCacheInspection
}

func newGenericRuntimeRecoveryFixture(t *testing.T, issuer string) genericRuntimeRecoveryFixture {
	t.Helper()
	cacheDirectory := t.TempDir()
	sourceRoot := t.TempDir()
	modelDirectory := filepath.Join(sourceRoot, "model")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create controlled model directory: %v", err)
	}
	modelBodies := map[string][]byte{
		"weights.gguf":  []byte("controlled model weights"),
		"projector.bin": []byte("controlled projector bytes"),
	}
	modelArtifacts := []models.AssetRequirement{
		{Name: "weights.gguf", Bytes: int64(len(modelBodies["weights.gguf"])), SHA256: sha256Hex(modelBodies["weights.gguf"])},
		{Name: "projector.bin", Bytes: int64(len(modelBodies["projector.bin"])), SHA256: sha256Hex(modelBodies["projector.bin"])},
	}
	for _, artifact := range modelArtifacts {
		if err := os.WriteFile(filepath.Join(modelDirectory, artifact.Name), modelBodies[artifact.Name], 0o644); err != nil {
			t.Fatalf("write controlled model artifact %q: %v", artifact.Name, err)
		}
	}
	backendBody := []byte("controlled backend archive")
	backendPath := filepath.Join(sourceRoot, "backend.zip")
	if err := os.WriteFile(backendPath, backendBody, 0o644); err != nil {
		t.Fatalf("write controlled backend artifact: %v", err)
	}
	scopes := newScopes(t, issuer)
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("controlled stale-stage recovery used the network")
		return nil, nil
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:            scope,
		Name:             "recovery-model",
		Reference:        models.ModelReference{NameOrURI: modelDirectory},
		Artifacts:        modelArtifacts,
		Backend:          "localai-vibevoice",
		BackendReference: models.ModelReference{NameOrURI: backendPath},
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.zip", Bytes: int64(len(backendBody)), SHA256: sha256Hex(backendBody),
		}},
	}
	prepared, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil || prepared.Outcome != models.AssetPreparationPrepared {
		t.Fatalf("initial PrepareModelAssets = (%#v, %v), want prepared controlled runtime", prepared, err)
	}
	inspection := inspectNamedGenericRuntime(t, service, scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, modelBodies, backendBody)
	return genericRuntimeRecoveryFixture{
		cacheDirectory: cacheDirectory,
		sourceRoot:     sourceRoot,
		scope:          scope,
		service:        service,
		request:        request,
		modelBodies:    modelBodies,
		backendBody:    backendBody,
		inspection:     inspection,
	}
}

func assertGenericRuntimeRecoveryContents(
	t *testing.T,
	inspection assets.RuntimeCacheInspection,
	modelBodies map[string][]byte,
	backendBody []byte,
) {
	t.Helper()
	assertGenericRuntimeRecoveryReady(t, inspection, len(modelBodies))
	assertGenericRuntimeRecoveryModelFiles(t, inspection, modelBodies)
	assertFileBody(t, inspection.BackendFiles[0], backendBody)
}

func assertGenericRuntimeRecoveryReady(
	t *testing.T,
	inspection assets.RuntimeCacheInspection,
	expectedModelFiles int,
) {
	t.Helper()
	if !inspection.Supported || !inspection.Installed || !inspection.ManifestPresent ||
		!inspection.ManifestValid || !inspection.IntegrityVerified ||
		inspection.InstalledFileCount != expectedModelFiles ||
		len(inspection.ObservedArtifacts) != expectedModelFiles || !inspection.BackendRequired ||
		inspection.BackendInstalledFiles != 1 || len(inspection.BackendFiles) != 1 {
		t.Fatalf("runtime inspection = %#v, want verified multi-artifact model and backend", inspection)
	}
}

func assertGenericRuntimeRecoveryModelFiles(
	t *testing.T,
	inspection assets.RuntimeCacheInspection,
	modelBodies map[string][]byte,
) {
	t.Helper()
	observed := make(map[string]models.AssetArtifact, len(inspection.ObservedArtifacts))
	var cacheBytes int64
	for _, artifact := range inspection.ObservedArtifacts {
		observed[artifact.Name] = artifact
	}
	for name, body := range modelBodies {
		artifact, ok := observed[name]
		if !ok || artifact.Bytes != int64(len(body)) || artifact.SHA256 != sha256Hex(body) {
			t.Fatalf("managed artifact %q = %#v, want %d bytes with digest %s", name, artifact, len(body), sha256Hex(body))
		}
		assertFileBody(t, filepath.Join(inspection.CachePath, filepath.FromSlash(name)), body)
		cacheBytes += int64(len(body))
	}
	if inspection.CacheBytes != cacheBytes {
		t.Fatalf("managed cache bytes = %d, want %d", inspection.CacheBytes, cacheBytes)
	}
}

func removeGenericRuntimeCommit(t *testing.T, fixture genericRuntimeRecoveryFixture) string {
	t.Helper()
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	if err := os.RemoveAll(fixture.inspection.CachePath); err != nil {
		t.Fatalf("remove committed managed runtime: %v", err)
	}
	if err := os.Remove(filepath.Join(root, metadataFileName)); err != nil {
		t.Fatalf("remove committed managed metadata: %v", err)
	}
	return root
}

func writeGenericRuntimeRevisionStage(t *testing.T, stagePath string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(stagePath, 0o755); err != nil {
		t.Fatalf("create managed runtime revision stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagePath, "abandoned.bin"), body, 0o644); err != nil {
		t.Fatalf("write managed runtime revision stage: %v", err)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q stat error = %v, want not-exist", path, err)
	}
}

func TestPrepareGenericRuntimeWaiterPreservesLiveOwnerStages(t *testing.T) {
	t.Parallel()

	fixture := newGenericRuntimeRecoveryFixture(t, "live-managed-runtime-owner")
	owner, ownerPath, coordination := holdGenericRuntimeLock(t, fixture, fixture.request.Name)
	ownerReleased := false
	waiterJoined := false
	waiterContext, cancel := context.WithCancel(context.Background())
	done := make(chan genericRuntimePrepareOutcome, 1)
	t.Cleanup(func() {
		cancel()
		if !ownerReleased {
			_ = owner.Close()
		}
		if !waiterJoined {
			select {
			case <-done:
			case <-time.After(genericRuntimeWaitCeiling):
				t.Errorf("same-model waiter did not stop during test cleanup")
			}
		}
	})

	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	revisionStage := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStage := filepath.Join(root, metadataFileName+".partial")
	writeGenericRuntimeRevisionStage(t, revisionStage, []byte("active owner's revision stage"))
	if err := os.WriteFile(metadataStage, []byte("active owner's metadata stage"), 0o644); err != nil {
		t.Fatalf("write active owner's metadata stage: %v", err)
	}
	metadataBefore, err := os.ReadFile(filepath.Join(root, metadataFileName))
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}

	observer := &observingRuntimeCoordination{
		delegate: coordination,
		attempts: make(chan string, 1),
	}
	fixture.service.coordination = observer
	request := fixture.request
	request.Offline = true
	done = prepareGenericAssetsAsync(fixture.service, waiterContext, request)
	waitedPath := waitForRuntimeLockAttempt(t, observer.attempts)
	if filepath.Clean(waitedPath) != filepath.Clean(ownerPath) {
		t.Fatalf("waiter lock path = %q, owner lock path = %q", waitedPath, ownerPath)
	}
	assertFileBody(t, filepath.Join(revisionStage, "abandoned.bin"), []byte("active owner's revision stage"))
	assertFileBody(t, metadataStage, []byte("active owner's metadata stage"))
	assertPrepareStillWaiting(t, done)

	if err := owner.Close(); err != nil {
		t.Fatalf("release active model lock: %v", err)
	}
	ownerReleased = true
	outcome := waitForGenericPrepare(t, done)
	waiterJoined = true
	if outcome.err != nil || outcome.result.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("same-model waiter result = (%#v, %v), want already-available commit", outcome.result, outcome.err)
	}
	inspection := inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, fixture.modelBodies, fixture.backendBody)
	if inspection.Revision != fixture.inspection.Revision {
		t.Fatalf("waiter changed committed revision from %q to %q", fixture.inspection.Revision, inspection.Revision)
	}
	assertPathAbsent(t, revisionStage)
	assertPathAbsent(t, metadataStage)
	assertFileBody(t, filepath.Join(root, metadataFileName), metadataBefore)
}

func TestPrepareGenericRuntimeWaiterCancellationPreservesLiveOwnerStages(t *testing.T) {
	t.Parallel()

	fixture := newGenericRuntimeRecoveryFixture(t, "cancel-managed-runtime-waiter")
	owner, ownerPath, coordination := holdGenericRuntimeLock(t, fixture, fixture.request.Name)
	ownerReleased := false
	waiterJoined := false
	waiterContext, cancel := context.WithCancel(context.Background())
	done := make(chan genericRuntimePrepareOutcome, 1)
	t.Cleanup(func() {
		cancel()
		if !ownerReleased {
			_ = owner.Close()
		}
		if !waiterJoined {
			select {
			case <-done:
			case <-time.After(genericRuntimeWaitCeiling):
				t.Errorf("cancelled same-model waiter did not stop during test cleanup")
			}
		}
	})

	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	revisionStage := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStage := filepath.Join(root, metadataFileName+".partial")
	metadataBefore, err := os.ReadFile(filepath.Join(root, metadataFileName))
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}
	writeGenericRuntimeRevisionStage(t, revisionStage, []byte("active owner's revision stage"))
	if err := os.WriteFile(metadataStage, []byte("active owner's metadata stage"), 0o644); err != nil {
		t.Fatalf("write active owner's metadata stage: %v", err)
	}
	observer := &observingRuntimeCoordination{
		delegate: coordination,
		attempts: make(chan string, 1),
	}
	fixture.service.coordination = observer
	request := fixture.request
	request.Offline = true
	done = prepareGenericAssetsAsync(fixture.service, waiterContext, request)
	waitedPath := waitForRuntimeLockAttempt(t, observer.attempts)
	if filepath.Clean(waitedPath) != filepath.Clean(ownerPath) {
		t.Fatalf("waiter lock path = %q, owner lock path = %q", waitedPath, ownerPath)
	}
	cancel()
	outcome := waitForGenericPrepare(t, done)
	waiterJoined = true
	if !errors.Is(outcome.err, models.ErrAssetCancelled) {
		t.Fatalf("cancelled same-model waiter error = %v, want ErrAssetCancelled", outcome.err)
	}
	assertFileBody(t, filepath.Join(revisionStage, "abandoned.bin"), []byte("active owner's revision stage"))
	assertFileBody(t, metadataStage, []byte("active owner's metadata stage"))
	assertGenericRuntimeCommittedState(t, fixture, metadataBefore)

	if err := owner.Close(); err != nil {
		t.Fatalf("release active model lock: %v", err)
	}
	ownerReleased = true
}

func TestPrepareGenericRuntimeAllowsDistinctModelPublicationWhileLocked(t *testing.T) {
	t.Parallel()

	fixture := newGenericRuntimeRecoveryFixture(t, "distinct-managed-runtime-locks")
	owner, ownerPath, coordination := holdGenericRuntimeLock(t, fixture, fixture.request.Name)
	ownerReleased := false
	waiterJoined := false
	waiterContext, cancel := context.WithCancel(context.Background())
	var done chan genericRuntimePrepareOutcome
	t.Cleanup(func() {
		cancel()
		if !ownerReleased {
			_ = owner.Close()
		}
		if done != nil && !waiterJoined {
			select {
			case <-done:
			case <-time.After(genericRuntimeWaitCeiling):
				t.Errorf("distinct-model preparation did not stop during test cleanup")
			}
		}
	})

	observer := &observingRuntimeCoordination{
		delegate: coordination,
		attempts: make(chan string, 1),
	}
	fixture.service.coordination = observer
	request := fixture.request
	request.Name = "independent-runtime-model"
	request.Offline = true
	done = prepareGenericAssetsAsync(fixture.service, waiterContext, request)
	waitedPath := waitForRuntimeLockAttempt(t, observer.attempts)
	if filepath.Clean(waitedPath) == filepath.Clean(ownerPath) {
		t.Fatalf("distinct model reused lock path %q", waitedPath)
	}
	outcome := waitForGenericPrepare(t, done)
	waiterJoined = true
	if outcome.err != nil {
		t.Fatalf("distinct-model PrepareModelAssets: %v", outcome.err)
	}
	inspection := inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, fixture.modelBodies, fixture.backendBody)
	assertGenericRuntimeRecoveryContents(
		t, inspectNamedGenericRuntime(t, fixture.service, fixture.scope, fixture.request.Name),
		fixture.modelBodies, fixture.backendBody,
	)

	if err := owner.Close(); err != nil {
		t.Fatalf("release first model lock: %v", err)
	}
	ownerReleased = true
}

func TestPrepareGenericRuntimeRestoresCommittedRuntimeAfterManagedPublicationFailures(t *testing.T) {
	cases := []struct {
		name               string
		failure            string
		wantError          error
		wantCause          bool
		cleanupUnavailable bool
	}{
		{name: "copy", failure: "copy", wantError: models.ErrAssetPreparationInterrupted, wantCause: true},
		{name: "verification", failure: "verification", wantError: models.ErrAssetPreparationInterrupted, wantCause: true},
		{name: "snapshot rename", failure: "snapshot rename", wantError: models.ErrAssetPreparationInterrupted},
		{name: "metadata rename", failure: "metadata rename", wantError: models.ErrAssetPreparationInterrupted},
		{name: "cancellation after snapshot promotion", failure: "cancellation", wantError: models.ErrAssetCancelled},
		{name: "metadata disk failure", failure: "disk", wantError: models.ErrAssetPreparationInterrupted, wantCause: true},
		{name: "stale-stage cleanup failure", failure: "cleanup", wantError: models.ErrAssetPreparationInterrupted, wantCause: true, cleanupUnavailable: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newGenericRuntimeRecoveryFixture(t, "managed-runtime-fault-"+strings.ReplaceAll(testCase.failure, " ", "-"))
			request, newBodies := replaceGenericRuntimeFixtureInputs(t, fixture)
			paths := genericRuntimePathsForReplacement(t, fixture, request, newBodies)
			if filepath.Base(paths.finalPath) == fixture.inspection.Revision {
				t.Fatal("replacement inputs did not produce a new managed revision")
			}
			metadataBefore, err := os.ReadFile(filepath.Join(paths.root, metadataFileName))
			if err != nil {
				t.Fatalf("read committed metadata: %v", err)
			}
			if testCase.cleanupUnavailable {
				writeGenericRuntimeRevisionStage(t, paths.stagePath, []byte("stale stage owned by failed cleanup"))
			}
			ctx, cancel, cause := injectGenericRuntimeFailure(t, fixture.service, paths, testCase.failure)
			defer cancel()
			_, err = fixture.service.PrepareModelAssets(ctx, request)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("PrepareModelAssets error = %v, want %v", err, testCase.wantError)
			}
			if testCase.wantCause && !errors.Is(err, cause) {
				t.Fatalf("PrepareModelAssets error = %v, want injected cause %v", err, cause)
			}
			assertPathAbsent(t, paths.finalPath)
			assertGenericRuntimeCommittedState(t, fixture, metadataBefore)
			if testCase.cleanupUnavailable {
				assertFileBody(t, filepath.Join(paths.stagePath, "abandoned.bin"), []byte("stale stage owned by failed cleanup"))
				assertPathAbsent(t, filepath.Join(paths.root, metadataFileName+".previous"))
				assertPathAbsent(t, filepath.Join(paths.root, metadataFileName+".partial"))
			} else {
				assertNoGenericRuntimeTransients(t, paths.root)
			}
		})
	}
}

type genericRuntimePrepareOutcome struct {
	result models.PrepareModelAssetsResult
	err    error
}

type genericRuntimeTestCoordination interface {
	Lock(context.Context, string) (io.Closer, error)
}

type recordingRuntimeCoordination struct {
	delegate genericRuntimeTestCoordination
	path     string
}

func (coordination *recordingRuntimeCoordination) Lock(ctx context.Context, path string) (io.Closer, error) {
	coordination.path = path
	return coordination.delegate.Lock(ctx, path)
}

type observingRuntimeCoordination struct {
	delegate genericRuntimeTestCoordination
	attempts chan string
}

func (coordination *observingRuntimeCoordination) Lock(ctx context.Context, path string) (io.Closer, error) {
	if strings.Contains(filepath.ToSlash(path), "/.you-asset-locks/runtime/") {
		select {
		case coordination.attempts <- path:
		default:
		}
	}
	return coordination.delegate.Lock(ctx, path)
}

type genericRuntimePublicationPaths struct {
	root              string
	stagePath         string
	finalPath         string
	metadataPath      string
	metadataStagePath string
}

const genericRuntimeWaitCeiling = 15 * time.Second

func holdGenericRuntimeLock(
	t *testing.T,
	fixture genericRuntimeRecoveryFixture,
	modelName string,
) (io.Closer, string, genericRuntimeTestCoordination) {
	t.Helper()
	coordination := fixture.service.coordination
	recorder := &recordingRuntimeCoordination{delegate: coordination}
	fixture.service.coordination = recorder
	lock, err := fixture.service.lockGenericRuntime(context.Background(), fixture.cacheDirectory, modelName)
	fixture.service.coordination = coordination
	if err != nil {
		t.Fatalf("acquire test-owned model runtime lock: %v", err)
	}
	if recorder.path == "" {
		_ = lock.Close()
		t.Fatal("model runtime lock service did not record its path")
	}
	return lock, recorder.path, coordination
}

func prepareGenericAssetsAsync(
	service *service,
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) chan genericRuntimePrepareOutcome {
	done := make(chan genericRuntimePrepareOutcome, 1)
	go func() {
		result, err := service.PrepareModelAssets(ctx, request)
		done <- genericRuntimePrepareOutcome{result: result, err: err}
	}()
	return done
}

func waitForRuntimeLockAttempt(t *testing.T, attempts <-chan string) string {
	t.Helper()
	select {
	case path := <-attempts:
		return path
	case <-time.After(genericRuntimeWaitCeiling):
		t.Fatal("preparation did not attempt the managed runtime lock")
		return ""
	}
}

func waitForGenericPrepare(t *testing.T, done <-chan genericRuntimePrepareOutcome) genericRuntimePrepareOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(genericRuntimeWaitCeiling):
		t.Fatal("managed runtime preparation did not complete before the safety ceiling")
		return genericRuntimePrepareOutcome{}
	}
}

func assertPrepareStillWaiting(t *testing.T, done <-chan genericRuntimePrepareOutcome) {
	t.Helper()
	select {
	case outcome := <-done:
		t.Fatalf("same-model waiter completed before lock release: (%#v, %v)", outcome.result, outcome.err)
	default:
	}
}

func replaceGenericRuntimeFixtureInputs(
	t *testing.T,
	fixture genericRuntimeRecoveryFixture,
) (models.PrepareModelAssetsRequest, map[string][]byte) {
	t.Helper()
	request := fixture.request
	request.Artifacts = append([]models.AssetRequirement(nil), fixture.request.Artifacts...)
	newBodies := make(map[string][]byte, len(request.Artifacts))
	for index, artifact := range request.Artifacts {
		body := append(append([]byte(nil), fixture.modelBodies[artifact.Name]...), []byte(" replacement")...)
		if err := os.WriteFile(filepath.Join(fixture.sourceRoot, "model", filepath.FromSlash(artifact.Name)), body, 0o644); err != nil {
			t.Fatalf("write replacement model artifact %q: %v", artifact.Name, err)
		}
		newBodies[artifact.Name] = body
		request.Artifacts[index].Bytes = int64(len(body))
		request.Artifacts[index].SHA256 = sha256Hex(body)
	}
	return request, newBodies
}

func genericRuntimePathsForReplacement(
	t *testing.T,
	fixture genericRuntimeRecoveryFixture,
	request models.PrepareModelAssetsRequest,
	bodies map[string][]byte,
) genericRuntimePublicationPaths {
	t.Helper()
	source, err := parseGenericSource(request.Reference.NameOrURI)
	if err != nil {
		t.Fatalf("parse replacement model source: %v", err)
	}
	artifacts := make([]models.AssetArtifact, 0, len(request.Artifacts))
	for _, artifact := range request.Artifacts {
		body, ok := bodies[artifact.Name]
		if !ok {
			t.Fatalf("replacement body for %q is missing", artifact.Name)
		}
		artifacts = append(artifacts, models.AssetArtifact{
			Name: artifact.Name, Bytes: int64(len(body)), SHA256: sha256Hex(body),
		})
	}
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(request.Name))
	stageName := genericRuntimeRevision(source, artifacts) + ".partial"
	stagePath := filepath.Join(root, stageName)
	return genericRuntimePublicationPaths{
		root:              root,
		stagePath:         stagePath,
		finalPath:         strings.TrimSuffix(stagePath, ".partial"),
		metadataPath:      filepath.Join(root, metadataFileName),
		metadataStagePath: filepath.Join(root, metadataFileName+".partial"),
	}
}

func injectGenericRuntimeFailure(
	t *testing.T,
	service *service,
	paths genericRuntimePublicationPaths,
	failure string,
) (context.Context, context.CancelFunc, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cause := errors.New("injected managed runtime " + failure + " failure")
	switch failure {
	case "copy":
		injectGenericRuntimeCopyFailure(service, paths, cause)
	case "verification":
		injectGenericRuntimeVerificationFailure(service, paths, cause)
	case "snapshot rename":
		injectGenericRuntimeSnapshotRenameFailure(service, paths, cause)
	case "metadata rename":
		injectGenericRuntimeMetadataRenameFailure(service, paths, cause)
	case "cancellation":
		injectGenericRuntimeCancellationAfterPromotion(service, paths, cancel)
		cause = context.Canceled
	case "disk":
		injectGenericRuntimeMetadataWriteFailure(service, paths, cause)
	case "cleanup":
		injectGenericRuntimeCleanupFailure(service, paths, cause)
	default:
		t.Fatalf("unknown managed runtime failure %q", failure)
	}
	return ctx, cancel, cause
}

func injectGenericRuntimeCopyFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	original := service.createFile
	service.createFile = func(path string) (io.WriteCloser, error) {
		if filepath.Clean(path) == filepath.Join(paths.stagePath, "weights.gguf") {
			return nil, cause
		}
		return original(path)
	}
}

func injectGenericRuntimeVerificationFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	original := service.inspectPath
	failed := false
	service.inspectPath = func(path string) (os.FileInfo, error) {
		if !failed && filepath.Clean(path) == filepath.Join(paths.stagePath, "weights.gguf") {
			failed = true
			return nil, cause
		}
		return original(path)
	}
}

func injectGenericRuntimeSnapshotRenameFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	original := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		if filepath.Clean(oldPath) == filepath.Clean(paths.stagePath) &&
			filepath.Clean(newPath) == filepath.Clean(paths.finalPath) {
			return cause
		}
		return original(oldPath, newPath)
	}
}

func injectGenericRuntimeMetadataRenameFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	original := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		if filepath.Clean(oldPath) == filepath.Clean(paths.metadataStagePath) &&
			filepath.Clean(newPath) == filepath.Clean(paths.metadataPath) {
			return cause
		}
		return original(oldPath, newPath)
	}
}

func injectGenericRuntimeCancellationAfterPromotion(
	service *service,
	paths genericRuntimePublicationPaths,
	cancel context.CancelFunc,
) {
	original := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		err := original(oldPath, newPath)
		if err == nil && filepath.Clean(oldPath) == filepath.Clean(paths.stagePath) &&
			filepath.Clean(newPath) == filepath.Clean(paths.finalPath) {
			cancel()
		}
		return err
	}
}

func injectGenericRuntimeMetadataWriteFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	original := service.writeFile
	service.writeFile = func(path string, body []byte, mode os.FileMode) error {
		if filepath.Clean(path) == filepath.Clean(paths.metadataStagePath) {
			return cause
		}
		return original(path, body, mode)
	}
}

func injectGenericRuntimeCleanupFailure(service *service, paths genericRuntimePublicationPaths, cause error) {
	abandonedPath := filepath.Join(paths.stagePath, "abandoned.bin")
	original := service.removePath
	service.removePath = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(abandonedPath) {
			return cause
		}
		return original(path)
	}
}

func assertGenericRuntimeCommittedState(
	t *testing.T,
	fixture genericRuntimeRecoveryFixture,
	metadataBefore []byte,
) {
	t.Helper()
	inspection := inspectNamedGenericRuntime(t, fixture.service, fixture.scope, fixture.request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, fixture.modelBodies, fixture.backendBody)
	if inspection.Revision != fixture.inspection.Revision ||
		filepath.Clean(inspection.CachePath) != filepath.Clean(fixture.inspection.CachePath) {
		t.Fatalf("failed publication changed committed identity: before=%#v after=%#v", fixture.inspection, inspection)
	}
	if metadataBefore != nil {
		metadataAfter, err := os.ReadFile(filepath.Join(
			fixture.cacheDirectory, canonicalModelName(fixture.request.Name), metadataFileName,
		))
		if err != nil || !bytes.Equal(metadataBefore, metadataAfter) {
			t.Fatalf("failed publication changed committed metadata: before=%q after=%q err=%v", metadataBefore, metadataAfter, err)
		}
	}
}

func assertNoGenericRuntimeTransients(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read managed runtime root %q: %v", root, err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".partial") || strings.HasSuffix(entry.Name(), ".previous") {
			t.Fatalf("managed runtime left transaction residue %q", entry.Name())
		}
	}
}
