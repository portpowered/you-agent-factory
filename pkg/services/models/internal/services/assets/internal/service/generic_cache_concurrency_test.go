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

func TestAcquireGenericCacheConcurrentCallersReceiveCommittedPaths(t *testing.T) {
	t.Parallel()

	const callers = 8

	body := []byte("concurrent committed backend")
	digest := sha256Hex(body)
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	var downloadStartedOnce sync.Once
	var releaseDownloadOnce sync.Once
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		downloadStartedOnce.Do(func() { close(downloadStarted) })
		<-releaseDownload
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})
	scopes := newScopes(t, "generic-committed-concurrent")
	cacheDirectory := t.TempDir()
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	var joined atomic.Int32
	allJoined := make(chan struct{})
	var allJoinedOnce sync.Once
	service.cacheJoinObserver = func() {
		if joined.Add(1) == callers-1 {
			allJoinedOnce.Do(func() { close(allJoined) })
		}
	}
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/backend.zip@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "backend.zip", revision: genericTestRevision,
	}
	artifact := genericArtifact{
		requirement: models.AssetRequirement{
			Name: "backend.zip", Bytes: int64(len(body)), SHA256: digest,
		},
		metadataResolved: true,
	}
	results := make(chan struct {
		result genericCacheResult
		err    error
	}, callers)
	for index := 0; index < callers; index++ {
		go func() {
			result, err := service.acquireGenericCache(
				context.Background(), assetKindBackend, models.AssetArtifactKindBackend,
				source, []genericArtifact{artifact}, []string{cacheDirectory}, false,
			)
			results <- struct {
				result genericCacheResult
				err    error
			}{result: result, err: err}
		}()
	}
	t.Cleanup(func() { releaseDownloadOnce.Do(func() { close(releaseDownload) }) })
	<-downloadStarted
	<-allJoined
	releaseDownloadOnce.Do(func() { close(releaseDownload) })

	var committedPath string
	var preparedSeen bool
	for index := 0; index < callers; index++ {
		outcome := <-results
		if outcome.err != nil {
			t.Fatalf("concurrent preparation %d: %v", index, outcome.err)
		}
		assertLiveGenericCacheResult(t, outcome.result)
		preparedSeen = preparedSeen || outcome.result.prepared
		if index == 0 {
			committedPath = outcome.result.paths[0]
		} else if outcome.result.paths[0] != committedPath {
			t.Fatalf("concurrent path %q, want shared committed path %q", outcome.result.paths[0], committedPath)
		}
	}
	if !preparedSeen {
		t.Fatal("concurrent cache-miss results did not report a prepared snapshot")
	}
	if got := joined.Load(); got != callers-1 {
		t.Fatalf("joined caller count = %d, want %d", got, callers-1)
	}

	hit, err := service.acquireGenericCache(
		context.Background(), assetKindBackend, models.AssetArtifactKindBackend,
		source, []genericArtifact{artifact}, []string{cacheDirectory}, false,
	)
	if err != nil {
		t.Fatalf("cache hit: %v", err)
	}
	assertLiveGenericCacheResult(t, hit)
	if hit.paths[0] != committedPath {
		t.Fatalf("cache-hit path %q, want %q", hit.paths[0], committedPath)
	}
}

func assertLiveGenericCacheResult(t *testing.T, result genericCacheResult) {
	t.Helper()
	if len(result.paths) != 1 || !filepath.IsAbs(result.snapshotPath) ||
		strings.Contains(result.snapshotPath, ".partial") {
		t.Fatalf("generic cache result = %#v, want absolute committed snapshot", result)
	}
	for _, path := range result.paths {
		if !filepath.IsAbs(path) || strings.Contains(path, ".partial") {
			t.Fatalf("generic cache path = %q, want absolute non-partial path", path)
		}
		relative, err := filepath.Rel(result.snapshotPath, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("generic cache path %q escaped snapshot %q", path, result.snapshotPath)
		}
		info, err := os.Stat(path)
		if err != nil || info == nil || !info.Mode().IsRegular() {
			t.Fatalf("generic cache path stat = (%#v, %v), want existing regular file", info, err)
		}
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

func TestPrepareGenericAssetsRepairsLegacyCacheAcrossServices(t *testing.T) {
	t.Parallel()

	body := []byte("concurrent legacy repair payload")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	cacheDirectory := t.TempDir()
	legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
	firstScopes := newScopes(t, "generic-legacy-repair-first")
	secondScopes := newScopes(t, "generic-legacy-repair-second")
	firstScope := openScope(t, firstScopes, cacheDirectory, models.RuntimeConfig{})
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	firstService := newGenericService(t, firstScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("concurrent legacy repair must not contact the source")
	}), func(string) string { return "" })
	secondService := newGenericService(t, secondScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("concurrent legacy repair must not contact the source")
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Reference: models.ModelReference{NameOrURI: source.safe},
		Artifacts: []models.AssetRequirement{{
			Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	firstRequest := request
	firstRequest.Scope = firstScope
	secondRequest := request
	secondRequest.Scope = secondScope
	results := make(chan struct {
		result models.PrepareModelAssetsResult
		err    error
	}, 2)
	go func() {
		result, err := firstService.PrepareModelAssets(context.Background(), firstRequest)
		results <- struct {
			result models.PrepareModelAssetsResult
			err    error
		}{result: result, err: err}
	}()
	go func() {
		result, err := secondService.PrepareModelAssets(context.Background(), secondRequest)
		results <- struct {
			result models.PrepareModelAssetsResult
			err    error
		}{result: result, err: err}
	}()
	for index := 0; index < 2; index++ {
		outcome := <-results
		if outcome.err != nil || outcome.result.Outcome != models.AssetPreparationAlreadyAvailable ||
			outcome.result.Asset.Integrity != models.AssetIntegrityVerified {
			t.Fatalf("concurrent legacy repair %d = %#v, %v", index, outcome.result, outcome.err)
		}
	}
	assertFileBody(t, filepath.Join(correctedPath, "weights.bin"), body)
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("concurrent legacy snapshot = %v, want absent", err)
	}
}

func TestPrepareGenericAssetsSerializesObservedIdentityPublishAcrossServices(t *testing.T) {
	t.Parallel()

	fixture := newObservedIdentityRaceFixture(t)
	results := make(chan observedIdentityPreparation, 2)
	startObservedIdentityPreparation(results, fixture.firstService, fixture.firstRequest)
	<-fixture.transferStarted
	startObservedIdentityPreparation(results, fixture.secondService, fixture.secondRequest)
	<-fixture.lockState.secondAttempted
	fixture.releaseTransfer()

	prepared, reused := collectObservedIdentityPreparations(t, results, fixture.body, fixture.digest)
	if prepared.result.Outcome != models.AssetPreparationPrepared ||
		reused.result.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("observed identity outcomes = %#v and %#v, want prepared then reused", prepared.result.Outcome, reused.result.Outcome)
	}
	assertObservedIdentityCoordination(t, fixture)
	assertObservedIdentitySnapshot(t, fixture)
}

type observedIdentityPreparation struct {
	result models.PrepareModelAssetsResult
	err    error
}

type observedIdentityRaceFixture struct {
	body            []byte
	digest          string
	cacheDirectory  string
	reference       models.ModelReference
	firstService    *service
	secondService   *service
	firstRequest    models.PrepareModelAssetsRequest
	secondRequest   models.PrepareModelAssetsRequest
	transferStarted <-chan struct{}
	releaseTransfer func()
	transfers       *atomic.Int32
	lockState       *genericLockPathState
}

func newObservedIdentityRaceFixture(t *testing.T) *observedIdentityRaceFixture {
	t.Helper()
	body := []byte("observed identity concurrency payload")
	digest := sha256Hex(body)
	transferStarted := make(chan struct{})
	releaseTransfer := make(chan struct{})
	var transferOnce sync.Once
	var releaseOnce sync.Once
	transfers := &atomic.Int32{}
	manifestClient := genericManifestWithoutDigestClient("weights.bin", func() []byte { return body })
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet && !strings.HasPrefix(request.URL.Path, "/models/") && transfers.Add(1) == 1 {
			transferOnce.Do(func() { close(transferStarted) })
			<-releaseTransfer
		}
		return manifestClient.Do(request)
	})
	release := func() { releaseOnce.Do(func() { close(releaseTransfer) }) }
	t.Cleanup(release)

	cacheDirectory := t.TempDir()
	firstScopes := newScopes(t, "generic-observed-identity-first")
	secondScopes := newScopes(t, "generic-observed-identity-second")
	firstScope := openScope(t, firstScopes, cacheDirectory, models.RuntimeConfig{})
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	lockState := &genericLockPathState{secondAttempted: make(chan struct{})}
	firstService := newGenericService(t, firstScopes, client, func(string) string { return "" })
	secondService := newGenericService(t, secondScopes, client, func(string) string { return "" })
	firstService.coordination = newGenericLockPathRecorder(t, lockState)
	secondService.coordination = newGenericLockPathRecorder(t, lockState)
	reference := models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision}
	return &observedIdentityRaceFixture{
		body: body, digest: digest, cacheDirectory: cacheDirectory, reference: reference,
		firstService: firstService, secondService: secondService,
		firstRequest: models.PrepareModelAssetsRequest{
			Scope: firstScope, Name: "observed-identity-first", Reference: reference,
			Artifacts: []models.AssetRequirement{{Name: "weights.bin"}},
		},
		secondRequest: models.PrepareModelAssetsRequest{
			Scope: secondScope, Name: "observed-identity-second", Reference: reference,
			Artifacts: []models.AssetRequirement{{Name: "weights.bin", Bytes: int64(len(body)), SHA256: digest}},
		},
		transferStarted: transferStarted, releaseTransfer: release, transfers: transfers, lockState: lockState,
	}
}

func newGenericLockPathRecorder(t *testing.T, state *genericLockPathState) platformlocking.Service {
	t.Helper()
	return &genericLockPathRecorder{delegate: mustPlatformLockingService(t), state: state}
}

func startObservedIdentityPreparation(
	results chan<- observedIdentityPreparation,
	assetService *service,
	request models.PrepareModelAssetsRequest,
) {
	go func() {
		result, err := assetService.PrepareModelAssets(context.Background(), request)
		results <- observedIdentityPreparation{result: result, err: err}
	}()
}

func collectObservedIdentityPreparations(
	t *testing.T,
	results <-chan observedIdentityPreparation,
	body []byte,
	digest string,
) (observedIdentityPreparation, observedIdentityPreparation) {
	t.Helper()
	var prepared, reused observedIdentityPreparation
	for index := 0; index < 2; index++ {
		outcome := <-results
		if outcome.err != nil {
			t.Fatalf("observed identity preparation %d: %v", index, outcome.err)
		}
		assertObservedIdentityResult(t, outcome.result, body, digest, index)
		switch outcome.result.Outcome {
		case models.AssetPreparationPrepared:
			prepared = outcome
		case models.AssetPreparationAlreadyAvailable:
			reused = outcome
		default:
			t.Fatalf("observed identity outcome %d = %v, want prepared or reused", index, outcome.result.Outcome)
		}
	}
	return prepared, reused
}

func assertObservedIdentityResult(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	body []byte,
	digest string,
	index int,
) {
	t.Helper()
	if result.Asset.Integrity != models.AssetIntegrityVerified || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Bytes != int64(len(body)) || result.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("observed identity result %d = %#v, want verified observed artifact", index, result)
	}
}

func assertObservedIdentityCoordination(t *testing.T, fixture *observedIdentityRaceFixture) {
	t.Helper()
	if got := fixture.transfers.Load(); got != 1 {
		t.Fatalf("observed identity transfers = %d, want one transfer", got)
	}
	if got := fixture.lockState.cacheCalls.Load(); got != 2 {
		t.Fatalf("observed identity cache lock calls = %d, want two service owners", got)
	}
	fixture.lockState.mu.Lock()
	lockPath := fixture.lockState.path
	fixture.lockState.mu.Unlock()
	if lockPath == "" {
		t.Fatal("observed identity lock path is empty")
	}
}

func assertObservedIdentitySnapshot(t *testing.T, fixture *observedIdentityRaceFixture) {
	t.Helper()
	source := genericSource{
		kind: genericSourceHF, safe: fixture.reference.NameOrURI,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	artifacts := []genericArtifact{{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(fixture.body)), SHA256: fixture.digest,
	}}}
	correctedPath := filepath.Join(
		fixture.cacheDirectory, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, source, artifacts),
	)
	assertGenericArtifact(t, filepath.Join(correctedPath, "weights.bin"), fixture.body, "observed identity committed artifact")
	entries, err := os.ReadDir(filepath.Dir(correctedPath))
	if err != nil {
		t.Fatalf("read observed identity cache: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(correctedPath) {
		t.Fatalf("observed identity cache entries = %#v, want one committed snapshot", entries)
	}
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

type genericLockPathState struct {
	mu              sync.Mutex
	path            string
	cacheCalls      atomic.Int32
	secondAttempted chan struct{}
	secondOnce      sync.Once
}

type genericLockPathRecorder struct {
	delegate platformlocking.Service
	state    *genericLockPathState
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

func (coordination *genericLockPathRecorder) Lock(ctx context.Context, path string) (io.Closer, error) {
	isRuntimeLock := strings.Contains(filepath.ToSlash(path), "/.you-asset-locks/runtime/")
	if !isRuntimeLock && coordination.state.cacheCalls.Add(1) == 2 {
		coordination.state.secondOnce.Do(func() { close(coordination.state.secondAttempted) })
	}
	coordination.state.mu.Lock()
	if !isRuntimeLock {
		if coordination.state.path == "" {
			coordination.state.path = path
		} else if coordination.state.path != path {
			coordination.state.mu.Unlock()
			return nil, errors.New("generic cache ownership key changed with artifact facts")
		}
	}
	coordination.state.mu.Unlock()
	return coordination.delegate.Lock(ctx, path)
}

type rejectingStagingCoordination struct {
	err error
}

func (coordination rejectingStagingCoordination) Lock(context.Context, string) (io.Closer, error) {
	return nil, coordination.err
}

func TestGenericPrivatePlanningAndCachePreservation(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-private-planning")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private planning test must not use HTTP")
	}), func(string) string { return "" })

	assertGenericLocalRequirements(t, service)
	assertGenericOverlayPlanning(t)
	assertGenericCachePreservation(t, service)
}

func assertGenericLocalRequirements(t *testing.T, service *service) {
	t.Helper()

	localRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localRoot, "nested"), 0o755); err != nil {
		t.Fatalf("create local fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "root.bin"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write root fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "nested", "child.bin"), []byte("child"), 0o644); err != nil {
		t.Fatalf("write nested fixture: %v", err)
	}
	requirements, err := service.localRequirements(localRoot)
	if err != nil || len(requirements) != 2 || requirements[0].Name != "nested/child.bin" || requirements[1].Name != "root.bin" {
		t.Fatalf("localRequirements = %#v, %v, want sorted recursive files", requirements, err)
	}
	if _, err := service.localRequirements(filepath.Join(localRoot, "missing")); !errors.Is(err, models.ErrAssetSourceMissing) {
		t.Fatalf("missing localRequirements error = %v, want ErrAssetSourceMissing", err)
	}
}

func assertGenericOverlayPlanning(t *testing.T) {
	t.Helper()

	sourceText := "hf://owner/repo@" + genericTestRevision
	overlays := map[string]models.ModelOverlay{
		"Alias":  {Source: &sourceText},
		"alias ": {Source: func() *string { value := "hf://owner/other@" + genericTestRevision; return &value }()},
	}
	name, overlay, ok := genericOverlay(overlays, "alias")
	if !ok || name != "Alias" || overlay.Source == nil {
		t.Fatalf("genericOverlay = %q, %#v, %v", name, overlay, ok)
	}
	*overlay.Source = "mutated"
	if *overlays["Alias"].Source != sourceText {
		t.Fatal("genericOverlay returned a mutable source pointer")
	}
	safe := genericHFSafeReference(genericSource{owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision})
	if safe != "hf://owner/repo/weights.bin@"+genericTestRevision {
		t.Fatalf("genericHFSafeReference = %q", safe)
	}
	var revisionFailure *models.InvocationFailure
	if !errors.As(genericRevisionFailure(), &revisionFailure) || revisionFailure.Class != models.InvocationFailureClassRevisionResolution {
		t.Fatalf("genericRevisionFailure = %#v", genericRevisionFailure())
	}
}

func assertGenericCachePreservation(t *testing.T, service *service) {
	t.Helper()

	cachedPath := filepath.Join(t.TempDir(), "cached.bin")
	if err := os.WriteFile(cachedPath, []byte("cached"), 0o644); err != nil {
		t.Fatalf("write cached fixture: %v", err)
	}
	stagePath := t.TempDir()
	artifact := genericCachePath{
		artifact: models.AssetArtifact{Name: "nested/cached.bin", Bytes: int64(len("cached"))},
		path:     cachedPath,
	}
	if err := service.preserveGenericArtifact(context.Background(), artifact, artifact.artifact.Name, stagePath); err != nil {
		t.Fatalf("preserveGenericArtifact: %v", err)
	}
	preserved, err := os.ReadFile(filepath.Join(stagePath, "nested", "cached.bin"))
	if err != nil || string(preserved) != "cached" {
		t.Fatalf("preserved cached artifact = %q, %v", preserved, err)
	}
}

func TestGenericSnapshotDiscoveryAndSourcePlanning(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-snapshot-planning")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("source planning test must not use HTTP")
	}), func(string) string { return "" })
	assertGenericSnapshotDiscovery(t, service)
	assertGenericCacheRoots(t, service)
	assertGenericSourceResolution(t, service)
	assertGenericPreparationSelection(t)
}

func assertGenericSnapshotDiscovery(t *testing.T, service *service) {
	t.Helper()
	snapshotRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(snapshotRoot, "nested"), 0o755); err != nil {
		t.Fatalf("create snapshot fixture: %v", err)
	}
	for name, body := range map[string]string{
		"root.bin":            "root",
		"nested/child.bin":    "child",
		".hidden/ignored.bin": "ignored",
	} {
		path := filepath.Join(snapshotRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create snapshot parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write snapshot file: %v", err)
		}
	}
	requirements := service.discoverSnapshotRequirements(snapshotRoot)
	if len(requirements) != 2 || requirements[0].Name != "nested/child.bin" || requirements[1].Name != "root.bin" {
		t.Fatalf("discoverSnapshotRequirements = %#v, want visible sorted files", requirements)
	}
	if got := service.discoverSnapshotRequirements(filepath.Join(snapshotRoot, "missing")); got != nil {
		t.Fatalf("missing snapshot requirements = %#v, want nil", got)
	}
}

func assertGenericCacheRoots(t *testing.T, service *service) {
	t.Helper()
	scopeConfig := models.RuntimeScopeConfig{CacheDirectory: t.TempDir()}
	modelRoots, err := service.genericCacheRoots(scopeConfig, models.AssetArtifactKindModel)
	if err != nil || len(modelRoots) == 0 || modelRoots[len(modelRoots)-1] != scopeConfig.CacheDirectory {
		t.Fatalf("model genericCacheRoots = %#v, %v", modelRoots, err)
	}
	backendRoots, err := service.genericCacheRoots(scopeConfig, models.AssetArtifactKindBackend)
	if err != nil || len(backendRoots) != 1 || !strings.HasSuffix(filepath.ToSlash(backendRoots[0]), "/backend-artifacts") {
		t.Fatalf("backend genericCacheRoots = %#v, %v", backendRoots, err)
	}
}

func assertGenericSourceResolution(t *testing.T, service *service) {
	t.Helper()
	scopeConfig := models.RuntimeScopeConfig{CacheDirectory: t.TempDir()}
	service.resolveRevision = func(context.Context, string) (string, error) { return genericTestRevision, nil }
	resolved, err := service.resolveGenericSource(context.Background(), scopeConfig, "hf://owner/repo")
	if err != nil || resolved.revision != genericTestRevision || resolved.safe != "hf://owner/repo@"+genericTestRevision {
		t.Fatalf("resolved generic source = %#v, %v", resolved, err)
	}
	service.resolveRevision = func(context.Context, string) (string, error) { return "not-a-commit", nil }
	if _, err := service.resolveGenericSource(context.Background(), scopeConfig, "hf://owner/repo"); !errors.Is(err, models.ErrModelRevisionUnresolved) {
		t.Fatalf("unresolved revision error = %v, want ErrModelRevisionUnresolved", err)
	}
	if _, err := parseGenericReleaseSource("https://example.com/releases/download/v1/backend.tar"); !errors.Is(err, models.ErrModelReferenceInvalid) {
		t.Fatalf("invalid release source error = %v, want ErrModelReferenceInvalid", err)
	}
	release, err := parseGenericReleaseSource("https://github.com/owner/repo/releases/download/v1/backend.tar")
	if err != nil || release.kind != genericSourceRelease || release.artifactURL == "" {
		t.Fatalf("valid release source = %#v, %v", release, err)
	}
}

func assertGenericPreparationSelection(t *testing.T) {
	t.Helper()
	for _, request := range []models.PrepareModelAssetsRequest{
		{Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision}},
		{Offline: true},
		{Artifacts: []models.AssetRequirement{}},
		{Backend: "backend-v1"},
		{Name: "llm"},
		{Name: "./local/model"},
	} {
		if !shouldPrepareGenericAssets(request) {
			t.Fatalf("shouldPrepareGenericAssets(%#v) = false, want true", request)
		}
	}
	if shouldPrepareGenericAssets(models.PrepareModelAssetsRequest{Name: "unknown-symbol"}) {
		t.Fatal("unknown symbolic name should not select generic preparation")
	}
}
