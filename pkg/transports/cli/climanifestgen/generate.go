package climanifestgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	// ProductionManifestPath is the authored CLI command manifest input.
	ProductionManifestPath = climanifest.ProductionManifestPath

	// RepresentativeFamilyJSONPath is the generated representative-family metadata artifact.
	RepresentativeFamilyJSONPath = "pkg/transports/cli/generated/representative_family.json"

	// RepresentativeFamilyCommandIDsPath is the generated stable command ID list.
	RepresentativeFamilyCommandIDsPath = "pkg/transports/cli/generated/command_ids_gen.go"

	// FactoryConfigInitFamilyJSONPath is the generated factory/config/init metadata artifact.
	FactoryConfigInitFamilyJSONPath = "pkg/transports/cli/generated/factory_config_init_family.json"

	// FactoryConfigInitFamilyCommandIDsPath is the generated factory/config/init command ID list.
	FactoryConfigInitFamilyCommandIDsPath = "pkg/transports/cli/generated/factory_config_init_command_ids_gen.go"

	// ModelsDocsFamilyJSONPath is the generated models/docs-family metadata artifact.
	ModelsDocsFamilyJSONPath = "pkg/transports/cli/generated/models_docs_family.json"

	// ModelsDocsFamilyCommandIDsPath is the generated models/docs stable command ID list.
	ModelsDocsFamilyCommandIDsPath = "pkg/transports/cli/generated/models_docs_command_ids_gen.go"
)

// RepresentativeFamilyArtifact returns deterministic generated representative-family metadata bytes.
func RepresentativeFamilyArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractRepresentativeFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// FactoryConfigInitFamilyArtifact returns deterministic generated factory/config/init metadata bytes.
func FactoryConfigInitFamilyArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractFactoryConfigInitFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// ModelsDocsArtifact returns the deterministic generated models/docs-family metadata bytes.
func ModelsDocsArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractModelsDocsFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// Generate writes CLI family metadata artifacts for review and drift checks.
func Generate(repositoryRoot string) error {
	if err := writeArtifact(repositoryRoot, RepresentativeFamilyJSONPath, RepresentativeFamilyArtifact); err != nil {
		return err
	}
	representativeIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(RepresentativeFamilyCommandIDsPath))
	if err := os.WriteFile(representativeIDsTarget, representativeCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", RepresentativeFamilyCommandIDsPath, err)
	}

	if err := writeArtifact(repositoryRoot, FactoryConfigInitFamilyJSONPath, FactoryConfigInitFamilyArtifact); err != nil {
		return err
	}
	factoryConfigInitIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(FactoryConfigInitFamilyCommandIDsPath))
	if err := os.WriteFile(factoryConfigInitIDsTarget, factoryConfigInitCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", FactoryConfigInitFamilyCommandIDsPath, err)
	}

	if err := writeArtifact(repositoryRoot, ModelsDocsFamilyJSONPath, ModelsDocsArtifact); err != nil {
		return err
	}
	modelsDocsIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(ModelsDocsFamilyCommandIDsPath))
	if err := os.WriteFile(modelsDocsIDsTarget, modelsDocsCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ModelsDocsFamilyCommandIDsPath, err)
	}
	return nil
}

func writeArtifact(repositoryRoot, path string, producer func(string) ([]byte, error)) error {
	payload, err := producer(repositoryRoot)
	if err != nil {
		return err
	}
	target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func representativeCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"RepresentativeFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the representative root/session-show family.",
		"RepresentativeFamilyCommandIDs",
		RepresentativeFamilyCommandIDs,
	)
}

func factoryConfigInitCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"FactoryConfigInitFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the factory/config/init family.",
		"FactoryConfigInitFamilyCommandIDs",
		FactoryConfigInitFamilyCommandIDs,
	)
}

func modelsDocsCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"ModelsDocsFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the models/docs CLI family.",
		"ModelsDocsFamilyCommandIDs",
		ModelsDocsFamilyCommandIDs,
	)
}

func renderCommandIDsSource(comment, varName string, ids []string) []byte {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("\t%q,", id)
	}
	return []byte(`// Code generated by climanifestgen. DO NOT EDIT.

package generated

// ` + comment + `
var ` + varName + ` = []string{
` + strings.Join(quoted, "\n") + `
}
`)
}
