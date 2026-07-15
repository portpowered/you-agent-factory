package climanifestgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Drift describes byte-level differences between generated artifacts and the
// current generator output.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
	// CommandIDs maps a generated artifact path to the stable IDs affected by drift.
	CommandIDs map[string][]string
}

// Empty reports whether generated artifacts match the generator output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// Check compares committed CLI family artifacts with freshly generated output.
func Check(repositoryRoot string) (Drift, error) {
	expected := map[string][]byte{
		RepresentativeFamilyJSONPath:          nil,
		SessionFamilyJSONPath:                 nil,
		WorkFamilyJSONPath:                    nil,
		FactoryConfigInitFamilyJSONPath:       nil,
		ModelsDocsFamilyJSONPath:              nil,
		RepresentativeFamilyCommandIDsPath:    representativeAndWorkCommandIDsSource(),
		SessionFamilyCommandIDsPath:           sessionCommandIDsSource(),
		FactoryConfigInitFamilyCommandIDsPath: factoryConfigInitCommandIDsSource(),
		ModelsDocsFamilyCommandIDsPath:        modelsDocsCommandIDsSource(),
		MCPFamilyJSONPath:                     nil,
		WorkflowCompatibilityFamilyJSONPath:   nil,
		WorkflowMCPFamilyCommandIDsPath:       workflowMCPCommandIDsSource(),
	}

	representativePayload, err := RepresentativeFamilyArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[RepresentativeFamilyJSONPath] = representativePayload

	sessionPayload, err := SessionFamilyArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[SessionFamilyJSONPath] = sessionPayload

	workPayload, err := WorkArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[WorkFamilyJSONPath] = workPayload

	factoryConfigInitPayload, err := FactoryConfigInitFamilyArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[FactoryConfigInitFamilyJSONPath] = factoryConfigInitPayload

	modelsDocsPayload, err := ModelsDocsArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[ModelsDocsFamilyJSONPath] = modelsDocsPayload

	mcpPayload, err := MCPArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[MCPFamilyJSONPath] = mcpPayload
	workflowPayload, err := WorkflowCompatibilityArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[WorkflowCompatibilityFamilyJSONPath] = workflowPayload

	drift := Drift{CommandIDs: map[string][]string{}}
	artifactIDs := map[string][]string{
		MCPFamilyJSONPath:                   MCPFamilyCommandIDs,
		WorkflowCompatibilityFamilyJSONPath: WorkflowCompatibilityFamilyCommandIDs,
		WorkflowMCPFamilyCommandIDsPath:     append(append([]string{}, MCPFamilyCommandIDs...), WorkflowCompatibilityFamilyCommandIDs...),
	}
	for path, want := range expected {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
		got, err := os.ReadFile(target)
		if err != nil {
			if os.IsNotExist(err) {
				drift.Missing = append(drift.Missing, path)
				drift.CommandIDs[path] = artifactIDs[path]
				continue
			}
			return Drift{}, fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(normalizeGeneratedArtifactBytes(got), normalizeGeneratedArtifactBytes(want)) {
			drift.Stale = append(drift.Stale, path)
			drift.CommandIDs[path] = artifactIDs[path]
		}
	}
	return drift, nil
}

func normalizeGeneratedArtifactBytes(payload []byte) []byte {
	return bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
}
