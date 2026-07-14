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

// Check compares committed representative-family artifacts with freshly generated output.
func Check(repositoryRoot string) (Drift, error) {
	expected := map[string][]byte{
		RepresentativeFamilyJSONPath:       nil,
		RepresentativeFamilyCommandIDsPath: commandIDsSource(),
	}

	jsonPayload, err := Artifact(repositoryRoot)
	if err != nil {
		return Drift{}, err
	}
	expected[RepresentativeFamilyJSONPath] = jsonPayload

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
		if !bytes.Equal(got, want) {
			drift.Stale = append(drift.Stale, path)
		}
	}
	return drift, nil
}
