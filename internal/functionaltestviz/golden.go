package functionaltestviz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoldenProvenance is fixture provenance loaded from a golden manifest.json.
type GoldenProvenance struct {
	Provider      string
	Case          string
	FidelityClass string
	ID            string
	ManifestPath  string
}

// Present reports whether provenance was attached from a manifest.
func (p GoldenProvenance) Present() bool {
	return strings.TrimSpace(p.ManifestPath) != ""
}

// goldenManifestFile mirrors the provider-session golden manifest contract
// fields needed for catalog provenance. The generator does not load the full
// provider golden harness.
type goldenManifestFile struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Case          string `json:"case"`
	FidelityClass string `json:"fidelityClass"`
}

// AttachGoldenProvenance loads manifest provenance for every golden-backed
// record. Missing or malformed manifests fail closed with an actionable error;
// provenance is never silently omitted.
//
// Paths in Record.Golden are resolved relative to baseDir when baseDir is
// non-empty; otherwise they are used as given (absolute or process-relative).
func AttachGoldenProvenance(records []ClassifiedRecord, baseDir string) ([]ClassifiedRecord, error) {
	out := make([]ClassifiedRecord, len(records))
	copy(out, records)
	for i := range out {
		if !out[i].GoldenBacked {
			continue
		}
		manifestPath := strings.TrimSpace(out[i].Record.Golden)
		if manifestPath == "" {
			return nil, fmt.Errorf(
				"golden-backed test %s missing golden manifest path",
				out[i].Record.Identity(),
			)
		}
		provenance, err := LoadGoldenProvenance(resolveManifestPath(baseDir, manifestPath), manifestPath)
		if err != nil {
			return nil, fmt.Errorf(
				"golden provenance for %s (%s): %w",
				out[i].Record.Identity(),
				manifestPath,
				err,
			)
		}
		out[i].Provenance = provenance
	}
	return out, nil
}

// LoadGoldenProvenance reads and validates a golden manifest at resolvedPath.
// catalogPath is the path recorded in Provenance.ManifestPath (usually the
// inventory golden reference).
func LoadGoldenProvenance(resolvedPath, catalogPath string) (GoldenProvenance, error) {
	resolvedPath = strings.TrimSpace(resolvedPath)
	catalogPath = strings.TrimSpace(catalogPath)
	if catalogPath == "" {
		catalogPath = resolvedPath
	}
	if resolvedPath == "" {
		return GoldenProvenance{}, fmt.Errorf("golden manifest path is required")
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return GoldenProvenance{}, fmt.Errorf("golden manifest not found: %s", catalogPath)
		}
		return GoldenProvenance{}, fmt.Errorf("read golden manifest %s: %w", catalogPath, err)
	}
	return DecodeGoldenProvenance(data, catalogPath)
}

// DecodeGoldenProvenance decodes and validates golden manifest JSON bytes.
func DecodeGoldenProvenance(data []byte, catalogPath string) (GoldenProvenance, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return GoldenProvenance{}, fmt.Errorf("golden manifest JSON is empty: %s", catalogPath)
	}
	var manifest goldenManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return GoldenProvenance{}, fmt.Errorf("invalid golden manifest JSON %s: %w", catalogPath, err)
	}
	if err := validateGoldenManifest(manifest, catalogPath); err != nil {
		return GoldenProvenance{}, err
	}
	return GoldenProvenance{
		Provider:      strings.TrimSpace(manifest.Provider),
		Case:          strings.TrimSpace(manifest.Case),
		FidelityClass: strings.TrimSpace(manifest.FidelityClass),
		ID:            strings.TrimSpace(manifest.ID),
		ManifestPath:  catalogPath,
	}, nil
}

// RequireGoldenProvenance fails when any golden-backed record lacks attached
// provenance. Use after AttachGoldenProvenance (or equivalent fixture setup).
func RequireGoldenProvenance(records []ClassifiedRecord) error {
	var missing []string
	for _, record := range records {
		if !record.GoldenBacked {
			continue
		}
		if record.Provenance.Present() {
			continue
		}
		identity := record.Record.Identity()
		golden := strings.TrimSpace(record.Record.Golden)
		if golden == "" {
			missing = append(missing, identity+" (empty golden path)")
			continue
		}
		missing = append(missing, identity+" ("+golden+")")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"golden-backed test(s) missing attached provenance; call AttachGoldenProvenance before rendering: %s",
		strings.Join(missing, ", "),
	)
}

func validateGoldenManifest(manifest goldenManifestFile, catalogPath string) error {
	var missing []string
	if strings.TrimSpace(manifest.ID) == "" {
		missing = append(missing, "id")
	}
	if strings.TrimSpace(manifest.Provider) == "" {
		missing = append(missing, "provider")
	}
	if strings.TrimSpace(manifest.Case) == "" {
		missing = append(missing, "case")
	}
	if strings.TrimSpace(manifest.FidelityClass) == "" {
		missing = append(missing, "fidelityClass")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"golden manifest %s missing required field(s): %s",
		catalogPath,
		strings.Join(missing, ", "),
	)
}

func resolveManifestPath(baseDir, manifestPath string) string {
	if filepath.IsAbs(manifestPath) || strings.TrimSpace(baseDir) == "" {
		return manifestPath
	}
	return filepath.Join(baseDir, filepath.FromSlash(manifestPath))
}
