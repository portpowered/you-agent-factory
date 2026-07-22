package contractstaging

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"
)

func TestGenerateWithDependenciesWritesArtifactsInSortedOrder(t *testing.T) {
	root := t.TempDir()
	written := make([]string, 0)
	artifacts := map[string][]byte{
		"packages/api/generated/zz.json":    []byte("z"),
		"packages/api/generated/aa.json":    []byte("a"),
		manifestTarget:                      []byte("manifest"),
		"packages/api/generated/readme.txt": []byte("README"),
	}
	deps := GenerateArtifactsDependencies{
		BuildArtifacts: func(_ string) (map[string][]byte, error) {
			return artifacts, nil
		},
		WriteManifest: func(path string, _ []byte) error {
			written = append(written, filepath.ToSlash(path))
			return nil
		},
		WriteArtifact: func(path string, _ []byte) error {
			written = append(written, filepath.ToSlash(path))
			return nil
		},
	}

	if err := GenerateWithDependencies(root, deps); err != nil {
		t.Fatalf("GenerateWithDependencies() error = %v", err)
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, filepath.ToSlash(filepath.Join(root, filepath.FromSlash(path))))
	}
	sort.Strings(paths)
	if len(written) != len(paths) {
		t.Fatalf("writes = %#v, want %#v", written, paths)
	}
	for index, got := range written {
		if got != paths[index] {
			t.Fatalf("write order[%d] = %q, want %q", index, got, paths[index])
		}
	}
}

func TestGenerateWithDependenciesPropagatesWriterFailure(t *testing.T) {
	root := t.TempDir()
	err := GenerateWithDependencies(root, GenerateArtifactsDependencies{
		BuildArtifacts: func(_ string) (map[string][]byte, error) {
			return map[string][]byte{"packages/api/generated/a.json": []byte("a")}, nil
		},
		WriteArtifact: func(string, []byte) error { return errors.New("write failed") },
	})
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("expected write failure, got %v", err)
	}
}
