package main

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func coverageCanonicalFilePath(filePath string, repoRoot string) (string, error) {
	normalizedPath := filepath.ToSlash(strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/"))
	if normalizedPath == "" {
		return "", errors.New("empty file path")
	}
	if normalizedPath == modulePath {
		return "", fmt.Errorf("profile path %q does not include a package directory", filePath)
	}

	switch {
	case strings.HasPrefix(normalizedPath, modulePath+"/"):
		return normalizedPath, nil
	case strings.HasPrefix(normalizedPath, "./"):
		normalizedPath = strings.TrimPrefix(normalizedPath, "./")
	case filepath.IsAbs(filePath):
		relativePath, err := filepath.Rel(repoRoot, filePath)
		if err != nil {
			return "", fmt.Errorf("resolve profile path relative to repository root: %w", err)
		}
		normalizedPath = filepath.ToSlash(relativePath)
	}

	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	if strings.HasPrefix(normalizedPath, "../") || normalizedPath == ".." {
		return "", fmt.Errorf("profile path %q escapes repository root", filePath)
	}

	importDir := path.Dir(normalizedPath)
	if importDir == "." || importDir == "" {
		return "", fmt.Errorf("profile path %q does not include a package directory", filePath)
	}
	return modulePath + "/" + normalizedPath, nil
}

func coverageImportPath(filePath string, repoRoot string) (string, error) {
	canonicalFilePath, err := coverageCanonicalFilePath(filePath, repoRoot)
	if err != nil {
		return "", err
	}
	return path.Dir(canonicalFilePath), nil
}
