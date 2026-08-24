package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

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
