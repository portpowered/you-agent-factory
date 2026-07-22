package contractstaging_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

var (
	artifactCacheMu sync.Mutex
	artifactCache   = map[string]map[string][]byte{}
)

func testArtifactsForRepository(t *testing.T, repositoryRoot string) map[string][]byte {
	t.Helper()

	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	artifactCacheMu.Lock()
	if cached, ok := artifactCache[root]; ok {
		artifactCacheMu.Unlock()
		return cloneArtifactMap(cached)
	}
	artifactCacheMu.Unlock()

	artifacts, err := contractstaging.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	cloned := cloneArtifactMap(artifacts)

	artifactCacheMu.Lock()
	artifactCache[root] = cloneArtifactMap(cloned)
	artifactCacheMu.Unlock()

	return cloned
}

func cloneArtifactMap(artifacts map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(artifacts))
	for path, payload := range artifacts {
		out[path] = append([]byte(nil), payload...)
	}
	return out
}
