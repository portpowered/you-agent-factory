package prompting

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const factoryDocsTargetPrefix = "factory/docs/"

// NormalizeFactoryBundledDocTargetPaths returns sorted unique DOC target paths under factory/docs/**.
func NormalizeFactoryBundledDocTargetPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		if cleaned == "" || !strings.HasPrefix(cleaned, factoryDocsTargetPrefix) {
			continue
		}
		if len(cleaned) <= len(factoryDocsTargetPrefix) {
			continue
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	sort.Strings(normalized)
	return normalized
}

func bundledDocVariableReferences(docPaths []string) []PromptTemplateVariableReference {
	references := make([]PromptTemplateVariableReference, 0, len(docPaths))
	for _, targetPath := range docPaths {
		references = append(references, PromptTemplateVariableReference{
			Category:    PromptTemplateVariableCategoryDoc,
			Description: fmt.Sprintf("Bundled factory documentation at %s.", targetPath),
			Example:     fmt.Sprintf(`{{ index .Docs %q }}`, targetPath),
			Path:        fmt.Sprintf(`.Docs[%q]`, targetPath),
		})
	}

	return references
}

func bundledDocTargetPathSet(docPaths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(docPaths))
	for _, targetPath := range docPaths {
		set[targetPath] = struct{}{}
	}
	return set
}

func bundledDocPlaceholderContents(docPaths []string) map[string]string {
	if len(docPaths) == 0 {
		return map[string]string{}
	}

	contents := make(map[string]string, len(docPaths))
	for _, targetPath := range docPaths {
		contents[targetPath] = "bundled-doc-content"
	}
	return contents
}

func loadBundledDocContentsFromFactoryDir(factoryDir string) map[string]string {
	if strings.TrimSpace(factoryDir) == "" {
		return map[string]string{}
	}

	docsDir := filepath.Join(factoryDir, "docs")
	info, err := os.Stat(docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		return map[string]string{}
	}
	if !info.IsDir() {
		return map[string]string{}
	}

	contents := make(map[string]string)
	_ = filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(docsDir, path)
		if err != nil {
			return err
		}

		targetPath := factoryDocsTargetPrefix + filepath.ToSlash(relativePath)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents[targetPath] = string(data)
		return nil
	})

	if len(contents) == 0 {
		return map[string]string{}
	}
	return contents
}
