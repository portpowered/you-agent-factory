package contractstaging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// GenerateArtifactsDependencies lets tests control artifact generation side effects.
type GenerateArtifactsDependencies struct {
	BuildArtifacts func(repositoryRoot string) (map[string][]byte, error)
	WriteManifest  func(path string, payload []byte) error
	WriteArtifact  func(path string, payload []byte) error
}

// Generate writes the complete reviewed staging projection and no other files.
func Generate(repositoryRoot string) error {
	return GenerateWithDependencies(repositoryRoot, GenerateArtifactsDependencies{})
}

// GenerateWithDependencies writes the complete reviewed staging projection with
// caller-supplied artifact builders/writers.
func GenerateWithDependencies(repositoryRoot string, dependencies GenerateArtifactsDependencies) error {
	normalizedRoot, deps, err := normalizeGenerateDependencies(repositoryRoot, dependencies)
	if err != nil {
		return err
	}

	artifacts, err := deps.BuildArtifacts(normalizedRoot)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		target := filepath.Join(normalizedRoot, filepath.FromSlash(path))
		writer := deps.WriteArtifact
		if path == manifestTarget {
			writer = deps.WriteManifest
		}
		if err := writer(target, artifacts[path]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeGenerateDependencies(repositoryRoot string, requested GenerateArtifactsDependencies) (string, GenerateArtifactsDependencies, error) {
	normalizedRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", GenerateArtifactsDependencies{}, fmt.Errorf("resolve repository root: %w", err)
	}
	deps := GenerateArtifactsDependencies{
		BuildArtifacts: func(path string) (map[string][]byte, error) {
			return ArtifactsWithDependencies(path, ArtifactsDependencies{})
		},
		WriteManifest: defaultWriteFile,
		WriteArtifact: defaultWriteFile,
	}
	if requested.BuildArtifacts != nil {
		deps.BuildArtifacts = requested.BuildArtifacts
	}
	if requested.WriteManifest != nil {
		deps.WriteManifest = requested.WriteManifest
	}
	if requested.WriteArtifact != nil {
		deps.WriteArtifact = requested.WriteArtifact
	}
	return normalizedRoot, deps, nil
}

func defaultWriteFile(path string, payload []byte) error {
	targetDir := filepath.Dir(path)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
