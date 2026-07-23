package packagedfactorycatalog

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Drift describes byte-level differences between the canonical catalog and
// the checked-in generated directory. Paths are package-relative and sorted.
type Drift struct {
	Stale      []string
	Missing    []string
	Unexpected []string
}

type checkedFile struct {
	payload []byte
	regular bool
}

// Empty reports whether the checked-in catalog exactly matches its canonical
// projection.
func (drift Drift) Empty() bool {
	return len(drift.Stale) == 0 &&
		len(drift.Missing) == 0 &&
		len(drift.Unexpected) == 0
}

// Check recomputes the complete catalog and compares its exact path set and
// bytes with the checked-in generated directory without changing either.
func Check(repositoryRoot string) (Drift, error) {
	absoluteRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return Drift{}, fmt.Errorf("catalog drift check: resolve repository root: %w", err)
	}
	packageRoot := filepath.Join(absoluteRoot, "packages", "packaged-factories")
	expected, err := BuildCatalog(
		context.Background(),
		os.DirFS(packageRoot),
		"factories",
		"schemas/factory.schema.json",
	)
	if err != nil {
		return Drift{}, err
	}
	actual, err := readGeneratedFiles(packageRoot)
	if err != nil {
		return Drift{}, err
	}
	return compareGeneratedFiles(expected.Files, actual), nil
}

func readGeneratedFiles(packageRoot string) (map[string]checkedFile, error) {
	generatedRoot := filepath.Join(packageRoot, "generated")
	files := make(map[string]checkedFile)
	err := filepath.WalkDir(generatedRoot, func(target string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if target == generatedRoot && os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(packageRoot, target)
		if err != nil {
			return err
		}
		packagePath := filepath.ToSlash(relative)
		if !entry.Type().IsRegular() {
			files[packagePath] = checkedFile{}
			return nil
		}
		payload, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read %s: %w", packagePath, err)
		}
		files[packagePath] = checkedFile{payload: payload, regular: true}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("catalog drift check: inspect generated outputs: %w", err)
	}
	return files, nil
}

func compareGeneratedFiles(expected map[string][]byte, actual map[string]checkedFile) Drift {
	drift := Drift{}
	for target, expectedPayload := range expected {
		actualFile, exists := actual[target]
		if !exists {
			drift.Missing = append(drift.Missing, target)
			continue
		}
		if !actualFile.regular || !bytes.Equal(actualFile.payload, expectedPayload) {
			drift.Stale = append(drift.Stale, target)
		}
		delete(actual, target)
	}
	for target := range actual {
		drift.Unexpected = append(drift.Unexpected, target)
	}
	sort.Strings(drift.Stale)
	sort.Strings(drift.Missing)
	sort.Strings(drift.Unexpected)
	return drift
}
