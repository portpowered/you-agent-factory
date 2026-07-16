package functionalscenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	functionalBoundaryBaselinePath     = "contracts/functional-boundary-baseline.json"
	functionalBoundaryBaselineVersion  = "functional-boundary-baseline/v1"
	FunctionalBoundaryBaselineCategory = "invalid-boundary-baseline"
)

type functionalBoundaryBaseline struct {
	FormatVersion string                            `json:"formatVersion"`
	MigrationTask string                            `json:"migrationTask"`
	Files         []functionalBoundaryBaselineEntry `json:"files"`
}

type functionalBoundaryBaselineEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func loadFunctionalBoundaryBaseline(repositoryRoot string) (*functionalBoundaryBaseline, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(functionalBoundaryBaselinePath))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &functionalBoundaryBaseline{FormatVersion: functionalBoundaryBaselineVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read functional boundary baseline: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var baseline functionalBoundaryBaseline
	if err := decoder.Decode(&baseline); err != nil {
		return nil, functionalBoundaryBaselineError("decode %s: %v", functionalBoundaryBaselinePath, err)
	}
	if baseline.FormatVersion != functionalBoundaryBaselineVersion {
		return nil, functionalBoundaryBaselineError("%s has formatVersion %q; want %q", functionalBoundaryBaselinePath, baseline.FormatVersion, functionalBoundaryBaselineVersion)
	}
	if len(baseline.Files) > 0 && strings.TrimSpace(baseline.MigrationTask) == "" {
		return nil, functionalBoundaryBaselineError("%s must name migrationTask while legacy files remain", functionalBoundaryBaselinePath)
	}
	seen := make(map[string]bool, len(baseline.Files))
	for _, entry := range baseline.Files {
		if entry.Path != filepath.ToSlash(filepath.Clean(entry.Path)) || !strings.HasPrefix(entry.Path, "tests/functional/") {
			return nil, functionalBoundaryBaselineError("baseline path %q must be a normalized file under tests/functional", entry.Path)
		}
		if seen[entry.Path] {
			return nil, functionalBoundaryBaselineError("baseline path %q is duplicated", entry.Path)
		}
		seen[entry.Path] = true
		decodedHash, err := hex.DecodeString(entry.SHA256)
		if err != nil || len(decodedHash) != sha256.Size {
			return nil, functionalBoundaryBaselineError("baseline path %q has invalid SHA-256 %q", entry.Path, entry.SHA256)
		}
	}
	return &baseline, nil
}

func applyFunctionalBoundaryBaseline(repositoryRoot string, violations []FunctionalBoundaryViolation, baseline *functionalBoundaryBaseline) (int, []FunctionalBoundaryViolation, error) {
	violatingFiles := make(map[string]bool, len(violations))
	for _, violation := range violations {
		violatingFiles[violation.File] = true
	}
	approved := make(map[string]bool, len(baseline.Files))
	for _, entry := range baseline.Files {
		if !violatingFiles[entry.Path] {
			return 0, nil, functionalBoundaryBaselineError("baseline path %q has no direct product-boundary violation; remove its stale entry", entry.Path)
		}
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(entry.Path)))
		if err != nil {
			return 0, nil, functionalBoundaryBaselineError("read baseline path %q: %v", entry.Path, err)
		}
		actualHash := fmt.Sprintf("%x", sha256.Sum256(content))
		if actualHash != entry.SHA256 {
			return 0, nil, functionalBoundaryBaselineError("baseline path %q changed (SHA-256 %s); migrate its direct product-boundary use or update the reviewed baseline", entry.Path, actualHash)
		}
		approved[entry.Path] = true
	}
	remaining := slices.DeleteFunc(slices.Clone(violations), func(violation FunctionalBoundaryViolation) bool {
		return approved[violation.File]
	})
	return len(approved), remaining, nil
}

func functionalBoundaryBaselineError(format string, arguments ...any) error {
	return fmt.Errorf("functional test boundary [%s]: %s", FunctionalBoundaryBaselineCategory, fmt.Sprintf(format, arguments...))
}
