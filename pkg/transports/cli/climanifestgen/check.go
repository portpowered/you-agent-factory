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
}

// Empty reports whether generated artifacts match the generator output.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 && len(drift.Missing) == 0 && len(drift.Unexpected) == 0
}

// Check compares committed CLI family artifacts with freshly generated output.
func Check(repositoryRoot string) (Drift, error) {
	expected := map[string][]byte{
		RepresentativeFamilyJSONPath:              nil,
		RepresentativeFamilyCommandIDsPath:        representativeCommandIDsSource(),
		FactoryConfigInitFamilyJSONPath:           nil,
		FactoryConfigInitFamilyCommandIDsPath:     factoryConfigInitCommandIDsSource(),
		ModelsDocsFamilyJSONPath:                  nil,
		ModelsDocsFamilyCommandIDsPath:            modelsDocsCommandIDsSource(),
	}

	representativePayload, err := RepresentativeFamilyArtifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[RepresentativeFamilyJSONPath] = representativePayload

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

	drift := Drift{}
	for path, want := range expected {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
		got, err := os.ReadFile(target)
		if err != nil {
			if os.IsNotExist(err) {
				drift.Missing = append(drift.Missing, path)
				continue
			}
			return Drift{}, fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(normalizeGeneratedArtifactBytes(got), normalizeGeneratedArtifactBytes(want)) {
			drift.Stale = append(drift.Stale, path)
		}
	}
	return drift, nil
}

func normalizeGeneratedArtifactBytes(payload []byte) []byte {
	return bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
}
