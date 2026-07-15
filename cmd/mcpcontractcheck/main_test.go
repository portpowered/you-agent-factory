package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
)

func TestRunCleanRepositoryIsDeterministic(t *testing.T) {
	root := testutil.MustRepoRoot(t)
	paths := []string{
		discoverygen.AuthoredCatalogPath,
		discoverygen.DiscoveryJSONPath,
		discoverygen.DiscoveryGoPath,
	}
	before := snapshotFiles(t, root, paths)

	for iteration := 0; iteration < 2; iteration++ {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if status := run(root, stdout, stderr); status != 0 {
			t.Fatalf("run %d status = %d, stderr = %q", iteration, status, stderr.String())
		}
		if stdout.String() != successMessage+"\n" || stderr.Len() != 0 {
			t.Fatalf("run %d stdout = %q, stderr = %q", iteration, stdout.String(), stderr.String())
		}
	}
	if after := snapshotFiles(t, root, paths); !reflectSnapshotsEqual(before, after) {
		t.Fatal("repeated clean checks changed authored or generated MCP artifacts")
	}
}

func snapshotFiles(t *testing.T, root string, paths []string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		snapshot[path] = data
	}
	return snapshot
}

func reflectSnapshotsEqual(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for path, expected := range left {
		if !bytes.Equal(expected, right[path]) {
			return false
		}
	}
	return true
}
