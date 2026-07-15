package discoverygen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const (
	// DiscoveryJSONPath is the generated MCP discovery metadata artifact.
	DiscoveryJSONPath = "pkg/transports/mcp/generated/discovery.json"
)

// DiscoveryArtifact returns deterministic generated MCP discovery metadata bytes.
func DiscoveryArtifact(repositoryRoot string) ([]byte, error) {
	resolved, err := LoadResolvedCatalog(repositoryRoot)
	if err != nil {
		return nil, err
	}
	metadata, err := ProjectDiscoveryFromCatalogDocument(resolved)
	if err != nil {
		return nil, err
	}
	if err := VerifyDiscoveryByteStability(metadata); err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(metadata)
}

// Generate writes MCP discovery metadata artifacts for review and drift checks.
func Generate(repositoryRoot string) error {
	return writeArtifact(repositoryRoot, DiscoveryJSONPath, DiscoveryArtifact)
}

// Drift describes byte-level differences between generated artifacts and the
// current generator output.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
}

// Empty reports whether generated artifacts match the generator output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// Check compares committed MCP discovery artifacts with freshly generated output.
func Check(repositoryRoot string) (Drift, error) {
	payload, err := DiscoveryArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}

	target := filepath.Join(repositoryRoot, filepath.FromSlash(DiscoveryJSONPath))
	got, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Drift{Missing: []string{DiscoveryJSONPath}}, nil
		}
		return Drift{}, fmt.Errorf("read %s: %w", DiscoveryJSONPath, err)
	}
	if !bytes.Equal(normalizeGeneratedArtifactBytes(got), normalizeGeneratedArtifactBytes(payload)) {
		return Drift{Stale: []string{DiscoveryJSONPath}}, nil
	}
	return Drift{}, nil
}

func writeArtifact(repositoryRoot, path string, producer func(string) ([]byte, error)) error {
	payload, err := producer(repositoryRoot)
	if err != nil {
		return err
	}
	target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func normalizeGeneratedArtifactBytes(payload []byte) []byte {
	return bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
}
