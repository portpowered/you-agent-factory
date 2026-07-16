package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
)

func TestReportReturnsNonZeroForStructuralDiagnostics(t *testing.T) {
	t.Parallel()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	diagnostic := mcpcontractcheck.Diagnostic{
		Code: "mcp.discovery.missing", ToolID: "mcp.tool.you.factory_session.list", Surface: "discovery",
		Message: "regenerate discovery from the authored catalog",
	}
	if status := report([]mcpcontractcheck.Diagnostic{diagnostic}, nil, stdout, stderr); status != 1 {
		t.Fatalf("report() status = %d, want 1", status)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), diagnostic.ToolID) || !strings.Contains(stderr.String(), diagnostic.Message) {
		t.Fatalf("report() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCleanRepositoryIsDeterministic(t *testing.T) {
	root := testutil.MustRepoRoot(t)
	paths := []string{
		discoverygen.AuthoredCatalogPath,
		discoverygen.DiscoveryJSONPath,
		discoverygen.DiscoveryGoPath,
		"contracts/mcp/deprecated.json",
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
