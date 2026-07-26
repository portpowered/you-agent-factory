package contractstaging

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func generateManifest(_ string, artifacts map[string][]byte) ([]byte, error) {
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	exports := make(map[string]any, len(paths))
	for _, repositoryPath := range paths {
		if !strings.HasPrefix(repositoryPath, "packages/api/") {
			continue
		}
		packagePath := strings.TrimPrefix(repositoryPath, "packages/api/")
		id := artifactID(packagePath)
		digest := fmt.Sprintf("%x", sha256.Sum256(artifacts[repositoryPath]))
		exports[id] = manifestExport(id, packagePath, artifactFamily(packagePath), digest)
	}
	manifest := map[string]any{
		"formatVersion":  "1.0.0",
		"packageId":      "you-agent-factory.api",
		"packageVersion": "0.0.0",
		"familyFormatVersions": map[string]any{
			"cli": "1.0.0", "config": "1.0.0", "javascript": "1.0.0",
			"mcp": "1.0.0", "openapi": "1.0.0", "shared": "1.0.0",
		},
		"exports": exports,
	}
	return marshalDocument(manifest)
}

func artifactID(path string) string {
	withoutExtension := strings.TrimSuffix(strings.TrimSuffix(path, filepath.Ext(path)), ".schema")
	replacer := strings.NewReplacer("/", ".", "_", "-", "@", "")
	return strings.Trim(replacer.Replace(strings.ToLower(withoutExtension)), ".")
}

func artifactFamily(path string) string {
	switch {
	case strings.Contains(path, "/schemas/"):
		return "config"
	case strings.Contains(path, "/cli/"):
		return "cli"
	case strings.Contains(path, "/mcp/"):
		return "mcp"
	case strings.Contains(path, "/javascript/"):
		return "javascript"
	case strings.Contains(path, "/openapi/"):
		return "openapi"
	default:
		return "shared"
	}
}

func manifestExport(id, path, family, digest string) map[string]any {
	title := "Published " + id + " contract"
	return map[string]any{
		"path":         path,
		"family":       family,
		"artifactHash": digest,
		"documentation": map[string]any{
			"formatVersion": "1.0.0",
			"itemId":        id,
			"documentation": map[string]any{
				"title": map[string]any{"id": id + ".title", "canonicalEnglish": title},
				"description": map[string]any{
					"id": id + ".description", "canonicalEnglish": title + " as raw JSON or YAML data.",
				},
			},
			"examples":   []any{path},
			"visibility": "public",
			"sourceHash": digest,
		},
		"lifecycle": map[string]any{
			"formatVersion": "1.0.0", "itemId": id, "state": "active", "since": "0.0.0",
		},
	}
}
