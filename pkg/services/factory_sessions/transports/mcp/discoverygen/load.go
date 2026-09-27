package discoverygen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const (
	// AuthoredCatalogPath is the reviewed MCP tool catalog input.
	AuthoredCatalogPath  = "contracts/mcp/tools.json"
	AuthoredManifestPath = "contracts/mcp/manifest.json"
)

// LoadAuthoredManifest reads the single source used to publish and register MCP surfaces.
func LoadAuthoredManifest(repositoryRoot string) (MCPManifest, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(AuthoredManifestPath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return MCPManifest{}, fmt.Errorf("read %s: %w", AuthoredManifestPath, err)
	}
	var manifest MCPManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return MCPManifest{}, fmt.Errorf("decode %s: %w", AuthoredManifestPath, err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return MCPManifest{}, err
	}
	return manifest, nil
}

// LoadResolvedCatalog loads and resolves the authored MCP tool catalog.
func LoadResolvedCatalog(repositoryRoot string) (any, error) {
	resolved, diagnostics := contractvalidator.LoadAndResolve(
		repositoryRoot,
		AuthoredCatalogPath,
		[]string{AuthoredCatalogPath},
	)
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("resolve %s: %s", AuthoredCatalogPath, diagnostics[0].Message)
	}
	return resolved, nil
}

func catalogToolDescription(record map[string]any) (string, error) {
	documentation, ok := record["documentation"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("missing documentation")
	}
	inner, ok := documentation["documentation"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("missing documentation.documentation")
	}
	description, ok := inner["description"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("missing documentation.documentation.description")
	}
	canonical, _ := description["canonicalEnglish"].(string)
	if strings.TrimSpace(canonical) == "" {
		return "", fmt.Errorf("empty canonicalEnglish description")
	}
	return canonical, nil
}
