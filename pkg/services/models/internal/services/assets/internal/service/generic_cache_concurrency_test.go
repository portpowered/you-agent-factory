package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestPrepareGenericAssetsSharesConcurrentFirstDownload(t *testing.T) {
	t.Parallel()

	const callers = 8

	body := []byte("concurrent payload")
	digest := sha256Hex(body)
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	var downloadStartedOnce sync.Once
	var releaseDownloadOnce sync.Once
	var downloads atomic.Int32
	client := genericManifestClient("weights.bin", body, func() []byte {
		downloads.Add(1)
		downloadStartedOnce.Do(func() { close(downloadStarted) })
		<-releaseDownload
		return body
	})
	scopes := newScopes(t, "generic-singleflight")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	var joined atomic.Int32
	allJoined := make(chan struct{})
	var allJoinedOnce sync.Once
	service.cacheJoinObserver = func() {
		if joined.Add(1) == callers-1 {
			allJoinedOnce.Do(func() { close(allJoined) })
		}
	}
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(body))}},
	}
	results := make(chan error, callers)
	for index := 0; index < callers; index++ {
		go func() {
			_, err := service.PrepareModelAssets(context.Background(), request)
			results <- err
		}()
	}
	t.Cleanup(func() { releaseDownloadOnce.Do(func() { close(releaseDownload) }) })
	<-downloadStarted
	<-allJoined
	releaseDownloadOnce.Do(func() { close(releaseDownload) })
	for index := 0; index < callers; index++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent preparation %d: %v", index, err)
		}
	}
	if got := joined.Load(); got != callers-1 {
		t.Fatalf("joined caller count = %d, want %d", got, callers-1)
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("download count = %d, want 1", got)
	}
}

func TestPrepareGenericAssetsSharesConcurrentFirstDownloadAcrossServices(t *testing.T) {
	t.Parallel()

	body := []byte("cross-service concurrent payload")
	digest := sha256Hex(body)
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	var downloadStartedOnce sync.Once
	var releaseDownloadOnce sync.Once
	var downloads atomic.Int32
	client := genericManifestClient("weights.bin", body, func() []byte {
		downloads.Add(1)
		downloadStartedOnce.Do(func() { close(downloadStarted) })
		<-releaseDownload
		return body
	})
	cacheDirectory := t.TempDir()
	firstScopes := newScopes(t, "generic-cross-service-first")
	secondScopes := newScopes(t, "generic-cross-service-second")
	firstScope := openScope(t, firstScopes, cacheDirectory, models.RuntimeConfig{})
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	firstService := newGenericService(t, firstScopes, client, func(string) string { return "" })
	secondService := newGenericService(t, secondScopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Name:      "shared-model",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(body))}},
	}
	firstRequest := request
	firstRequest.Scope = firstScope
	secondRequest := request
	secondRequest.Scope = secondScope
	results := make(chan error, 2)
	go func() {
		_, err := firstService.PrepareModelAssets(context.Background(), firstRequest)
		results <- err
	}()
	<-downloadStarted
	go func() {
		_, err := secondService.PrepareModelAssets(context.Background(), secondRequest)
		results <- err
	}()
	t.Cleanup(func() { releaseDownloadOnce.Do(func() { close(releaseDownload) }) })
	releaseDownloadOnce.Do(func() { close(releaseDownload) })
	for index := 0; index < 2; index++ {
		if err := <-results; err != nil {
			t.Fatalf("cross-service preparation %d: %v", index, err)
		}
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("cross-service download count = %d, want 1", got)
	}
}

func TestPrepareGenericAssetsAllowsDistinctCrossServiceTransfersToOverlap(t *testing.T) {
	t.Parallel()

	body := []byte("distinct concurrent payload")
	digest := sha256Hex(body)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseTransfers := make(chan struct{})
	var releaseTransfersOnce sync.Once
	var starts atomic.Int32
	var firstStartedOnce sync.Once
	var secondStartedOnce sync.Once
	baseClient := genericManifestClient("weights.bin", body, func() []byte { return body })
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet && !strings.HasPrefix(request.URL.Path, "/models/") {
			switch starts.Add(1) {
			case 1:
				firstStartedOnce.Do(func() { close(firstStarted) })
			case 2:
				secondStartedOnce.Do(func() { close(secondStarted) })
			}
			<-releaseTransfers
		}
		return baseClient.Do(request)
	})
	t.Cleanup(func() { releaseTransfersOnce.Do(func() { close(releaseTransfers) }) })
	cacheDirectory := t.TempDir()
	firstScopes := newScopes(t, "generic-distinct-first")
	secondScopes := newScopes(t, "generic-distinct-second")
	firstScope := openScope(t, firstScopes, cacheDirectory, models.RuntimeConfig{})
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	firstService := newGenericService(t, firstScopes, client, func(string) string { return "" })
	secondService := newGenericService(t, secondScopes, client, func(string) string { return "" })
	firstRequest := models.PrepareModelAssetsRequest{
		Scope:     firstScope,
		Name:      "distinct-model-a",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo-a/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(body))}},
	}
	secondRequest := firstRequest
	secondRequest.Scope = secondScope
	secondRequest.Name = "distinct-model-b"
	secondRequest.Reference = models.ModelReference{NameOrURI: "hf://owner/repo-b/weights.bin@" + genericTestRevision}
	results := make(chan error, 2)
	go func() {
		_, err := firstService.PrepareModelAssets(context.Background(), firstRequest)
		results <- err
	}()
	<-firstStarted
	go func() {
		_, err := secondService.PrepareModelAssets(context.Background(), secondRequest)
		results <- err
	}()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("distinct identity transfer waited behind the first identity")
	}
	releaseTransfersOnce.Do(func() { close(releaseTransfers) })
	for index := 0; index < 2; index++ {
		if err := <-results; err != nil {
			t.Fatalf("distinct cross-service preparation %d: %v", index, err)
		}
	}
	if got := starts.Load(); got != 2 {
		t.Fatalf("distinct transfer count = %d, want 2", got)
	}
}

func TestPrepareGenericAssetsCancelsCrossServiceWaiter(t *testing.T) {
	t.Parallel()

	body := []byte("cross-service cancellation payload")
	digest := sha256Hex(body)
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	var downloadStartedOnce sync.Once
	var releaseDownloadOnce sync.Once
	var downloads atomic.Int32
	client := genericManifestClient("weights.bin", body, func() []byte {
		downloads.Add(1)
		downloadStartedOnce.Do(func() { close(downloadStarted) })
		<-releaseDownload
		return body
	})
	cacheDirectory := t.TempDir()
	firstScopes := newScopes(t, "generic-cross-service-cancel-first")
	secondScopes := newScopes(t, "generic-cross-service-cancel-second")
	firstScope := openScope(t, firstScopes, cacheDirectory, models.RuntimeConfig{})
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	firstService := newGenericService(t, firstScopes, client, func(string) string { return "" })
	secondService := newGenericService(t, secondScopes, client, func(string) string { return "" })
	waiter := &observingStagingCoordination{
		delegate:  mustPlatformLockingService(t),
		attempted: make(chan struct{}),
	}
	secondService.coordination = waiter
	request := models.PrepareModelAssetsRequest{
		Name:      "cancelled-shared-model",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(body))}},
	}
	firstRequest := request
	firstRequest.Scope = firstScope
	secondRequest := request
	secondRequest.Scope = secondScope
	firstResult := make(chan error, 1)
	go func() {
		_, err := firstService.PrepareModelAssets(context.Background(), firstRequest)
		firstResult <- err
	}()
	<-downloadStarted
	waiterContext, cancel := context.WithCancel(context.Background())
	secondResult := make(chan error, 1)
	go func() {
		_, err := secondService.PrepareModelAssets(waiterContext, secondRequest)
		secondResult <- err
	}()
	<-waiter.attempted
	cancel()
	if err := <-secondResult; !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("cross-service waiter error = %v, want typed cancellation", err)
	}
	releaseDownloadOnce.Do(func() { close(releaseDownload) })
	if err := <-firstResult; err != nil {
		t.Fatalf("owner preparation after waiter cancellation: %v", err)
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("cross-service cancellation download count = %d, want 1", got)
	}
}

func TestPrepareGenericAssetsRecoversAfterStagingAccessDenied(t *testing.T) {
	t.Parallel()

	body := []byte("access denial recovery payload")
	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "generic-access-denied")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, genericManifestClient("weights.bin", body, func() []byte { return body }), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "access-denied-model",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: sha256Hex(body), Bytes: int64(len(body))}},
	}
	prepared, err := service.PrepareModelAssets(context.Background(), request)
	requireGenericPreparationOutcome(t, prepared, err, models.AssetPreparationPrepared, "initial")
	artifactPath, snapshotPath := genericSnapshotPaths(t, cacheDirectory, request)
	assertGenericArtifact(t, artifactPath, body, "initial snapshot")

	service.coordination = rejectingStagingCoordination{err: errors.New("access denied")}
	assertStagingAccessDenied(t, service, request)
	assertGenericArtifact(t, artifactPath, body, "snapshot after access denial")
	assertNoPartialGenericSnapshot(t, snapshotPath)

	service.coordination = mustPlatformLockingService(t)
	retried, err := service.PrepareModelAssets(context.Background(), request)
	requireGenericPreparationOutcome(t, retried, err, models.AssetPreparationAlreadyAvailable, "retry after access denial")
}

func requireGenericPreparationOutcome(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	err error,
	want models.AssetPreparationOutcome,
	label string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s preparation: %v", label, err)
	}
	if result.Outcome != want {
		t.Fatalf("%s outcome = %v, want %v", label, result.Outcome, want)
	}
}

func genericSnapshotPaths(
	t *testing.T,
	cacheDirectory string,
	request models.PrepareModelAssetsRequest,
) (string, string) {
	t.Helper()
	source, err := parseGenericSource(request.Reference.NameOrURI)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	snapshotPath := filepath.Join(
		cacheDirectory, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{{requirement: request.Artifacts[0]}}),
	)
	return filepath.Join(snapshotPath, request.Artifacts[0].Name), snapshotPath
}

func assertGenericArtifact(t *testing.T, path string, want []byte, label string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("%s = %q, %v; want %q", label, got, err, want)
	}
}

func assertStagingAccessDenied(t *testing.T, service *service, request models.PrepareModelAssetsRequest) {
	t.Helper()
	_, err := service.PrepareModelAssets(context.Background(), request)
	if err == nil {
		t.Fatal("access-denied preparation error = nil")
	}
	var stageErr *models.PullStageError
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.As(err, &stageErr) ||
		stageErr.Stage != models.PullStageCacheInstallation {
		t.Fatalf("access-denied error = %v, want cache-installation interruption", err)
	}
}

func assertNoPartialGenericSnapshot(t *testing.T, snapshotPath string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(snapshotPath))
	if err != nil {
		t.Fatalf("read content cache after access denial: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".partial") {
			t.Fatalf("access denial left partial snapshot %q", entry.Name())
		}
	}
}

type observingStagingCoordination struct {
	delegate  platformlocking.Service
	attempted chan struct{}
	once      sync.Once
}

func mustPlatformLockingService(t *testing.T) platformlocking.Service {
	t.Helper()
	service, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("construct asset coordination: %v", err)
	}
	return service
}

func (coordination *observingStagingCoordination) Lock(ctx context.Context, path string) (io.Closer, error) {
	coordination.once.Do(func() { close(coordination.attempted) })
	return coordination.delegate.Lock(ctx, path)
}

type rejectingStagingCoordination struct {
	err error
}

func (coordination rejectingStagingCoordination) Lock(context.Context, string) (io.Closer, error) {
	return nil, coordination.err
}
