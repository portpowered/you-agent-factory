package configcontractsmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestPath = "packages/api/generated/manifest.json"

// Check runs the complete read-only configuration contract closeout with the
// production global configuration parser selected by the command root.
func Check(repositoryRoot string, globalParser ParseFunc) ([]Diagnostic, error) {
	families := FamiliesWithParser(globalParser)
	diagnostics := ValidateFamilies(families, globalParser)

	_, acceptanceDiagnostics := CheckAcceptanceParity(repositoryRoot, families, AcceptanceCases())
	diagnostics = append(diagnostics, acceptanceDiagnostics...)
	diagnostics = append(diagnostics, CheckPublishedExports(repositoryRoot, families)...)

	projectionDiagnostics, err := CheckProjectionParity(repositoryRoot, families)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, projectionDiagnostics...)
	sortDiagnostics(diagnostics)
	return diagnostics, nil
}

// CheckPublishedExports verifies that every registered root has one distinct,
// active manifest export whose hash matches the staged artifact.
func CheckPublishedExports(repositoryRoot string, families []Family) []Diagnostic {
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(manifestPath)))
	if err != nil {
		return []Diagnostic{exportDiagnostic("config.export.manifest", familyConfiguration, manifestPath, "package manifest is missing or unreadable")}
	}
	var manifest struct {
		Exports map[string]struct {
			Path         string `json:"path"`
			Family       string `json:"family"`
			ArtifactHash string `json:"artifactHash"`
			Lifecycle    struct {
				State string `json:"state"`
			} `json:"lifecycle"`
		} `json:"exports"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return []Diagnostic{exportDiagnostic("config.export.manifest", familyConfiguration, manifestPath, "package manifest is malformed")}
	}

	seen := make(map[string]FamilyID, len(families))
	var diagnostics []Diagnostic
	for _, family := range families {
		manifestRelativePath := strings.TrimPrefix(family.ExportPath, "packages/api/")
		export, ok := manifest.Exports[manifestID(manifestRelativePath)]
		if !ok {
			diagnostics = append(diagnostics, exportDiagnostic("config.export.missing", family.ID, family.ExportPath, "approved export is absent from the package manifest"))
			continue
		}
		if export.Path != manifestRelativePath || export.Family != "config" || export.Lifecycle.State != "active" {
			diagnostics = append(diagnostics, exportDiagnostic("config.export.metadata", family.ID, family.ExportPath, "manifest export must use its approved path, config family, and active lifecycle"))
		}
		if prior, duplicate := seen[export.Path]; duplicate {
			diagnostics = append(diagnostics, exportDiagnostic("config.export.duplicate", family.ID, family.ExportPath, fmt.Sprintf("manifest export is already owned by configuration family %q", prior)))
		} else {
			seen[export.Path] = family.ID
		}
		artifact, readErr := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(family.ExportPath)))
		if readErr != nil {
			diagnostics = append(diagnostics, exportDiagnostic("config.export.missing", family.ID, family.ExportPath, "approved export is missing or unreadable"))
			continue
		}
		digest := sha256.Sum256(artifact)
		if export.ArtifactHash != hex.EncodeToString(digest[:]) {
			diagnostics = append(diagnostics, exportDiagnostic("config.export.hash", family.ID, family.ExportPath, "manifest artifact hash does not match the approved export"))
		}
	}
	return diagnostics
}

func manifestID(path string) string {
	withoutExtension := strings.TrimSuffix(strings.TrimSuffix(path, filepath.Ext(path)), ".schema")
	replacer := strings.NewReplacer("/", ".", "_", "-", "@", "")
	return strings.Trim(replacer.Replace(strings.ToLower(withoutExtension)), ".")
}

func exportDiagnostic(code string, family FamilyID, path, message string) Diagnostic {
	return Diagnostic{Code: code, Family: family, Path: path, Message: message}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Family != right.Family {
			return left.Family < right.Family
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Code < right.Code
	})
}
