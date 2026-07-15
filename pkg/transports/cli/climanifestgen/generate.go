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
	ProductionManifestPath    = climanifest.ProductionManifestPath
	CompatibilityManifestPath = climanifest.CompatibilityManifestPath

	// RepresentativeFamilyJSONPath is the generated representative-family metadata artifact.
	RepresentativeFamilyJSONPath = "pkg/transports/cli/generated/representative_family.json"

	// SessionFamilyJSONPath is the generated complete session-family metadata artifact.
	SessionFamilyJSONPath = "pkg/transports/cli/generated/session_family.json"

	// SessionFamilyCommandIDsPath is the generated stable session command ID list.
	SessionFamilyCommandIDsPath = "pkg/transports/cli/generated/session_command_ids_gen.go"

	// WorkFamilyJSONPath is the generated work-family metadata artifact.
	WorkFamilyJSONPath = "pkg/transports/cli/generated/work_family.json"

	// RunSubmitFamilyJSONPath is the generated run/submit-family metadata artifact.
	RunSubmitFamilyJSONPath = "pkg/transports/cli/generated/run_submit_family.json"

	// RunSubmitFamilyCommandIDsPath is the generated run/submit stable command ID list.
	RunSubmitFamilyCommandIDsPath = "pkg/transports/cli/generated/run_submit_command_ids_gen.go"

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

	MCPFamilyJSONPath                   = "pkg/transports/cli/generated/mcp_family.json"
	WorkflowCompatibilityFamilyJSONPath = "pkg/transports/cli/generated/workflow_compatibility_family.json"
	WorkflowMCPFamilyCommandIDsPath     = "pkg/transports/cli/generated/workflow_mcp_command_ids_gen.go"
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

// SessionFamilyArtifact returns deterministic generated session-family metadata bytes.
func SessionFamilyArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractSessionFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// WorkArtifact returns the deterministic generated work-family metadata bytes.
func WorkArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractWorkFamily(manifest)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// RunSubmitArtifact returns deterministic generated run/submit-family metadata bytes.
func RunSubmitArtifact(repositoryRoot string) ([]byte, error) {
	manifestPath := filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath))
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		return nil, err
	}
	family, err := ExtractRunSubmitFamily(manifest)
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

// MCPArtifact returns canonical MCP family metadata and enforces source classification.
func MCPArtifact(repositoryRoot string) ([]byte, error) {
	production, compatibility, err := loadWorkflowMCPManifests(repositoryRoot)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowMCPClassification(production, compatibility); err != nil {
		return nil, err
	}
	family, err := ExtractMCPFamily(production)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

// WorkflowCompatibilityArtifact returns workflow metadata without promoting it
// into any canonical generated family.
func WorkflowCompatibilityArtifact(repositoryRoot string) ([]byte, error) {
	production, compatibility, err := loadWorkflowMCPManifests(repositoryRoot)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowMCPClassification(production, compatibility); err != nil {
		return nil, err
	}
	family, err := ExtractWorkflowCompatibilityFamily(compatibility)
	if err != nil {
		return nil, err
	}
	return contractjoiner.MarshalCanonicalJSON(family)
}

func loadWorkflowMCPManifests(repositoryRoot string) (climanifest.Manifest, climanifest.Manifest, error) {
	production, err := climanifest.LoadProduction(filepath.Join(repositoryRoot, filepath.FromSlash(ProductionManifestPath)))
	if err != nil {
		return climanifest.Manifest{}, climanifest.Manifest{}, err
	}
	compatibility, err := climanifest.LoadCompatibility(filepath.Join(repositoryRoot, filepath.FromSlash(CompatibilityManifestPath)))
	if err != nil {
		return climanifest.Manifest{}, climanifest.Manifest{}, err
	}
	return production, compatibility, nil
}

func validateWorkflowMCPClassification(production, compatibility climanifest.Manifest) error {
	for _, id := range WorkflowCompatibilityFamilyCommandIDs {
		if _, exists := production.Commands[id]; exists {
			return fmt.Errorf("classification mismatch for command %q: compatibility-only workflow command is present in %s", id, ProductionManifestPath)
		}
	}
	for _, id := range MCPFamilyCommandIDs {
		if _, exists := compatibility.Commands[id]; exists {
			return fmt.Errorf("classification mismatch for command %q: canonical MCP command is present in %s", id, CompatibilityManifestPath)
		}
	}
	return nil
}

// Generate writes CLI family metadata artifacts for review and drift checks.
func Generate(repositoryRoot string) error {
	if err := writeArtifact(repositoryRoot, RepresentativeFamilyJSONPath, RepresentativeFamilyArtifact); err != nil {
		return err
	}
	if err := writeArtifact(repositoryRoot, WorkFamilyJSONPath, WorkArtifact); err != nil {
		return err
	}
	if err := writeArtifact(repositoryRoot, SessionFamilyJSONPath, SessionFamilyArtifact); err != nil {
		return err
	}
	sessionIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(SessionFamilyCommandIDsPath))
	if err := os.WriteFile(sessionIDsTarget, sessionCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SessionFamilyCommandIDsPath, err)
	}
	idsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(RepresentativeFamilyCommandIDsPath))
	if err := os.WriteFile(idsTarget, representativeAndWorkCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", RepresentativeFamilyCommandIDsPath, err)
	}
	if err := writeArtifact(repositoryRoot, RunSubmitFamilyJSONPath, RunSubmitArtifact); err != nil {
		return err
	}
	runSubmitIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(RunSubmitFamilyCommandIDsPath))
	if err := os.WriteFile(runSubmitIDsTarget, runSubmitCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", RunSubmitFamilyCommandIDsPath, err)
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
	if err := writeArtifact(repositoryRoot, MCPFamilyJSONPath, MCPArtifact); err != nil {
		return err
	}
	if err := writeArtifact(repositoryRoot, WorkflowCompatibilityFamilyJSONPath, WorkflowCompatibilityArtifact); err != nil {
		return err
	}
	workflowMCPIDsTarget := filepath.Join(repositoryRoot, filepath.FromSlash(WorkflowMCPFamilyCommandIDsPath))
	if err := os.WriteFile(workflowMCPIDsTarget, workflowMCPCommandIDsSource(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", WorkflowMCPFamilyCommandIDsPath, err)
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

func representativeAndWorkCommandIDsSource() []byte {
	var builder strings.Builder
	builder.WriteString(`// Code generated by climanifestgen. DO NOT EDIT.

package generated

`)
	writeCommandIDVar(&builder, "RepresentativeFamilyCommandIDs", ProductionManifestPath, "representative root/session-show family", RepresentativeFamilyCommandIDs)
	builder.WriteString("\n")
	writeCommandIDVar(&builder, "WorkFamilyCommandIDs", ProductionManifestPath, "work inspection/control family", WorkFamilyCommandIDs)
	return []byte(builder.String())
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

func runSubmitCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"RunSubmitFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the run/submit CLI family.",
		"RunSubmitFamilyCommandIDs",
		RunSubmitFamilyCommandIDs,
	)
}

func workflowMCPCommandIDsSource() []byte {
	var builder strings.Builder
	builder.WriteString("// Code generated by climanifestgen. DO NOT EDIT.\n\npackage generated\n\n")
	writeCommandIDVar(&builder, "MCPFamilyCommandIDs", ProductionManifestPath, "canonical MCP family", MCPFamilyCommandIDs)
	builder.WriteString("\n")
	writeCommandIDVar(&builder, "WorkflowCompatibilityFamilyCommandIDs", CompatibilityManifestPath, "separately classified workflow compatibility family", WorkflowCompatibilityFamilyCommandIDs)
	return []byte(builder.String())
}

func sessionCommandIDsSource() []byte {
	return renderCommandIDsSource(
		"SessionFamilyCommandIDs lists the stable command IDs emitted from\n// contracts/cli/commands.json for the canonical Factory Session family.",
		"SessionFamilyCommandIDs",
		SessionFamilyCommandIDs,
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

func writeCommandIDVar(builder *strings.Builder, varName, sourcePath, familyLabel string, ids []string) {
	fmt.Fprintf(builder, "// %s lists the stable command IDs emitted from\n", varName)
	fmt.Fprintf(builder, "// %s for the %s.\n", sourcePath, familyLabel)
	fmt.Fprintf(builder, "var %s = []string{\n", varName)
	for _, id := range ids {
		fmt.Fprintf(builder, "\t%q,\n", id)
	}
	builder.WriteString("}\n")
}
