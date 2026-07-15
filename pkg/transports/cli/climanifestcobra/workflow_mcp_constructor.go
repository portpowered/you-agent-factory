package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry/workflowmcp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// WorkflowMCPFamilyComponents holds the canonical MCP tree and the two
// separately classified workflow compatibility leaves before root attachment.
type WorkflowMCPFamilyComponents struct {
	MCP              *cobra.Command
	MCPServe         *cobra.Command
	WorkflowPreview  *cobra.Command
	WorkflowValidate *cobra.Command
}

// MCPServeFlagBindings supplies mutable flag storage for the MCP serve handler.
type MCPServeFlagBindings struct {
	FixtureCatalogPath *string
	RuntimeBacked      *bool
	ProjectRoot        *string
	FlagUsages         map[string]string
}

// WorkflowSourceFlagBindings supplies mutable source-selection flag storage for
// one generated workflow compatibility handler.
type WorkflowSourceFlagBindings struct {
	Dir                 *string
	SourceKind          *string
	SourceValue         *string
	InlineSource        *string
	ArtifactRoot        *string
	ArgsSchema          *string
	RequestedPolicyJSON *string
	FlagUsages          map[string]string
}

// WorkflowMCPFlagBindings keeps canonical and compatibility flag state
// separate while constructing the family slice.
type WorkflowMCPFlagBindings struct {
	MCPServe         MCPServeFlagBindings
	WorkflowPreview  WorkflowSourceFlagBindings
	WorkflowValidate WorkflowSourceFlagBindings
}

// NewWorkflowMCPFamilyComponents builds generated canonical MCP metadata and
// separately classified workflow compatibility leaves.
func NewWorkflowMCPFamilyComponents(
	registries workflowmcp.Registries,
	bindings WorkflowMCPFlagBindings,
) (WorkflowMCPFamilyComponents, error) {
	mcpManifest, err := generated.MCPFamilyManifest()
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	workflowManifest, err := generated.WorkflowCompatibilityFamilyManifest()
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	return NewWorkflowMCPFamilyComponentsFromManifests(mcpManifest, workflowManifest, registries, bindings)
}

// NewWorkflowMCPFamilyComponentsFromManifests builds the family slice from one
// canonical and one compatibility manifest snapshot.
func NewWorkflowMCPFamilyComponentsFromManifests(
	mcpManifest climanifest.Manifest,
	workflowManifest climanifest.Manifest,
	registries workflowmcp.Registries,
	bindings WorkflowMCPFlagBindings,
) (WorkflowMCPFamilyComponents, error) {
	if err := validateWorkflowMCPInputs(mcpManifest, workflowManifest, registries, bindings); err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}

	mcpRecord, err := mcpManifest.CommandByID("you.mcp")
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	serveRecord, err := mcpManifest.CommandByID("you.mcp.serve")
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	previewRecord, err := workflowManifest.CommandByID("you.workflow.preview")
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	validateRecord, err := workflowManifest.CommandByID("you.workflow.validate")
	if err != nil {
		return WorkflowMCPFamilyComponents{}, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}

	mcp := workflowMCPCommandFromRecord(mcpRecord, false)
	serve, err := buildWorkflowMCPLeaf(serveRecord, registries.MCP, bindings.MCPServe, WorkflowSourceFlagBindings{})
	if err != nil {
		return WorkflowMCPFamilyComponents{}, err
	}
	preview, err := buildWorkflowMCPLeaf(previewRecord, registries.WorkflowCompatibility, MCPServeFlagBindings{}, bindings.WorkflowPreview)
	if err != nil {
		return WorkflowMCPFamilyComponents{}, err
	}
	validate, err := buildWorkflowMCPLeaf(validateRecord, registries.WorkflowCompatibility, MCPServeFlagBindings{}, bindings.WorkflowValidate)
	if err != nil {
		return WorkflowMCPFamilyComponents{}, err
	}
	mcp.AddCommand(serve)

	return WorkflowMCPFamilyComponents{MCP: mcp, MCPServe: serve, WorkflowPreview: preview, WorkflowValidate: validate}, nil
}

func validateWorkflowMCPInputs(
	mcpManifest, workflowManifest climanifest.Manifest,
	registries workflowmcp.Registries,
	bindings WorkflowMCPFlagBindings,
) error {
	if err := validateFamilyManifest(mcpManifest, climanifestgen.MCPFamilyCommandIDs, climanifestgen.AssertMCPFamilyCommandID, "canonical MCP"); err != nil {
		return err
	}
	if err := validateFamilyManifest(workflowManifest, climanifestgen.WorkflowCompatibilityFamilyCommandIDs, climanifestgen.AssertWorkflowCompatibilityFamilyCommandID, "workflow compatibility"); err != nil {
		return err
	}
	if err := workflowmcp.VerifyMCPRunnableCoverage(mcpManifest, registries.MCP); err != nil {
		return err
	}
	if err := workflowmcp.VerifyWorkflowCompatibilityRunnableCoverage(workflowManifest, registries.WorkflowCompatibility); err != nil {
		return err
	}
	required := []struct {
		name string
		ok   bool
	}{
		{name: "MCPServe.FixtureCatalogPath", ok: bindings.MCPServe.FixtureCatalogPath != nil},
		{name: "MCPServe.RuntimeBacked", ok: bindings.MCPServe.RuntimeBacked != nil},
		{name: "MCPServe.ProjectRoot", ok: bindings.MCPServe.ProjectRoot != nil},
		{name: "WorkflowPreview", ok: workflowSourceBindingsComplete(bindings.WorkflowPreview)},
		{name: "WorkflowValidate", ok: workflowSourceBindingsComplete(bindings.WorkflowValidate)},
	}
	for _, field := range required {
		if !field.ok {
			return fmt.Errorf("bindings.%s is required", field.name)
		}
	}
	return nil
}

func validateFamilyManifest(manifest climanifest.Manifest, ids []string, assertID func(string) error, label string) error {
	if len(manifest.Commands) != len(ids) {
		return fmt.Errorf("%s manifest command count = %d, want %d", label, len(manifest.Commands), len(ids))
	}
	for commandID := range manifest.Commands {
		if err := assertID(commandID); err != nil {
			return fmt.Errorf("%s classification mismatch for stable command ID %q: %w", label, commandID, err)
		}
	}
	for _, commandID := range ids {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("%s manifest missing command %q", label, commandID)
		}
	}
	return nil
}

func workflowSourceBindingsComplete(bindings WorkflowSourceFlagBindings) bool {
	return bindings.Dir != nil && bindings.SourceKind != nil && bindings.SourceValue != nil &&
		bindings.InlineSource != nil && bindings.ArtifactRoot != nil && bindings.ArgsSchema != nil &&
		bindings.RequestedPolicyJSON != nil
}

func buildWorkflowMCPLeaf(
	record climanifest.Command,
	registry *commandregistry.Registry,
	mcpBindings MCPServeFlagBindings,
	workflowBindings WorkflowSourceFlagBindings,
) (*cobra.Command, error) {
	cmd := workflowMCPCommandFromRecord(record, true)
	for _, flag := range sortedFlags(record.Flags) {
		if flag.Scope != "local" {
			continue
		}
		target, usage, err := workflowMCPLocalFlagTarget(flag, mcpBindings, workflowBindings)
		if err != nil {
			return nil, fmt.Errorf("build workflow/MCP family commands: %s: %w", record.ID, err)
		}
		if err := registerFlag(cmd.Flags(), flag, target, usage); err != nil {
			return nil, fmt.Errorf("build workflow/MCP family commands: %s: %w", record.ID, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return nil, fmt.Errorf("build workflow/MCP family commands: %s: %w", record.ID, err)
		}
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build workflow/MCP family commands: %w", err)
	}
	return cmd, nil
}

func workflowMCPCommandFromRecord(record climanifest.Command, includeLong bool) *cobra.Command {
	long := ""
	if includeLong {
		long = record.Documentation.Documentation.Description.CanonicalEnglish
	}
	return &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    long,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
		Hidden:  record.Visibility == "hidden",
	}
}

func workflowMCPLocalFlagTarget(
	flag climanifest.Flag,
	mcp MCPServeFlagBindings,
	workflow WorkflowSourceFlagBindings,
) (flagTarget, string, error) {
	usage := workflow.FlagUsages[flag.Long]
	switch flag.Long {
	case "fixture-catalog":
		return flagTarget{stringValue: mcp.FixtureCatalogPath}, mcp.FlagUsages[flag.Long], nil
	case "runtime":
		return flagTarget{boolValue: mcp.RuntimeBacked}, mcp.FlagUsages[flag.Long], nil
	case "project-root":
		return flagTarget{stringValue: mcp.ProjectRoot}, mcp.FlagUsages[flag.Long], nil
	case "dir":
		return flagTarget{stringValue: workflow.Dir}, usage, nil
	case "kind":
		return flagTarget{stringValue: workflow.SourceKind}, usage, nil
	case "value":
		return flagTarget{stringValue: workflow.SourceValue}, usage, nil
	case "inline":
		return flagTarget{stringValue: workflow.InlineSource}, usage, nil
	case "artifact-root":
		return flagTarget{stringValue: workflow.ArtifactRoot}, usage, nil
	case "args-schema":
		return flagTarget{stringValue: workflow.ArgsSchema}, usage, nil
	case "requested-policy":
		return flagTarget{stringValue: workflow.RequestedPolicyJSON}, usage, nil
	default:
		return flagTarget{}, "", fmt.Errorf("unsupported local flag %q", flag.Long)
	}
}
