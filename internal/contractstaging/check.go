package contractstaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const (
	apiStagingDirectory             = "packages/api/generated"
	packagedFactorySchemasDirectory = "packages/packaged-factories/schemas"
)

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

// ArtifactsDependencies lets unit tests replace expensive operations in
// artifact orchestration without changing production behavior.
type ArtifactsDependencies struct {
	Join                      func(contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic)
	ReadRawArtifact           func(path string) ([]byte, error)
	ProjectOpenAPI            func(canonical []byte, policy OpenAPIBytePolicy) ([]byte, error)
	GenerateSchema            func(repositoryRoot string) ([]byte, error)
	GenerateStandaloneSchemas func(repositoryRoot string) (map[string][]byte, error)
	GenerateManifest          func(repositoryRoot string, artifacts map[string][]byte) ([]byte, error)
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
	if err := verifyManifestMetadata(expected); err != nil {
		return Drift{}, err
	}
	actual, err := stagedArtifacts(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	relevantActual := filterArtifactsByStagingDirectory(actual)
	if !shouldRunArtifactCheck(expected, relevantActual) {
		return Drift{}, nil
	}
	drift := compareArtifacts(repositoryRoot, expected, relevantActual)
	sort.Strings(drift.Stale)
	sort.Strings(drift.Missing)
	sort.Strings(drift.Unexpected)
	return drift, nil
}

func shouldRunArtifactCheck(expected map[string][]byte, staged map[string]stagedFile) bool {
	return len(expected) > 0 || len(staged) > 0
}

func filterArtifactsByStagingDirectory(actual map[string]stagedFile) map[string]stagedFile {
	filtered := make(map[string]stagedFile, len(actual))
	for path, item := range actual {
		for _, directory := range artifactDirectories() {
			if strings.HasPrefix(path, directory+"/") {
				filtered[path] = item
				break
			}
		}
	}
	return filtered
}

func compareArtifacts(repositoryRoot string, expected map[string][]byte, actual map[string]stagedFile) Drift {
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
	return drift
}

func verifyManifestMetadata(artifacts map[string][]byte) error {
	payload, ok := artifacts[manifestTarget]
	if !ok {
		return fmt.Errorf("missing generated manifest at %s", manifestTarget)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("decode manifest artifact: %w", err)
	}
	for _, key := range []string{"formatVersion", "packageId", "packageVersion", "sourceCommit", "familyFormatVersions", "exports"} {
		if _, ok := manifest[key]; !ok {
			return fmt.Errorf("manifest metadata missing %s", key)
		}
	}
	if sourceCommit, ok := manifest["sourceCommit"].(string); !ok || sourceCommit == "" {
		return fmt.Errorf("manifest metadata missing sourceCommit")
	}
	return nil
}

// ArtifactsWithDependencies runs the same artifact pipeline as Artifacts but allows
// callers to replace expensive collaborators for deterministic unit tests.
func ArtifactsWithDependencies(repositoryRoot string, dependencies ArtifactsDependencies) (map[string][]byte, error) {
	normalizedRoot, dependencies, err := normalizeArtifactsDependencies(repositoryRoot, dependencies)
	if err != nil {
		return nil, err
	}
	return artifactsWithDependencies(normalizedRoot, dependencies)
}

// Artifacts returns the complete deterministic package staging projection.
func Artifacts(repositoryRoot string) (map[string][]byte, error) {
	return ArtifactsWithDependencies(repositoryRoot, ArtifactsDependencies{})
}

func artifactsWithDependencies(repositoryRoot string, dependencies ArtifactsDependencies) (map[string][]byte, error) {
	documents, diagnostics := dependencies.Join(JoinInput(repositoryRoot))
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
		payload, err := dependencies.ReadRawArtifact(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read canonical raw artifact %s: %w", artifact.Source, err)
		}
		if artifact.Source == CanonicalOpenAPIPath {
			payload, err = dependencies.ProjectOpenAPI(payload, ReviewedOpenAPIBytePolicy)
			if err != nil {
				return nil, fmt.Errorf("project staged OpenAPI: %w", err)
			}
		}
		expected[artifact.Target] = payload
	}
	factorySchema, err := dependencies.GenerateSchema(repositoryRoot)
	if err != nil {
		return nil, err
	}
	expected[FactorySchemaAuthoredPath] = factorySchema
	expected[factorySchemaTarget] = factorySchema
	expected[packagedFactorySchemaJSON] = bytes.Clone(factorySchema)
	factorySchemaYAML, err := marshalFactorySchemaYAML(factorySchema)
	if err != nil {
		return nil, err
	}
	expected[packagedFactorySchemaYAML] = factorySchemaYAML
	standaloneSchemas, err := dependencies.GenerateStandaloneSchemas(repositoryRoot)
	if err != nil {
		return nil, err
	}
	for path, payload := range standaloneSchemas {
		expected[path] = payload
	}
	manifest, err := dependencies.GenerateManifest(repositoryRoot, expected)
	if err != nil {
		return nil, err
	}
	expected[manifestTarget] = manifest
	return expected, nil
}

func normalizeArtifactsDependencies(repositoryRoot string, requested ArtifactsDependencies) (string, ArtifactsDependencies, error) {
	normalizedRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", ArtifactsDependencies{}, fmt.Errorf("resolve repository root: %w", err)
	}
	resolved := ArtifactsDependencies{
		Join:                      contractjoiner.Join,
		ReadRawArtifact:           os.ReadFile,
		ProjectOpenAPI:            ProjectStagedOpenAPI,
		GenerateSchema:            generateFactorySchema,
		GenerateStandaloneSchemas: generateStandaloneFactorySchemas,
		GenerateManifest:          generateManifest,
	}
	if requested.Join != nil {
		resolved.Join = requested.Join
	}
	if requested.ReadRawArtifact != nil {
		resolved.ReadRawArtifact = requested.ReadRawArtifact
	}
	if requested.ProjectOpenAPI != nil {
		resolved.ProjectOpenAPI = requested.ProjectOpenAPI
	}
	if requested.GenerateSchema != nil {
		resolved.GenerateSchema = requested.GenerateSchema
	}
	if requested.GenerateStandaloneSchemas != nil {
		resolved.GenerateStandaloneSchemas = requested.GenerateStandaloneSchemas
	}
	if requested.GenerateManifest != nil {
		resolved.GenerateManifest = requested.GenerateManifest
	}
	return normalizedRoot, resolved, nil
}

func stagedArtifacts(repositoryRoot string) (map[string]stagedFile, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	actual := make(map[string]stagedFile)
	for _, directory := range artifactDirectories() {
		base := filepath.Join(root, filepath.FromSlash(directory))
		if err := walkStagedArtifacts(root, base, actual); err != nil {
			return nil, err
		}
	}
	return actual, nil
}

func artifactDirectories() [2]string {
	return [2]string{apiStagingDirectory, packagedFactorySchemasDirectory}
}

func walkStagedArtifacts(root, base string, actual map[string]stagedFile) error {
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
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
		return fmt.Errorf("read package staging: %w", err)
	}
	return nil
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
