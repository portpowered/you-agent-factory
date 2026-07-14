package contractstaging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Generate writes the complete reviewed staging projection and no other files.
func Generate(repositoryRoot string) error {
	artifacts, err := Artifacts(repositoryRoot)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if err := os.WriteFile(target, artifacts[path], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
