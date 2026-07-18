package contractstaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const stagingDirectory = "packages/api/generated"

// Drift describes every difference between canonical joined output and package
// staging. Paths are repository-relative and sorted within each category.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
}

type stagedFile struct {
	payload []byte
	regular bool
}

// Empty reports whether package staging exactly matches canonical joined output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// Check computes expected artifacts in memory, then compares their path set and
// bytes with package staging without changing either authored or staged files.
func Check(repositoryRoot string) (Drift, error) {
	expected, err := Artifacts(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	actual, err := stagedArtifacts(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}

	drift := Drift{}
	for path, expectedPayload := range expected {
		if path == FactorySchemaAuthoredPath {
			continue
		}
		actualFile, exists := actual[path]
		if !exists {
			drift.Missing = append(drift.Missing, path)
			continue
		}
		if !actualFile.regular || !bytes.Equal(actualFile.payload, expectedPayload) {
			drift.Stale = append(drift.Stale, path)
		}
		delete(actual, path)
	}
	for path := range actual {
		drift.Unexpected = append(drift.Unexpected, path)
	}
	if category, path := authoredArtifactDrift(repositoryRoot, FactorySchemaAuthoredPath, expected[FactorySchemaAuthoredPath]); category != "" {
		switch category {
		case "stale":
			drift.Stale = append(drift.Stale, path)
		case "missing":
			drift.Missing = append(drift.Missing, path)
		}
	}
	sort.Strings(drift.Stale)
	sort.Strings(drift.Missing)
	sort.Strings(drift.Unexpected)
	return drift, nil
}

func authoredArtifactDrift(repositoryRoot, path string, expected []byte) (category, repositoryPath string) {
	target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
	actual, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", path
		}
		return "missing", path
	}
	if !bytes.Equal(actual, expected) {
		return "stale", path
	}
	return "", path
}

// Artifacts returns the complete deterministic package staging projection.
func Artifacts(repositoryRoot string) (map[string][]byte, error) {
	documents, diagnostics := contractjoiner.Join(JoinInput(repositoryRoot))
	if len(diagnostics) != 0 {
		payload, err := json.Marshal(diagnostics)
		if err != nil {
			return nil, fmt.Errorf("encode join diagnostics: %w", err)
		}
		return nil, fmt.Errorf("join canonical contracts: %s", payload)
	}

	expected := make(map[string][]byte, len(documents)+len(rawArtifacts)+4)
	for _, document := range documents {
		payload, err := contractjoiner.MarshalCanonicalJSON(document.Value)
		if err != nil {
			return nil, fmt.Errorf("serialize joined contract %s: %w", document.Path, err)
		}
		path := filepath.ToSlash(filepath.Join(joinedOutputDirectory, document.Path))
		expected[path] = payload
	}
	for _, artifact := range rawArtifacts {
		sourcePath := filepath.Join(repositoryRoot, filepath.FromSlash(artifact.Source))
		payload, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read canonical raw artifact %s: %w", artifact.Source, err)
		}
		if artifact.Source == CanonicalOpenAPIPath {
			payload, err = ProjectStagedOpenAPI(payload, ReviewedOpenAPIBytePolicy)
			if err != nil {
				return nil, fmt.Errorf("project staged OpenAPI: %w", err)
			}
		}
		expected[artifact.Target] = payload
	}
	factorySchema, err := generateFactorySchema(repositoryRoot)
	if err != nil {
		return nil, err
	}
	expected[FactorySchemaAuthoredPath] = factorySchema
	expected[factorySchemaTarget] = factorySchema
	standaloneSchemas, err := generateStandaloneFactorySchemas(repositoryRoot)
	if err != nil {
		return nil, err
	}
	for path, payload := range standaloneSchemas {
		expected[path] = payload
	}
	manifest, err := generateManifest(repositoryRoot, expected)
	if err != nil {
		return nil, err
	}
	expected[manifestTarget] = manifest
	return expected, nil
}

func stagedArtifacts(repositoryRoot string) (map[string]stagedFile, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	base := filepath.Join(root, filepath.FromSlash(stagingDirectory))
	actual := make(map[string]stagedFile)
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == base && os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			actual[filepath.ToSlash(relative)] = stagedFile{}
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		actual[filepath.ToSlash(relative)] = stagedFile{payload: payload, regular: true}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read package staging: %w", err)
	}
	return actual, nil
}
