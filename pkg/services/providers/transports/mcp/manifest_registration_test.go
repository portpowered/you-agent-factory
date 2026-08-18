package providersmcp_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	mcpAdapterImportPath            = "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
	mcpAdapterServiceRootImportPath = "github.com/portpowered/infinite-you/pkg/services/providers"
)

type coverageMinimumManifest struct {
	Lane     string `json:"lane"`
	Packages []struct {
		Package   string  `json:"package"`
		Minimum   float64 `json:"minimum"`
		Exception *struct {
			Kind string `json:"kind"`
		} `json:"exception"`
	} `json:"packages"`
}

func TestManifestRegistration_MCPAdapterPackageIsRegistered(t *testing.T) {
	t.Helper()

	assertCoverageMinimumRegistration(t, "unit", "docs/internal/baselines/go-unit-coverage-package-minimums.json")
	assertCoverageMinimumRegistration(t, "functional", "docs/internal/baselines/go-functional-coverage-package-minimums.json")
}

func assertCoverageMinimumRegistration(t *testing.T, lane string, relativePath string) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, relativePath))
	if err != nil {
		t.Fatalf("read %s coverage manifest: %v", lane, err)
	}

	var manifest coverageMinimumManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s coverage manifest: %v", lane, err)
	}
	if manifest.Lane != lane {
		t.Fatalf("%s coverage manifest lane = %q, want %q", relativePath, manifest.Lane, lane)
	}

	for _, entry := range manifest.Packages {
		if entry.Package != mcpAdapterServiceRootImportPath {
			continue
		}
		if entry.Exception != nil {
			if entry.Exception.Kind != "measurement" {
				t.Fatalf("%s coverage exception kind for %q = %q, want measurement", lane, mcpAdapterServiceRootImportPath, entry.Exception.Kind)
			}
			return
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, mcpAdapterServiceRootImportPath)
		}
		return
	}
	t.Fatalf("%s coverage manifest missing service root %q declaring %q", lane, mcpAdapterServiceRootImportPath, mcpAdapterImportPath)
}
